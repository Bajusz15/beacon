package tunnel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"beacon/internal/ipc"

	"github.com/gorilla/websocket"
)

func TestClientWSURL(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{"https", "https://beaconinfra.dev/api/", "wss://beaconinfra.dev/api/tunnel/connect?tunnel_id=home"},
		{"http", "http://localhost:8080/api", "ws://localhost:8080/api/tunnel/connect?tunnel_id=home"},
		{"bare", "localhost:8080/api", "localhost:8080/api/tunnel/connect?tunnel_id=home"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(ClientConfig{TunnelID: "home", CloudURL: tt.base})
			if got := c.wsURL(); got != tt.want {
				t.Fatalf("wsURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientHealthAndConnectionState(t *testing.T) {
	dir := t.TempDir()
	c := NewClient(ClientConfig{
		TunnelID: "home",
		Dial:     DialTarget{Protocol: "http", Host: "127.0.0.1", Port: 8123},
		IPCDir:   dir,
	})

	c.writeHealthOnce()
	report := readTunnelHealth(t, dir)
	if report.Status != ipc.StatusDown {
		t.Fatalf("expected down health, got %q", report.Status)
	}
	if report.Metrics["connected"] != false {
		t.Fatalf("expected disconnected metric, got %+v", report.Metrics)
	}

	c.setConnected(true)
	if !c.isConnected() {
		t.Fatal("expected connected state")
	}
	c.writeHealthOnce()
	report = readTunnelHealth(t, dir)
	if report.Status != ipc.StatusHealthy {
		t.Fatalf("expected healthy status, got %q", report.Status)
	}
	if report.Metrics["tunnel_id"] != "home" {
		t.Fatalf("expected tunnel metric, got %+v", report.Metrics)
	}

	c.closeConn()
	if c.isConnected() {
		t.Fatal("expected closeConn to mark disconnected")
	}
}

func TestClientStreamCloseRemovesUnknownAndKnownStreams(t *testing.T) {
	c := NewClient(ClientConfig{TunnelID: "home"})
	c.handleWSClose(Message{StreamID: "missing"})

	server, _ := testWebSocketConn(t)
	defer server.Close()
	c.streams.Store("stream-1", server)
	c.handleWSClose(Message{StreamID: "stream-1"})
	if _, ok := c.streams.Load("stream-1"); ok {
		t.Fatal("expected stream to be removed")
	}
}

func TestClientSendMessageAndWSFrame(t *testing.T) {
	c := NewClient(ClientConfig{TunnelID: "home"})
	c.sendMessage(&Message{Type: MsgPong})

	server, client := testWebSocketConn(t)
	defer server.Close()
	c.conn = server
	c.sendMessage(&Message{Type: MsgPong})
	var msg Message
	if err := client.ReadJSON(&msg); err != nil {
		t.Fatalf("read sent message: %v", err)
	}
	if msg.Type != MsgPong {
		t.Fatalf("expected pong, got %+v", msg)
	}

	c.handleWSFrame(Message{StreamID: "missing", WSPayload: "bad-base64"})
	c.streams.Store("stream-1", server)
	c.handleWSFrame(Message{StreamID: "stream-1", WSPayload: "bad-base64"})
}

func testWebSocketConn(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()
	connCh := make(chan *websocket.Conn, 1)
	var upgrader websocket.Upgrader
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		connCh <- conn
	}))
	t.Cleanup(srv.Close)

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	select {
	case conn := <-connCh:
		return conn, client
	case <-time.After(time.Second):
		t.Fatal("websocket server did not receive connection")
		return nil, nil
	}
}

func readTunnelHealth(t *testing.T, dir string) ipc.HealthReport {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "health.json"))
	if err != nil {
		t.Fatalf("read health: %v", err)
	}
	var report ipc.HealthReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("parse health: %v", err)
	}
	if time.Since(report.Timestamp) > time.Minute {
		t.Fatalf("health timestamp too old: %s", report.Timestamp)
	}
	return report
}
