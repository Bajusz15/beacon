package tunnel

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sync"

	"beacon/internal/identity"
	"beacon/internal/ipc"
)

// TunnelStatus describes a managed tunnel's current state.
type TunnelStatus struct {
	ID        string `json:"id"`
	LocalPort int    `json:"local_port"`
	Status    string `json:"status"` // "connected", "reconnecting", "failed", "disabled"
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

	ipcDir := filepath.Join(tm.ipcBase, "tunnel-"+t.ID)

	client := NewClient(ClientConfig{
		TunnelID:   t.ID,
		LocalPort:  t.LocalPort,
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
		log.Printf("[Beacon master] Starting tunnel %s -> localhost:%d", t.ID, t.LocalPort)
		if err := client.Run(tm.ctx); err != nil && tm.ctx.Err() == nil {
			log.Printf("[Beacon master] Tunnel %s stopped: %v", t.ID, err)
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
		statuses = append(statuses, TunnelStatus{
			ID:        id,
			LocalPort: mt.cfg.LocalPort,
			Status:    status,
		})
	}
	return statuses
}

// Shutdown stops all tunnel goroutines and waits for them to finish.
func (tm *TunnelManager) Shutdown() {
	log.Printf("[Beacon master] Stopping all tunnels...")
	tm.cancel()
	tm.wg.Wait()
	log.Printf("[Beacon master] All tunnels stopped")
}

func isTunnelEnabled(t identity.TunnelConfig) bool {
	if t.ID == "" || t.LocalPort <= 0 {
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

