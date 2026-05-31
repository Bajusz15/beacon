package tunnel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"beacon/internal/ipc"
	"beacon/internal/logging"

	"github.com/gorilla/websocket"
)

const (
	maxReconnects    = 50
	maxBackoff       = 24 * time.Hour
	healthWriteEvery = 10 * time.Second
	pingInterval     = 30 * time.Second
	writeTimeout     = 10 * time.Second
	readTimeout      = 60 * time.Second
	// resetAttemptsAfter: if a connection stays up longer than this, reset the attempt counter.
	resetAttemptsAfter = 5 * time.Minute
)

// ClientConfig holds parameters for a tunnel Client.
type ClientConfig struct {
	TunnelID   string
	Dial       DialTarget
	CloudURL   string // e.g. "https://beaconinfra.dev/api"
	APIKey     string
	DeviceName string
	IPCDir     string // e.g. ~/.beacon/ipc/tunnel-homeassistant
}

// Client maintains a WebSocket connection to the cloud and proxies traffic to a local service.
type Client struct {
	cfg  ClientConfig
	conn *websocket.Conn
	mu   sync.Mutex // protects conn writes
	log  *logging.Logger

	// Active WebSocket passthrough streams (streamID -> local ws conn)
	streams sync.Map

	connected bool
	startedAt time.Time
}

// NewClient creates a tunnel client.
func NewClient(cfg ClientConfig) *Client {
	return &Client{
		cfg:       cfg,
		log:       logging.New("tunnel " + cfg.TunnelID),
		startedAt: time.Now(),
	}
}

// Run connects to the cloud and handles messages until ctx is canceled.
// Reconnects automatically with exponential backoff on failure.
func (c *Client) Run(ctx context.Context) error {
	if err := os.MkdirAll(c.cfg.IPCDir, 0755); err != nil {
		return fmt.Errorf("create IPC dir: %w", err)
	}

	// Start health writer
	go c.healthLoop(ctx)

	attempt := 0
	for {
		select {
		case <-ctx.Done():
			c.closeConn()
			return ctx.Err()
		default:
		}

		connStart := time.Now()
		err := c.connectAndServe(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// If the connection was stable for a while, reset backoff so transient blips don't accumulate.
		if time.Since(connStart) >= resetAttemptsAfter {
			attempt = 0
		}

		attempt++
		if attempt > maxReconnects {
			c.log.Warnf("Max reconnects (%d) exceeded, giving up", maxReconnects)
			c.setConnected(false)
			c.writeHealthOnce()
			return fmt.Errorf("max reconnects exceeded after error: %v", err)
		}

		backoff := time.Duration(1<<uint(attempt)) * time.Second
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		c.log.Infof("Disconnected (%v), reconnecting in %v (attempt %d/%d)",
			err, backoff, attempt, maxReconnects)
		c.setConnected(false)
		c.writeHealthOnce()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
}

// connectAndServe dials the cloud WebSocket and processes messages until disconnect.
func (c *Client) connectAndServe(ctx context.Context) error {
	wsURL := c.wsURL()
	headers := http.Header{}
	headers.Set("X-API-Key", c.cfg.APIKey)
	headers.Set("X-Device-Name", c.cfg.DeviceName)

	dialer := websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
	}
	conn, _, err := dialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		return fmt.Errorf("dial %s: %w", wsURL, err)
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.mu.Unlock()

	c.log.Infof("Connected to %s", wsURL)

	defer c.closeConn()

	// Start ping sender
	pingCtx, pingCancel := context.WithCancel(ctx)
	defer pingCancel()
	go c.pingLoop(pingCtx)

	// Read loop
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			c.log.Warnf("Invalid message: %v", err)
			continue
		}

		switch msg.Type {
		case MsgHTTPRequest:
			go c.handleHTTPRequest(msg)
		case MsgWSOpen:
			go c.handleWSOpen(ctx, msg)
		case MsgWSFrame:
			c.handleWSFrame(msg)
		case MsgWSClose:
			c.handleWSClose(msg)
		case MsgPing:
			c.sendMessage(&Message{Type: MsgPong})
		case MsgPong:
			// Keepalive response from BeaconInfra.
		default:
			c.log.Warnf("Unknown message type: %s", msg.Type)
		}
	}
}

func (c *Client) handleHTTPRequest(msg Message) {
	c.log.Infof("HTTP request from cloud method=%s path=%s headers=%s body_b64_bytes=%d",
		msg.Method, safeTunnelLogPath(msg.Path), proxyHeaderSummary(msg.Headers), len(msg.Body))
	resp, err := ProxyHTTPRequest(c.cfg.Dial, &msg)
	if err != nil {
		c.log.Errorf("Proxy error for %s %s: %v", msg.Method, msg.Path, err)
		resp = &Message{
			Type:      MsgHTTPResponse,
			RequestID: msg.RequestID,
			Status:    502,
			Error:     err.Error(),
		}
	}
	if resp != nil {
		c.log.Infof("HTTP response to cloud method=%s path=%s status=%d error=%q body_b64_bytes=%d",
			msg.Method, safeTunnelLogPath(msg.Path), resp.Status, resp.Error, len(resp.Body))
	}
	c.sendMessage(resp)
}

func (c *Client) handleWSOpen(ctx context.Context, msg Message) {
	c.log.Infof("WS open requested path=%s stream=%s headers=%s", safeTunnelLogPath(msg.Path), msg.StreamID, proxyHeaderSummary(msg.Headers))
	localConn, err := ProxyWSOpen(ctx, c.cfg.Dial, msg.Path, msg.Headers)
	if err != nil {
		c.log.Errorf("WS open failed for %s: %v", msg.Path, err)
		c.sendMessage(&Message{
			Type:     MsgWSOpenResult,
			StreamID: msg.StreamID,
			Error:    err.Error(),
		})
		return
	}

	c.streams.Store(msg.StreamID, localConn)
	c.log.Infof("WS open succeeded path=%s stream=%s", safeTunnelLogPath(msg.Path), msg.StreamID)
	c.sendMessage(&Message{
		Type:     MsgWSOpenResult,
		StreamID: msg.StreamID,
	})

	// Read from local WS and forward to cloud
	go func() {
		defer func() {
			_ = localConn.Close()
			c.streams.Delete(msg.StreamID)
			c.sendMessage(&Message{Type: MsgWSClose, StreamID: msg.StreamID})
			c.log.Infof("WS upstream stream closed path=%s stream=%s", safeTunnelLogPath(msg.Path), msg.StreamID)
		}()
		for {
			msgType, data, err := localConn.ReadMessage()
			if err != nil {
				c.log.Warnf("WS upstream read ended path=%s stream=%s err=%v", safeTunnelLogPath(msg.Path), msg.StreamID, err)
				return
			}
			c.log.Infof("WS upstream -> cloud path=%s stream=%s frame_type=%d bytes=%d", safeTunnelLogPath(msg.Path), msg.StreamID, msgType, len(data))
			c.sendMessage(&Message{
				Type:        MsgWSFrame,
				StreamID:    msg.StreamID,
				WSFrameType: msgType,
				WSPayload:   base64.StdEncoding.EncodeToString(data),
			})
		}
	}()
}

func (c *Client) handleWSFrame(msg Message) {
	val, ok := c.streams.Load(msg.StreamID)
	if !ok {
		c.log.Warnf("WS frame for unknown stream=%s frame_type=%d payload_b64_bytes=%d", msg.StreamID, msg.WSFrameType, len(msg.WSPayload))
		return
	}
	localConn := val.(*websocket.Conn)

	data, err := base64.StdEncoding.DecodeString(msg.WSPayload)
	if err != nil {
		c.log.Warnf("WS frame decode failed stream=%s err=%v", msg.StreamID, err)
		return
	}
	c.log.Infof("WS cloud -> upstream stream=%s frame_type=%d bytes=%d", msg.StreamID, msg.WSFrameType, len(data))
	if err := localConn.WriteMessage(msg.WSFrameType, data); err != nil {
		c.log.Warnf("WS upstream write failed stream=%s frame_type=%d bytes=%d err=%v", msg.StreamID, msg.WSFrameType, len(data), err)
	}
}

func (c *Client) handleWSClose(msg Message) {
	val, ok := c.streams.LoadAndDelete(msg.StreamID)
	if !ok {
		c.log.Infof("WS close for unknown stream=%s", msg.StreamID)
		return
	}
	localConn := val.(*websocket.Conn)
	_ = localConn.Close()
	c.log.Infof("WS close from cloud stream=%s", msg.StreamID)
}

func (c *Client) sendMessage(msg *Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	if err := c.conn.WriteJSON(msg); err != nil {
		c.log.Errorf("Write error: %v", err)
	}
}

func (c *Client) pingLoop(ctx context.Context) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.sendMessage(&Message{Type: MsgPing})
		}
	}
}

// setConnected records connection state under the same mutex that guards conn,
// so the health-writer goroutine never reads it concurrently with a write.
func (c *Client) setConnected(v bool) {
	c.mu.Lock()
	c.connected = v
	c.mu.Unlock()
}

// isConnected reads connection state under the mutex.
func (c *Client) isConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

func (c *Client) closeConn() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	c.connected = false

	// Close all active WS streams
	c.streams.Range(func(key, value any) bool {
		if conn, ok := value.(*websocket.Conn); ok {
			_ = conn.Close()
		}
		c.streams.Delete(key)
		return true
	})
}

func (c *Client) healthLoop(ctx context.Context) {
	c.writeHealthOnce()
	ticker := time.NewTicker(healthWriteEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.writeHealthOnce()
		}
	}
}

func (c *Client) writeHealthOnce() {
	connected := c.isConnected()
	status := ipc.StatusDown
	if connected {
		status = ipc.StatusHealthy
	}

	report := &ipc.HealthReport{
		ProjectID:     "tunnel-" + c.cfg.TunnelID,
		Timestamp:     time.Now(),
		Status:        status,
		UptimeSeconds: int64(time.Since(c.startedAt).Seconds()),
		Metrics: map[string]any{
			"type":            "tunnel",
			"tunnel_id":       c.cfg.TunnelID,
			"local_port":      c.cfg.Dial.Port,
			"upstream_host":   c.cfg.Dial.Host,
			"upstream_scheme": c.cfg.Dial.Protocol,
			"connected":       connected,
		},
	}

	data, err := json.Marshal(report)
	if err != nil {
		return
	}

	tmp := filepath.Join(c.cfg.IPCDir, "health.json.tmp")
	target := filepath.Join(c.cfg.IPCDir, "health.json")
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return
	}
	_ = os.Rename(tmp, target)
}

// wsURL converts the cloud API base URL to a WebSocket URL.
func (c *Client) wsURL() string {
	base := c.cfg.CloudURL
	base = strings.TrimSuffix(base, "/")

	// Replace http(s) with ws(s)
	if strings.HasPrefix(base, "https://") {
		base = "wss://" + strings.TrimPrefix(base, "https://")
	} else if strings.HasPrefix(base, "http://") {
		base = "ws://" + strings.TrimPrefix(base, "http://")
	}

	return fmt.Sprintf("%s/tunnel/connect?tunnel_id=%s", base, c.cfg.TunnelID)
}
