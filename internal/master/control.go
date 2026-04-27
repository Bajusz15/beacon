package master

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"beacon/internal/identity"

	"github.com/gorilla/websocket"
)

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
	base := controlWSBase(uc.EffectiveCloudAPIBase())

	go func() {
		backoff := time.Second
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			err := runAgentControl(ctx, base, apiKey, deviceName, dispatcher)
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

func runAgentControl(ctx context.Context, base, apiKey, deviceName string, dispatcher *CommandDispatcher) error {
	headers := http.Header{}
	headers.Set("X-API-Key", apiKey)
	headers.Set("X-Device-Name", deviceName)

	dialer := websocket.Dialer{
		HandshakeTimeout: controlHandshakeTimeout,
		Proxy:            http.ProxyFromEnvironment,
	}
	conn, _, err := dialer.DialContext(ctx, base+"/agent/control/ws", headers)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.Close() }()
	logger.Infof("Agent control socket connected")

	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(controlReadTimeout))
		return nil
	})
	_ = conn.SetReadDeadline(time.Now().Add(controlReadTimeout))

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
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
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
		if strings.TrimSpace(cmd.Action) == "" {
			continue
		}
		dispatcher.DispatchCommands([]HeartbeatCommand{cmd})
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
