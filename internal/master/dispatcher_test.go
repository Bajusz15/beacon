package master

import (
	"testing"
	"time"

	"beacon/internal/identity"
	"beacon/internal/ipc"

	"github.com/stretchr/testify/require"
)

func TestCommandDispatcher_recordResult(t *testing.T) {
	d := NewCommandDispatcher(nil, nil)

	before := time.Now()
	d.recordResult("cmd123", ipc.ResultSuccess, "Operation completed")
	after := time.Now()

	results := d.GetPendingResults()
	require.Len(t, results, 1)

	r := results[0]
	require.Equal(t, "cmd123", r.CommandID)
	require.Equal(t, ipc.ResultSuccess, r.Status)
	require.Equal(t, "Operation completed", r.Message)
	require.False(t, r.Timestamp.Before(before))
	require.False(t, r.Timestamp.After(after))
}

func TestCommandDispatcher_GetPendingResults(t *testing.T) {
	d := NewCommandDispatcher(nil, nil)
	d.recordResult("cmd1", ipc.ResultSuccess, "done")
	d.recordResult("cmd2", ipc.ResultFailed, "error")

	t.Run("returns all results", func(t *testing.T) {
		results := d.GetPendingResults()
		require.Len(t, results, 2)
	})

	t.Run("clears after read", func(t *testing.T) {
		results := d.GetPendingResults()
		require.Empty(t, results)
	})
}

func TestCommandDispatcher_Dispatch(t *testing.T) {
	t.Run("nil PM fails gracefully", func(t *testing.T) {
		d := NewCommandDispatcher(nil, nil)
		d.DispatchCommands([]HeartbeatCommand{
			{ID: "cmd1", Action: "restart", TargetProject: "test"},
		})
		results := d.GetPendingResults()
		require.Len(t, results, 1)
		require.Equal(t, ipc.ResultFailed, results[0].Status)
	})

	t.Run("empty target project rejected", func(t *testing.T) {
		d := NewCommandDispatcher(nil, nil)
		d.DispatchCommands([]HeartbeatCommand{
			{ID: "cmd1", Action: "restart", TargetProject: ""},
		})
		results := d.GetPendingResults()
		require.Len(t, results, 1)
		require.Equal(t, ipc.ResultFailed, results[0].Status)
	})

	t.Run("unknown action allowed by default", func(t *testing.T) {
		d := NewCommandDispatcher(nil, nil)
		d.DispatchCommands([]HeartbeatCommand{
			{ID: "cmd1", Action: "custom_action", TargetProject: "myapp"},
		})
		results := d.GetPendingResults()
		require.Len(t, results, 1)
		require.Equal(t, ipc.ResultFailed, results[0].Status)
		require.Contains(t, results[0].Message, "Process manager not available")
	})

	t.Run("unknown action rejected by override", func(t *testing.T) {
		d := NewCommandDispatcher(nil, nil)
		d.SetAllowedActions([]string{"health_check"})
		d.DispatchCommands([]HeartbeatCommand{
			{ID: "cmd1", Action: "custom_action", TargetProject: "myapp"},
		})
		results := d.GetPendingResults()
		require.Len(t, results, 1)
		require.Contains(t, results[0].Message, "not allowed")
	})
}

func TestCommandDispatcher_Dedup(t *testing.T) {
	d := NewCommandDispatcher(nil, nil)

	t.Run("duplicate command skipped", func(t *testing.T) {
		d.DispatchCommands([]HeartbeatCommand{
			{ID: "cmd_dup", Action: "restart", TargetProject: "myapp"},
		})
		_ = d.GetPendingResults()

		d.DispatchCommands([]HeartbeatCommand{
			{ID: "cmd_dup", Action: "restart", TargetProject: "myapp"},
		})
		results := d.GetPendingResults()
		require.Empty(t, results)
	})

	t.Run("empty ID never deduped", func(t *testing.T) {
		d.DispatchCommands([]HeartbeatCommand{
			{ID: "", Action: "restart", TargetProject: "myapp"},
		})
		d.DispatchCommands([]HeartbeatCommand{
			{ID: "", Action: "restart", TargetProject: "myapp"},
		})
		results := d.GetPendingResults()
		require.Len(t, results, 2)
	})
}

func TestCommandDispatcher_Allowlist(t *testing.T) {
	d := NewCommandDispatcher(nil, nil)

	t.Run("default allows everything", func(t *testing.T) {
		require.True(t, d.isAllowed("restart"))
		require.True(t, d.isAllowed("anything_goes"))
	})

	t.Run("override restricts", func(t *testing.T) {
		d.SetAllowedActions([]string{"health_check", "fetch_logs"})
		require.False(t, d.isAllowed("restart"))
		require.True(t, d.isAllowed("health_check"))
		require.True(t, d.isAllowed("fetch_logs"))
	})

	t.Run("nil reverts to default", func(t *testing.T) {
		d.SetAllowedActions(nil)
		require.True(t, d.isAllowed("restart"))
	})
}

func TestCommandDispatcher_VPN(t *testing.T) {
	setup := func(t *testing.T) *CommandDispatcher {
		t.Helper()
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("BEACON_HOME", "")
		return NewCommandDispatcher(nil, nil)
	}

	t.Run("enable", func(t *testing.T) {
		d := setup(t)
		d.DispatchCommands([]HeartbeatCommand{
			{ID: "vpn1", Action: "vpn_enable", Payload: map[string]any{"listen_port": float64(41820)}},
		})
		results := d.GetPendingResults()
		require.Len(t, results, 1)
		require.Equal(t, ipc.ResultSuccess, results[0].Status)

		uc, err := identity.LoadUserConfig()
		require.NoError(t, err)
		require.NotNil(t, uc.VPN)
		require.True(t, uc.VPN.Enabled)
		require.Equal(t, "exit_node", uc.VPN.Role)
		require.Equal(t, 41820, uc.VPN.ListenPort)
	})

	t.Run("use", func(t *testing.T) {
		d := setup(t)
		d.DispatchCommands([]HeartbeatCommand{
			{ID: "vpn2", Action: "vpn_use", Payload: map[string]any{"peer_device": "home-pi"}},
		})
		results := d.GetPendingResults()
		require.Len(t, results, 1)
		require.Equal(t, ipc.ResultSuccess, results[0].Status)

		uc, err := identity.LoadUserConfig()
		require.NoError(t, err)
		require.Equal(t, "client", uc.VPN.Role)
		require.Equal(t, "home-pi", uc.VPN.PeerDevice)
	})

	t.Run("use missing peer_device", func(t *testing.T) {
		d := setup(t)
		d.DispatchCommands([]HeartbeatCommand{
			{ID: "vpn3", Action: "vpn_use", Payload: map[string]any{}},
		})
		results := d.GetPendingResults()
		require.Len(t, results, 1)
		require.Equal(t, ipc.ResultFailed, results[0].Status)
		require.Contains(t, results[0].Message, "peer_device")
	})

	t.Run("disable", func(t *testing.T) {
		d := setup(t)
		d.DispatchCommands([]HeartbeatCommand{{ID: "a", Action: "vpn_enable"}})
		_ = d.GetPendingResults()

		d.DispatchCommands([]HeartbeatCommand{{ID: "b", Action: "vpn_disable"}})
		results := d.GetPendingResults()
		require.Len(t, results, 1)
		require.Equal(t, ipc.ResultSuccess, results[0].Status)

		uc, err := identity.LoadUserConfig()
		require.NoError(t, err)
		require.Nil(t, uc.VPN)
	})

	t.Run("blocked by allowlist", func(t *testing.T) {
		d := setup(t)
		d.SetAllowedActions([]string{"health_check"})
		d.DispatchCommands([]HeartbeatCommand{{ID: "vpn5", Action: "vpn_enable"}})

		results := d.GetPendingResults()
		require.Len(t, results, 1)
		require.Equal(t, ipc.ResultFailed, results[0].Status)
		require.Contains(t, results[0].Message, "not allowed")
	})
}

func TestHeartbeatCommand_structure(t *testing.T) {
	cmd := HeartbeatCommand{
		ID:            "cmd_abc123",
		Action:        "restart",
		TargetProject: "my-project",
		Payload:       map[string]any{"lines": 100},
	}
	require.Equal(t, "cmd_abc123", cmd.ID)
	require.Equal(t, "restart", cmd.Action)
	require.Equal(t, "my-project", cmd.TargetProject)
}

func TestCommandResultReport_structure(t *testing.T) {
	now := time.Now()
	r := CommandResultReport{
		CommandID: "cmd_xyz",
		Status:    ipc.ResultSuccess,
		Message:   "Done",
		Timestamp: now,
	}
	require.Equal(t, "cmd_xyz", r.CommandID)
	require.Equal(t, ipc.ResultSuccess, r.Status)
	require.Equal(t, now, r.Timestamp)
}
