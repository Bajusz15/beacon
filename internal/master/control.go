package master

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"beacon/internal/audit"
	"beacon/internal/identity"

	"github.com/gorilla/websocket"
)

const (
	actionRemoteAccessChallenge = "remote_access_challenge"
	actionRemoteAccessUnlock    = "remote_access_unlock"
)

// controlReply is an agent→cloud reply frame sent on the control socket,
// correlated to a request via ReplyTo. The cloud relays Data to the browser.
type controlReply struct {
	ReplyTo string         `json:"reply_to"`
	Type    string         `json:"type"`
	OK      bool           `json:"ok"`
	Error   string         `json:"error,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

const (
	controlPingInterval     = 30 * time.Second
	controlReadTimeout      = 75 * time.Second
	controlHandshakeTimeout = 15 * time.Second
	controlMaxBackoff       = 5 * time.Minute
)

func startAgentControl(ctx context.Context, uc *identity.UserConfig, dispatcher *CommandDispatcher) {
	if uc == nil || dispatcher == nil || !uc.CloudReportingEnabled || strings.TrimSpace(uc.APIKey) == "" {
		return
	}
	apiKey := strings.TrimSpace(uc.APIKey)
	deviceName := strings.TrimSpace(uc.DeviceName)
	if deviceName == "" {
		deviceName = getHostname()
	}
	deviceID := strings.TrimSpace(uc.DeviceID)
	base := controlWSBase(uc.EffectiveCloudAPIBase())

	go func() {
		backoff := time.Second
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			err := runAgentControl(ctx, base, apiKey, deviceName, deviceID, dispatcher)
			if ctx.Err() != nil {
				return
			}
			logger.Infof("Agent control socket disconnected: %v; reconnecting in %s", err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > controlMaxBackoff {
				backoff = controlMaxBackoff
			}
		}
	}()
}

func runAgentControl(ctx context.Context, base, apiKey, deviceName, deviceID string, dispatcher *CommandDispatcher) error {
	headers := http.Header{}
	headers.Set("X-API-Key", apiKey)
	headers.Set("X-Device-Name", deviceName)
	if deviceID != "" {
		headers.Set("X-Device-ID", deviceID)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: controlHandshakeTimeout,
		Proxy:            http.ProxyFromEnvironment,
	}
	conn, resp, err := dialer.DialContext(ctx, base+"/agent/control/ws", headers)
	if err != nil {
		if resp != nil {
			defer func() { _ = resp.Body.Close() }()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			return fmt.Errorf("dial: %w (status=%d body=%q)", err, resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.Close() }()
	logger.Infof("Agent control socket connected")

	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(controlReadTimeout))
		return nil
	})
	_ = conn.SetReadDeadline(time.Now().Add(controlReadTimeout))

	// writeMu serializes all writes to conn (ping frames and reply frames).
	var writeMu sync.Mutex

	pingCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		tk := time.NewTicker(controlPingInterval)
		defer tk.Stop()
		for {
			select {
			case <-pingCtx.Done():
				return
			case <-tk.C:
				writeMu.Lock()
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				err := conn.WriteMessage(websocket.PingMessage, nil)
				writeMu.Unlock()
				if err != nil {
					_ = conn.Close()
					return
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(controlReadTimeout))

		var cmd HeartbeatCommand
		if err := json.Unmarshal(raw, &cmd); err != nil {
			logger.Infof("Agent control: invalid command: %v", err)
			continue
		}
		action := strings.TrimSpace(cmd.Action)
		if action == "" {
			continue
		}
		// Remote-access challenge/unlock are handled inline and answered on this
		// same socket — they are NOT remote commands and must not be dispatched.
		if action == actionRemoteAccessChallenge || action == actionRemoteAccessUnlock {
			handleRemoteAccessControl(conn, &writeMu, dispatcher, cmd)
			continue
		}
		dispatcher.DispatchCommandsWithSource("control_ws", []HeartbeatCommand{cmd})
	}
}

// handleRemoteAccessControl answers a challenge or unlock request from the
// cloud relay against the dispatcher's in-memory grant state, writing a
// correlated reply frame back on the control socket.
func handleRemoteAccessControl(conn *websocket.Conn, writeMu *sync.Mutex, dispatcher *CommandDispatcher, cmd HeartbeatCommand) {
	grants := dispatcher.Grants()
	sid, _ := cmd.Payload["session_id"].(string)
	sid = strings.TrimSpace(sid)

	reply := controlReply{ReplyTo: cmd.ID, Type: cmd.Action}

	switch cmd.Action {
	case actionRemoteAccessChallenge:
		if sid == "" {
			reply.Error = "session_id required"
			break
		}
		// The action being unlocked (terminal_open / tunnel_connect) is what the
		// proof will be bound to; default to terminal_open for back-compat.
		boundAction, _ := cmd.Payload["bind_action"].(string)
		boundAction = strings.TrimSpace(boundAction)
		if boundAction == "" {
			boundAction = actionTerminalOpen
		}
		ch, err := grants.Challenge(boundAction, sid)
		if err != nil {
			reply.Error = err.Error()
			break
		}
		reply.OK = true
		reply.Data = map[string]any{
			"nonce":      ch.Nonce,
			"salt":       ch.Salt,
			"params":     ch.Params,
			"action":     boundAction,
			"expires_at": ch.ExpiresAt,
		}
		dispatcher.auditCmd(audit.Event{
			Action: actionRemoteAccessChallenge, Source: "control_ws", Status: "received",
			CommandID: cmd.ID, Detail: "challenge issued",
		})

	case actionRemoteAccessUnlock:
		boundAction, _ := cmd.Payload["action"].(string)
		boundAction = strings.TrimSpace(boundAction)
		if boundAction == "" {
			boundAction = actionTerminalOpen
		}
		nonce, _ := cmd.Payload["nonce"].(string)
		proof, _ := cmd.Payload["proof"].(string)
		err := grants.Verify(boundAction, sid, strings.TrimSpace(nonce), strings.TrimSpace(proof))
		if err != nil {
			reply.Error = err.Error()
			dispatcher.auditCmd(audit.Event{
				Action: actionRemoteAccessUnlock, Source: "control_ws", Status: "failed",
				CommandID: cmd.ID, Detail: err.Error(),
			})
			break
		}
		reply.OK = true
		dispatcher.auditCmd(audit.Event{
			Action: actionRemoteAccessUnlock, Source: "control_ws", Status: "executed",
			CommandID: cmd.ID, Detail: "remote-access unlocked for session",
		})
	}

	writeMu.Lock()
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err := conn.WriteJSON(reply)
	writeMu.Unlock()
	if err != nil {
		logger.Infof("Agent control: failed to write remote-access reply: %v", err)
	}
}

func controlWSBase(apiBase string) string {
	base := strings.TrimSuffix(apiBase, "/")
	if strings.HasPrefix(base, "https://") {
		return "wss://" + strings.TrimPrefix(base, "https://")
	}
	if strings.HasPrefix(base, "http://") {
		return "ws://" + strings.TrimPrefix(base, "http://")
	}
	return "ws://" + base
}
