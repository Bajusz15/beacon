package vpn

import (
	"context"
	"errors"
	"sync"
	"testing"

	"beacon/internal/identity"

	"github.com/stretchr/testify/require"
)

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

func isolateBeaconHomeForManager(t *testing.T) {
	t.Helper()
	t.Setenv("BEACON_HOME", t.TempDir())
}

func TestManager_Status(t *testing.T) {
	m := NewManager(&mockResolver{})
	s := m.Status()

	t.Run("fresh manager is disabled", func(t *testing.T) {
		require.False(t, s.Enabled)
		require.Empty(t, s.VPNAddress)
		require.False(t, s.Connected)
	})

	t.Run("all fields zero", func(t *testing.T) {
		require.Empty(t, s.Role)
		require.Empty(t, s.InterfaceName)
		require.Zero(t, s.ListenPort)
		require.Empty(t, s.PublicKey)
		require.Empty(t, s.PeerDevice)
		require.Empty(t, s.PeerEndpoint)
		require.Zero(t, s.BytesRx)
		require.Zero(t, s.BytesTx)
		require.True(t, s.LastHandshake.IsZero())
	})
}

func TestManager_Stop(t *testing.T) {
	t.Run("fresh manager", func(t *testing.T) {
		m := NewManager(&mockResolver{})
		require.NotPanics(t, func() { m.Stop() })
		require.False(t, m.Status().Enabled)
	})

	t.Run("double stop", func(t *testing.T) {
		m := NewManager(&mockResolver{})
		require.NotPanics(t, func() {
			m.Stop()
			m.Stop()
		})
	})
}

func TestManager_Reconcile(t *testing.T) {
	t.Run("nil config is no-op", func(t *testing.T) {
		mr := &mockResolver{}
		m := NewManager(mr)

		require.NoError(t, m.Reconcile(context.Background(), nil))
		reg, get, dereg := mr.counts()
		require.Zero(t, reg)
		require.Zero(t, get)
		require.Zero(t, dereg)
		require.False(t, m.Status().Enabled)
	})

	t.Run("disabled config is no-op", func(t *testing.T) {
		mr := &mockResolver{}
		m := NewManager(mr)

		require.NoError(t, m.Reconcile(context.Background(), &identity.VPNConfig{Enabled: false, Role: "exit_node"}))
		reg, _, dereg := mr.counts()
		require.Zero(t, reg)
		require.Zero(t, dereg)
	})

	t.Run("repeated disable is no-op", func(t *testing.T) {
		mr := &mockResolver{}
		m := NewManager(mr)

		require.NoError(t, m.Reconcile(context.Background(), nil))
		require.NoError(t, m.Reconcile(context.Background(), &identity.VPNConfig{Enabled: false}))
		require.NoError(t, m.Reconcile(context.Background(), nil))
		reg, _, dereg := mr.counts()
		require.Zero(t, reg)
		require.Zero(t, dereg)
	})

	t.Run("unknown role errors", func(t *testing.T) {
		isolateBeaconHomeForManager(t)
		mr := &mockResolver{}
		m := NewManager(mr)

		err := m.Reconcile(context.Background(), &identity.VPNConfig{Enabled: true, Role: "blender"})
		require.ErrorContains(t, err, "unknown vpn role")
		reg, get, _ := mr.counts()
		require.Zero(t, reg)
		require.Zero(t, get)
	})

	t.Run("nil after failed enable is clean", func(t *testing.T) {
		isolateBeaconHomeForManager(t)
		mr := &mockResolver{registerErr: errors.New("fail")}
		m := NewManager(mr)

		_ = m.Reconcile(context.Background(), &identity.VPNConfig{Enabled: true, Role: "exit_node"})
		require.NoError(t, m.Reconcile(context.Background(), nil))
		require.False(t, m.Status().Enabled)
	})

	t.Run("exit node register error propagates", func(t *testing.T) {
		isolateBeaconHomeForManager(t)
		mr := &mockResolver{registerErr: errors.New("network down")}
		m := NewManager(mr)

		err := m.Reconcile(context.Background(), &identity.VPNConfig{Enabled: true, Role: "exit_node"})
		require.ErrorContains(t, err, "register vpn with cloud")
		require.ErrorContains(t, err, "network down")

		reg, get, _ := mr.counts()
		require.Equal(t, 1, reg)
		require.Zero(t, get)
		require.Equal(t, "exit_node", mr.lastRegister.role)
		require.Equal(t, DefaultListenPort, mr.lastRegister.listenPort)
		require.NotEmpty(t, mr.lastRegister.publicKey)
		require.NoError(t, EnsureBase64(mr.lastRegister.publicKey))
	})

	t.Run("exit node custom listen port", func(t *testing.T) {
		isolateBeaconHomeForManager(t)
		mr := &mockResolver{registerErr: errors.New("offline")}
		m := NewManager(mr)

		_ = m.Reconcile(context.Background(), &identity.VPNConfig{Enabled: true, Role: "exit_node", ListenPort: 41820})
		require.Equal(t, 41820, mr.lastRegister.listenPort)
	})

	t.Run("client missing peer_device", func(t *testing.T) {
		isolateBeaconHomeForManager(t)
		mr := &mockResolver{}
		m := NewManager(mr)

		err := m.Reconcile(context.Background(), &identity.VPNConfig{Enabled: true, Role: "client"})
		require.ErrorContains(t, err, "peer_device")
		reg, get, _ := mr.counts()
		require.Zero(t, reg)
		require.Zero(t, get)
	})

	t.Run("client register error stops before GetPeer", func(t *testing.T) {
		isolateBeaconHomeForManager(t)
		mr := &mockResolver{registerErr: errors.New("auth failed")}
		m := NewManager(mr)

		err := m.Reconcile(context.Background(), &identity.VPNConfig{Enabled: true, Role: "client", PeerDevice: "home-pi"})
		require.ErrorContains(t, err, "register vpn with cloud")

		reg, get, _ := mr.counts()
		require.Equal(t, 1, reg)
		require.Zero(t, get)
		require.Equal(t, "client", mr.lastRegister.role)
	})

	t.Run("client GetPeer error rolls back registration", func(t *testing.T) {
		isolateBeaconHomeForManager(t)
		mr := &mockResolver{registerAddr: "10.13.37.5", getPeerErr: errors.New("peer not found")}
		m := NewManager(mr)

		err := m.Reconcile(context.Background(), &identity.VPNConfig{Enabled: true, Role: "client", PeerDevice: "home-pi"})
		require.ErrorContains(t, err, "fetch peer info")
		require.ErrorContains(t, err, "peer not found")

		reg, get, dereg := mr.counts()
		require.Equal(t, 1, reg)
		require.Equal(t, 1, get)
		require.Equal(t, 1, dereg, "should deregister after post-register failure")
	})

	t.Run("client empty peer endpoint rolls back registration", func(t *testing.T) {
		isolateBeaconHomeForManager(t)
		mr := &mockResolver{
			registerAddr: "10.13.37.5",
			getPeer:      &PeerInfo{DeviceName: "home-pi", PublicKey: "AAAA", Endpoint: "", VPNAddress: "10.13.37.2"},
		}
		m := NewManager(mr)

		err := m.Reconcile(context.Background(), &identity.VPNConfig{Enabled: true, Role: "client", PeerDevice: "home-pi"})
		require.ErrorContains(t, err, "no public endpoint")

		_, _, dereg := mr.counts()
		require.Equal(t, 1, dereg, "should deregister after post-register failure")
	})

	t.Run("register failure does not trigger deregister", func(t *testing.T) {
		isolateBeaconHomeForManager(t)
		mr := &mockResolver{registerErr: errors.New("network down")}
		m := NewManager(mr)

		_ = m.Reconcile(context.Background(), &identity.VPNConfig{Enabled: true, Role: "exit_node"})
		_, _, dereg := mr.counts()
		require.Zero(t, dereg, "should not deregister if register itself failed")
	})
}
