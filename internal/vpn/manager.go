package vpn

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"beacon/internal/identity"
)

// PeerResolver is the indirection the manager uses to fetch a peer's public key
// and endpoint from BeaconInfra. The cloud client implements this; we keep it
// behind an interface so the manager can be unit-tested without HTTP.
type PeerResolver interface {
	// RegisterVPN posts the local public key and listen port. Returns the
	// VPN address the server allocated for this device.
	RegisterVPN(ctx context.Context, publicKey, role string, listenPort int, endpoint string) (string, error)
	// GetPeer fetches a peer's public key + endpoint by device name (within the same user).
	GetPeer(ctx context.Context, deviceName string) (*PeerInfo, error)
	// DeregisterVPN removes the device's VPN config from the cloud.
	DeregisterVPN(ctx context.Context) error
}

// Manager owns the lifecycle of the local WireGuard interface. It is created
// once by the master at startup, then Reconcile() is called every time the
// user's VPN config changes (and on a periodic timer to refresh stats).
type Manager struct {
	resolver PeerResolver

	mu      sync.Mutex
	cfg     *identity.VPNConfig // last config we acted on (nil = VPN disabled)
	wg      *wgDevice
	keyPair *KeyPair
	egress  string // detected egress iface (exit node only) — needed for teardown
	status  Status
}

// NewManager constructs a Manager. Pass the cloud client (or a mock) as resolver.
func NewManager(resolver PeerResolver) *Manager {
	return &Manager{resolver: resolver}
}

// Status returns a copy of the current VPN runtime state. Safe for concurrent reads.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.status
	if m.wg != nil {
		if rx, tx, lh, err := m.wg.stats(); err == nil {
			s.BytesRx = rx
			s.BytesTx = tx
			if !lh.IsZero() {
				s.LastHandshake = lh
				s.Connected = time.Since(lh) < 3*time.Minute
			}
		}
	}
	return s
}

// Reconcile aligns the running interface with the desired config. Called by the
// master after every config reload. The contract is:
//   - cfg == nil OR cfg.Enabled == false → tear everything down (idempotent)
//   - cfg.Enabled == true and not yet running → bring it up
//   - cfg.Enabled == true and already running with same role/peer → no-op
//   - cfg.Enabled == true but role/peer changed → tear down + bring up fresh
func (m *Manager) Reconcile(ctx context.Context, cfg *identity.VPNConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Disabled path
	if cfg == nil || !cfg.Enabled {
		m.shutdownLocked()
		return nil
	}

	// Already running with the same intent — refresh peer info but don't churn the device.
	if m.wg != nil && m.cfg != nil &&
		m.cfg.Role == cfg.Role && m.cfg.PeerDevice == cfg.PeerDevice && m.cfg.ListenPort == cfg.ListenPort {
		if cfg.Role == string(RoleClient) {
			// Refresh peer endpoint (it may have changed if the exit node's public IP rotated).
			if err := m.refreshClientPeerLocked(ctx); err != nil {
				m.status.Error = err.Error()
				return err
			}
		}
		m.status.Error = ""
		return nil
	}

	// Role/peer changed (or first start) — tear down then bring up.
	m.shutdownLocked()

	switch cfg.Role {
	case string(RoleExitNode):
		return m.startExitNodeLocked(ctx, cfg)
	case string(RoleClient):
		return m.startClientLocked(ctx, cfg)
	default:
		return fmt.Errorf("unknown vpn role %q", cfg.Role)
	}
}

func (m *Manager) startExitNodeLocked(ctx context.Context, cfg *identity.VPNConfig) error {
	kp, err := LoadOrCreatePrivateKey()
	if err != nil {
		return err
	}
	listenPort := cfg.ListenPort
	if listenPort <= 0 {
		listenPort = DefaultListenPort
	}

	// Register with cloud first so we get an address allocated.
	addr, err := m.resolver.RegisterVPN(ctx, kp.PublicKey, string(RoleExitNode), listenPort, "")
	if err != nil {
		return fmt.Errorf("register vpn with cloud: %w", err)
	}
	registered := true
	defer func() {
		if registered {
			_ = m.resolver.DeregisterVPN(ctx)
		}
	}()

	wg, err := createWGDevice(InterfaceName, kp.PrivateKey, listenPort)
	if err != nil {
		return err
	}

	egress, err := applyExitNodeNetwork(InterfaceName, addr)
	if err != nil {
		wg.close()
		return err
	}

	registered = false // success — keep the registration
	m.wg = wg
	m.keyPair = kp
	m.egress = egress
	m.cfg = cfg
	m.status = Status{
		Enabled:       true,
		Role:          RoleExitNode,
		InterfaceName: InterfaceName,
		VPNAddress:    addr,
		ListenPort:    listenPort,
		PublicKey:     kp.PublicKey,
	}
	_ = identity.SetVPNExitNode(listenPort, addr)
	return nil
}

func (m *Manager) startClientLocked(ctx context.Context, cfg *identity.VPNConfig) error {
	if cfg.PeerDevice == "" {
		return errors.New("client mode requires peer_device")
	}
	kp, err := LoadOrCreatePrivateKey()
	if err != nil {
		return err
	}
	listenPort := cfg.ListenPort
	if listenPort <= 0 {
		listenPort = DefaultListenPort
	}

	addr, err := m.resolver.RegisterVPN(ctx, kp.PublicKey, string(RoleClient), listenPort, "")
	if err != nil {
		return fmt.Errorf("register vpn with cloud: %w", err)
	}
	registered := true
	defer func() {
		if registered {
			_ = m.resolver.DeregisterVPN(ctx)
		}
	}()

	peer, err := m.resolver.GetPeer(ctx, cfg.PeerDevice)
	if err != nil {
		return fmt.Errorf("fetch peer info: %w", err)
	}
	if peer.Endpoint == "" {
		return fmt.Errorf("peer %q has no public endpoint yet — make sure the exit node is reachable (port-forwarded) and has run `beacon vpn enable`", cfg.PeerDevice)
	}

	wg, err := createWGDevice(InterfaceName, kp.PrivateKey, listenPort)
	if err != nil {
		return err
	}

	if err := applyClientNetwork(InterfaceName, addr); err != nil {
		wg.close()
		return err
	}

	if err := wg.configurePeer(peer.PublicKey, peer.Endpoint, peer.VPNAddress+"/32", 25*time.Second); err != nil {
		wg.close()
		teardownNetwork(InterfaceName, "")
		return err
	}

	registered = false // success — keep the registration
	m.wg = wg
	m.keyPair = kp
	m.cfg = cfg
	m.status = Status{
		Enabled:       true,
		Role:          RoleClient,
		InterfaceName: InterfaceName,
		VPNAddress:    addr,
		ListenPort:    listenPort,
		PublicKey:     kp.PublicKey,
		PeerDevice:    cfg.PeerDevice,
		PeerEndpoint:  peer.Endpoint,
	}
	_ = identity.SetVPNClient(cfg.PeerDevice, addr)
	return nil
}

func (m *Manager) refreshClientPeerLocked(ctx context.Context) error {
	if m.wg == nil || m.cfg == nil || m.cfg.PeerDevice == "" {
		return nil
	}
	peer, err := m.resolver.GetPeer(ctx, m.cfg.PeerDevice)
	if err != nil {
		return err
	}
	if peer.Endpoint == "" || peer.Endpoint == m.status.PeerEndpoint {
		return nil
	}
	if err := m.wg.configurePeer(peer.PublicKey, peer.Endpoint, peer.VPNAddress+"/32", 25*time.Second); err != nil {
		return err
	}
	m.status.PeerEndpoint = peer.Endpoint
	return nil
}

func (m *Manager) shutdownLocked() {
	hadDevice := m.wg != nil
	if m.wg != nil {
		m.wg.close()
	}
	if hadDevice {
		teardownNetwork(InterfaceName, m.egress)
	}
	if m.resolver != nil && m.cfg != nil {
		_ = m.resolver.DeregisterVPN(context.Background())
	}
	m.wg = nil
	m.keyPair = nil
	m.egress = ""
	m.cfg = nil
	m.status = Status{}
}

// Stop is the master-shutdown entry point. Same as Reconcile(nil) but doesn't
// require a context.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdownLocked()
}
