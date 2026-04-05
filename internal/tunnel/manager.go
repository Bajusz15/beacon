package tunnel

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"beacon/internal/identity"
	"beacon/internal/ipc"
	"beacon/internal/logging"
)

var managerLog = logging.New("tunnel")

// MaxActiveTunnels is the maximum number of tunnels that can be active simultaneously per user.
// Matches the server-side limit. Additional configured tunnels stay dormant.
const MaxActiveTunnels = 2

// TunnelStatus describes a managed tunnel's current state.
type TunnelStatus struct {
	ID              string `json:"id"`
	LocalPort       int    `json:"local_port"`
	UpstreamHost    string `json:"upstream_host,omitempty"`
	UpstreamProtocol string `json:"upstream_protocol,omitempty"`
	Status          string `json:"status"` // "connected", "reconnecting", "failed", "disabled"
}

// TunnelManager manages tunnel goroutines within the master process.
type TunnelManager struct {
	mu      sync.RWMutex
	tunnels map[string]*managedTunnel
	ipcBase string
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

type managedTunnel struct {
	client *Client
	cfg    identity.TunnelConfig
}

// NewTunnelManager creates a new tunnel manager.
func NewTunnelManager(ctx context.Context) (*TunnelManager, error) {
	ipcBase, err := ipc.IPCDir()
	if err != nil {
		return nil, fmt.Errorf("get IPC dir: %w", err)
	}

	childCtx, cancel := context.WithCancel(ctx)

	return &TunnelManager{
		tunnels: make(map[string]*managedTunnel),
		ipcBase: ipcBase,
		ctx:     childCtx,
		cancel:  cancel,
	}, nil
}

// EnsureStarted starts a single tunnel if not already running (for piggyback tunnel_connect).
func (tm *TunnelManager) EnsureStarted(t identity.TunnelConfig, cloudURL, apiKey, deviceName string) error {
	if !isTunnelEnabled(t) {
		return fmt.Errorf("tunnel %q is disabled or invalid", t.ID)
	}
	tm.start(t, cloudURL, apiKey, deviceName)
	return nil
}

func (tm *TunnelManager) start(t identity.TunnelConfig, cloudURL, apiKey, deviceName string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.tunnels[t.ID]; exists {
		return
	}

	proto, host, port, err := t.EffectiveUpstream()
	if err != nil {
		managerLog.Warnf("Tunnel %s: invalid upstream: %v", t.ID, err)
		return
	}
	if err := ValidateDialTarget(proto, host, port); err != nil {
		managerLog.Warnf("Tunnel %s: upstream not allowed: %v", t.ID, err)
		return
	}

	ipcDir := filepath.Join(tm.ipcBase, "tunnel-"+t.ID)

	client := NewClient(ClientConfig{
		TunnelID:   t.ID,
		Dial:       DialTarget{Protocol: proto, Host: host, Port: port},
		CloudURL:   cloudURL,
		APIKey:     apiKey,
		DeviceName: deviceName,
		IPCDir:     ipcDir,
	})

	mt := &managedTunnel{
		client: client,
		cfg:    t,
	}
	tm.tunnels[t.ID] = mt

	tm.wg.Add(1)
	go func() {
		defer tm.wg.Done()
		managerLog.Infof("Starting tunnel %s -> %s://%s:%d", t.ID, proto, host, port)
		if err := client.Run(tm.ctx); err != nil && tm.ctx.Err() == nil {
			managerLog.Warnf("Tunnel %s stopped: %v", t.ID, err)
		}
	}()
}

// GetIPCReaders returns IPC readers for all managed tunnels.
func (tm *TunnelManager) GetIPCReaders() map[string]*ipc.Reader {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	readers := make(map[string]*ipc.Reader, len(tm.tunnels))
	for id := range tm.tunnels {
		ipcDir := filepath.Join(tm.ipcBase, "tunnel-"+id)
		readers["tunnel-"+id] = ipc.NewReader(ipcDir)
	}
	return readers
}

// GetTunnelStatuses returns the current status of all managed tunnels.
func (tm *TunnelManager) GetTunnelStatuses() []TunnelStatus {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	statuses := make([]TunnelStatus, 0, len(tm.tunnels))
	for id, mt := range tm.tunnels {
		status := "reconnecting"
		if mt.client.connected {
			status = "connected"
		}
		proto, host, port, _ := mt.cfg.EffectiveUpstream()
		statuses = append(statuses, TunnelStatus{
			ID:               id,
			LocalPort:        port,
			UpstreamHost:     host,
			UpstreamProtocol: proto,
			Status:           status,
		})
	}
	return statuses
}

// Shutdown stops all tunnel goroutines and waits for them to finish.
func (tm *TunnelManager) Shutdown() {
	managerLog.Infof("Stopping all tunnels...")
	tm.cancel()
	tm.wg.Wait()
	managerLog.Infof("All tunnels stopped")
}

func isTunnelEnabled(t identity.TunnelConfig) bool {
	if strings.TrimSpace(t.ID) == "" {
		return false
	}
	_, _, _, err := t.EffectiveUpstream()
	if err != nil {
		return false
	}
	if t.Enabled == nil {
		return true
	}
	return *t.Enabled
}

// ConfigTunnelEnabled reports whether the tunnel entry is valid and enabled in config.
func ConfigTunnelEnabled(t identity.TunnelConfig) bool {
	return isTunnelEnabled(t)
}
