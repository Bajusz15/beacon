package master

import (
	"context"
	"testing"

	"beacon/internal/identity"
	"beacon/internal/remoteaccess"

	"github.com/stretchr/testify/require"
)

// gatedUserConfig returns a minimal cloud-enabled config with one tunnel.
func gatedUserConfig() *identity.UserConfig {
	return &identity.UserConfig{
		APIKey:                "usr_test_key",
		DeviceName:            "test-box",
		CloudReportingEnabled: true,
		Tunnels: []identity.TunnelConfig{
			{ID: "homeassistant"},
		},
	}
}

// When a remote-access passphrase is configured, tunnels must NOT auto-start —
// they may come up only via a passphrase-unlocked tunnel_connect. The manager is
// still created so those on-demand connects work.
func TestInitTunnelManager_PassphraseGatesAutoStart(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())
	require.NoError(t, remoteaccess.SetPassphrase("correct horse battery"))

	tm := initTunnelManager(context.Background(), gatedUserConfig())
	require.NotNil(t, tm, "manager should exist so on-demand tunnel_connect can start tunnels after unlock")
	t.Cleanup(tm.Shutdown)

	require.Empty(t, tm.GetTunnelStatuses(), "no tunnel should auto-start while the gate is on")
}

// With no passphrase configured the gate is off: behavior is unchanged and the
// configured tunnel is started (registered) at boot.
func TestInitTunnelManager_NoPassphraseAutoStarts(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())
	require.False(t, remoteaccess.IsConfigured())

	uc := gatedUserConfig()
	uc.Tunnels[0].Upstream = &identity.TunnelUpstream{Protocol: "http", Host: "127.0.0.1", Port: 8123}

	tm := initTunnelManager(context.Background(), uc)
	require.NotNil(t, tm)
	t.Cleanup(tm.Shutdown)

	require.Len(t, tm.GetTunnelStatuses(), 1, "configured tunnel should auto-start when the gate is off")
}
