package vpn

import (
	"context"
	"errors"
	"sync"
	"testing"

	"beacon/internal/identity"

	"github.com/stretchr/testify/require"
)

// mockResolver is a hand-rolled stub for PeerResolver — counts calls and lets
// each test program the responses it cares about.
type mockResolver struct {
	mu sync.Mutex

	registerErr  error
	registerAddr string
	registerN    int
	lastRegister vpnRegisterCall

	getPeerErr error
	getPeer    *PeerInfo
	getPeerN   int

	deregisterErr error
	deregisterN   int
}

type vpnRegisterCall struct {
	publicKey  string
	role       string
	listenPort int
	endpoint   string
}

func (m *mockResolver) RegisterVPN(_ context.Context, publicKey, role string, listenPort int, endpoint string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registerN++
	m.lastRegister = vpnRegisterCall{publicKey: publicKey, role: role, listenPort: listenPort, endpoint: endpoint}
	return m.registerAddr, m.registerErr
}

func (m *mockResolver) GetPeer(_ context.Context, _ string) (*PeerInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getPeerN++
	return m.getPeer, m.getPeerErr
}

func (m *mockResolver) DeregisterVPN(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deregisterN++
	return m.deregisterErr
}

func (m *mockResolver) counts() (reg, get, dereg int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.registerN, m.getPeerN, m.deregisterN
}

// isolateBeaconHomeForManager points BEACON_HOME at a tempdir so the
// LoadOrCreatePrivateKey calls inside startExitNodeLocked / startClientLocked
// don't touch the developer's real ~/.beacon.
func isolateBeaconHomeForManager(t *testing.T) {
	t.Helper()
	t.Setenv("BEACON_HOME", t.TempDir())
}

func TestManager_FreshStatus_disabled(t *testing.T) {
	m := NewManager(&mockResolver{})
	s := m.Status()
	require.False(t, s.Enabled)
	require.Empty(t, s.VPNAddress)
	require.False(t, s.Connected)
}

func TestManager_ReconcileNil_isNoOp(t *testing.T) {
	mr := &mockResolver{}
	m := NewManager(mr)

	require.NoError(t, m.Reconcile(context.Background(), nil))
	reg, get, dereg := mr.counts()
	require.Zero(t, reg, "nil cfg must not call register")
	require.Zero(t, get)
	require.Zero(t, dereg, "no prior cfg → no deregister")
	require.False(t, m.Status().Enabled)
}

func TestManager_ReconcileDisabled_isNoOp(t *testing.T) {
	mr := &mockResolver{}
	m := NewManager(mr)

	cfg := &identity.VPNConfig{Enabled: false, Role: "exit_node"}
	require.NoError(t, m.Reconcile(context.Background(), cfg))
	reg, _, dereg := mr.counts()
	require.Zero(t, reg)
	require.Zero(t, dereg)
}

func TestManager_ReconcileUnknownRole_errorsButShutsDown(t *testing.T) {
	isolateBeaconHomeForManager(t)
	mr := &mockResolver{}
	m := NewManager(mr)

	cfg := &identity.VPNConfig{Enabled: true, Role: "blender"}
	err := m.Reconcile(context.Background(), cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown vpn role")
	// No network attempts at all — the unknown-role check is before any IO.
	reg, get, _ := mr.counts()
	require.Zero(t, reg)
	require.Zero(t, get)
}

func TestManager_ReconcileClient_missingPeerDevice(t *testing.T) {
	isolateBeaconHomeForManager(t)
	mr := &mockResolver{}
	m := NewManager(mr)

	cfg := &identity.VPNConfig{Enabled: true, Role: "client"}
	err := m.Reconcile(context.Background(), cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "peer_device")
	// Bailed out before touching the cloud or generating keys.
	reg, get, _ := mr.counts()
	require.Zero(t, reg)
	require.Zero(t, get)
}

func TestManager_ReconcileExitNode_resolverErrorPropagates(t *testing.T) {
	isolateBeaconHomeForManager(t)
	mr := &mockResolver{registerErr: errors.New("network down")}
	m := NewManager(mr)

	cfg := &identity.VPNConfig{Enabled: true, Role: "exit_node", ListenPort: 0}
	err := m.Reconcile(context.Background(), cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "register vpn with cloud")
	require.Contains(t, err.Error(), "network down")

	reg, get, _ := mr.counts()
	require.Equal(t, 1, reg, "register should have been attempted exactly once")
	require.Zero(t, get, "GetPeer is irrelevant for exit-node mode")

	// Verify the agent sent the right shape: default port + correct role.
	require.Equal(t, "exit_node", mr.lastRegister.role)
	require.Equal(t, DefaultListenPort, mr.lastRegister.listenPort)
	require.NotEmpty(t, mr.lastRegister.publicKey, "public key should be set from generated KeyPair")
	// Public key must be the base64 form parseable by EnsureBase64.
	require.NoError(t, EnsureBase64(mr.lastRegister.publicKey))
}

func TestManager_ReconcileClient_resolverErrorPropagates(t *testing.T) {
	isolateBeaconHomeForManager(t)
	mr := &mockResolver{registerErr: errors.New("auth failed")}
	m := NewManager(mr)

	cfg := &identity.VPNConfig{Enabled: true, Role: "client", PeerDevice: "home-pi"}
	err := m.Reconcile(context.Background(), cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "register vpn with cloud")

	reg, get, _ := mr.counts()
	require.Equal(t, 1, reg)
	require.Zero(t, get, "GetPeer must not run after RegisterVPN fails")
	require.Equal(t, "client", mr.lastRegister.role)
}

func TestManager_ReconcileClient_getPeerErrorPropagates(t *testing.T) {
	isolateBeaconHomeForManager(t)
	mr := &mockResolver{
		registerAddr: "10.13.37.5",
		getPeerErr:   errors.New("peer not found"),
	}
	m := NewManager(mr)

	cfg := &identity.VPNConfig{Enabled: true, Role: "client", PeerDevice: "home-pi"}
	err := m.Reconcile(context.Background(), cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "fetch peer info")
	require.Contains(t, err.Error(), "peer not found")

	reg, get, _ := mr.counts()
	require.Equal(t, 1, reg)
	require.Equal(t, 1, get, "GetPeer must run after a successful RegisterVPN")
}

func TestManager_ReconcileClient_emptyPeerEndpointRejected(t *testing.T) {
	isolateBeaconHomeForManager(t)
	mr := &mockResolver{
		registerAddr: "10.13.37.5",
		getPeer: &PeerInfo{
			DeviceName: "home-pi",
			PublicKey:  "AAAA", // not parsed yet — endpoint check runs first
			Endpoint:   "",     // peer hasn't reported one — Phase 1 needs an endpoint
			VPNAddress: "10.13.37.2",
		},
	}
	m := NewManager(mr)

	cfg := &identity.VPNConfig{Enabled: true, Role: "client", PeerDevice: "home-pi"}
	err := m.Reconcile(context.Background(), cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no public endpoint")

	reg, get, _ := mr.counts()
	require.Equal(t, 1, reg)
	require.Equal(t, 1, get)
}

func TestManager_StopOnFreshManager_isSafe(t *testing.T) {
	m := NewManager(&mockResolver{})
	require.NotPanics(t, func() { m.Stop() })
	require.False(t, m.Status().Enabled)
}
