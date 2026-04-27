//go:build unix

package terminal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

var allowedShells = map[string]bool{
	"/bin/sh":       true,
	"/bin/bash":     true,
	"/bin/zsh":      true,
	"/bin/ash":      true,
	"/bin/dash":     true,
	"/usr/bin/bash": true,
	"/usr/bin/zsh":  true,
	"/usr/bin/fish": true,
}

func safeShell() (string, error) {
	if s := strings.TrimSpace(os.Getenv("SHELL")); s != "" && allowedShells[s] {
		if _, err := os.Stat(s); err == nil {
			return s, nil
		}
	}
	for _, fallback := range []string{"/bin/bash", "/bin/sh"} {
		if _, err := os.Stat(fallback); err == nil {
			return fallback, nil
		}
	}
	return "", fmt.Errorf("no suitable shell found")
}

// RunSession dials the cloud terminal WebSocket and relays a local shell until disconnect.
func RunSession(cfg RunConfig) error {
	if strings.TrimSpace(cfg.WSURL) == "" {
		return fmt.Errorf("ws_url is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return fmt.Errorf("api key is required for terminal")
	}
	h := http.Header{}
	h.Set("X-API-Key", strings.TrimSpace(cfg.APIKey))
	if n := strings.TrimSpace(cfg.DeviceName); n != "" {
		h.Set("X-Device-Name", n)
	}
	dialer := websocket.Dialer{HandshakeTimeout: 20 * time.Second, Proxy: http.ProxyFromEnvironment}
	conn, _, err := dialer.Dial(cfg.WSURL, h)
	if err != nil {
		return fmt.Errorf("dial terminal websocket: %w", err)
	}
	defer func() { _ = conn.Close() }()

	shell, err := safeShell()
	if err != nil {
		return err
	}
	c := exec.Command(shell, "-l") // #nosec G702 -- shell is validated by safeShell allow-list
	c.Env = os.Environ()
	if wd, err := os.Getwd(); err == nil {
		c.Dir = wd
	}
	ptmx, err := pty.Start(c)
	if err != nil {
		return fmt.Errorf("start pty: %w", err)
	}
	defer func() { _ = ptmx.Close() }()
	defer func() { _ = c.Process.Kill() }()
	_ = pty.Setsize(ptmx, &pty.Winsize{Rows: 40, Cols: 120})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var once sync.Once
	finish := func() { once.Do(cancel) }

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		defer finish()
		buf := make([]byte, 32*1024)
		for {
			n, rerr := ptmx.Read(buf)
			if n > 0 {
				_ = conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
				if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					return
				}
			}
			if rerr != nil {
				if rerr != io.EOF {
					_ = rerr
				}
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		defer finish()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
			mt, p, rerr := conn.ReadMessage()
			if rerr != nil {
				return
			}
			if mt == websocket.TextMessage {
				var ctrl struct {
					Type string `json:"type"`
					Cols int    `json:"cols"`
					Rows int    `json:"rows"`
				}
				if json.Unmarshal(p, &ctrl) == nil && strings.EqualFold(ctrl.Type, "resize") && ctrl.Cols > 0 && ctrl.Rows > 0 {
					_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(ctrl.Rows), Cols: uint16(ctrl.Cols)})
				}
				continue
			}
			if _, werr := ptmx.Write(p); werr != nil {
				return
			}
		}
	}()

	wg.Wait()
	return nil
}
