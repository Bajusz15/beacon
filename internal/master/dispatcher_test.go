package master

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"beacon/internal/identity"
	"beacon/internal/ipc"

	"github.com/stretchr/testify/require"
)

func TestNewCommandDispatcher(t *testing.T) {
	d := NewCommandDispatcher(nil, nil)
	if d == nil {
		t.Fatal("NewCommandDispatcher returned nil")
	}
}

func TestCommandDispatcher_DispatchCommands_nilPM(t *testing.T) {
	d := NewCommandDispatcher(nil, nil)
	d.DispatchCommands([]HeartbeatCommand{
		{ID: "cmd1", Action: "restart", TargetProject: "test"},
	})

	results := d.GetPendingResults()
	if len(results) != 1 {
		t.Errorf("expected 1 failed result for nil PM, got %d", len(results))
	}
}

func TestCommandDispatcher_DispatchCommands_projectNotFound(t *testing.T) {
	dir := t.TempDir()
	ipcDir := filepath.Join(dir, "ipc", "other-project")
	os.MkdirAll(ipcDir, 0755)

	// Create a mock process manager state by writing to IPC
	d := &CommandDispatcher{
		pm:             nil,
		pendingResults: make([]CommandResultReport, 0),
		seenCommands:   make(map[string]time.Time),
	}

	// Manually record a failed result
	d.recordResult("cmd1", ipc.ResultFailed, "Project not found: missing-project")

	results := d.GetPendingResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != ipc.ResultFailed {
		t.Errorf("expected failed status, got %s", results[0].Status)
	}
}

func TestCommandDispatcher_GetPendingResults_clearsResults(t *testing.T) {
	d := NewCommandDispatcher(nil, nil)

	// Add some results
	d.recordResult("cmd1", ipc.ResultSuccess, "done")
	d.recordResult("cmd2", ipc.ResultFailed, "error")

	// First call should return 2 results
	results1 := d.GetPendingResults()
	if len(results1) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results1))
	}

	// Second call should return 0 (cleared)
	results2 := d.GetPendingResults()
	if len(results2) != 0 {
		t.Errorf("expected 0 results after clear, got %d", len(results2))
	}
}

func TestCommandDispatcher_recordResult(t *testing.T) {
	d := NewCommandDispatcher(nil, nil)

	beforeTime := time.Now()
	d.recordResult("cmd123", ipc.ResultSuccess, "Operation completed")
	afterTime := time.Now()

	results := d.GetPendingResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.CommandID != "cmd123" {
		t.Errorf("command ID mismatch: got %s", r.CommandID)
	}
	if r.Status != ipc.ResultSuccess {
		t.Errorf("status mismatch: got %s", r.Status)
	}
	if r.Message != "Operation completed" {
		t.Errorf("message mismatch: got %s", r.Message)
	}
	if r.Timestamp.Before(beforeTime) || r.Timestamp.After(afterTime) {
		t.Errorf("timestamp out of range: %v", r.Timestamp)
	}
}

func TestCommandDispatcher_DispatchCommands_deviceLevel(t *testing.T) {
	d := NewCommandDispatcher(nil, nil)

	d.DispatchCommands([]HeartbeatCommand{
		{ID: "cmd1", Action: "restart", TargetProject: ""},
	})

	results := d.GetPendingResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 result for nil PM, got %d", len(results))
	}
	if results[0].Status != ipc.ResultFailed {
		t.Errorf("expected failed status, got %s", results[0].Status)
	}
}

func TestHeartbeatCommand_structure(t *testing.T) {
	cmd := HeartbeatCommand{
		ID:            "cmd_abc123",
		Action:        "restart",
		TargetProject: "my-project",
		Payload:       map[string]any{"lines": 100},
	}

	if cmd.ID != "cmd_abc123" {
		t.Errorf("ID mismatch")
	}
	if cmd.Action != "restart" {
		t.Errorf("action mismatch")
	}
	if cmd.TargetProject != "my-project" {
		t.Errorf("target project mismatch")
	}
}

func TestCommandResultReport_structure(t *testing.T) {
	now := time.Now()
	r := CommandResultReport{
		CommandID: "cmd_xyz",
		Status:    ipc.ResultSuccess,
		Message:   "Done",
		Timestamp: now,
	}

	if r.CommandID != "cmd_xyz" {
		t.Errorf("command ID mismatch")
	}
	if r.Status != ipc.ResultSuccess {
		t.Errorf("status mismatch")
	}
	if r.Timestamp != now {
		t.Errorf("timestamp mismatch")
	}
}

// --- New tests for dedup, allowlist, VPN commands ---

func TestDispatcher_UnknownActionAllowedByDefault(t *testing.T) {
	d := NewCommandDispatcher(nil, nil)
	d.DispatchCommands([]HeartbeatCommand{
		{ID: "cmd1", Action: "custom_action", TargetProject: "myapp"},
	})

	results := d.GetPendingResults()
	require.Len(t, results, 1)
	// Fails because PM is nil, NOT because the action is blocked.
	require.Equal(t, ipc.ResultFailed, results[0].Status)
	require.Contains(t, results[0].Message, "Process manager not available")
}

func TestDispatcher_UnknownActionRejectedByOverride(t *testing.T) {
	d := NewCommandDispatcher(nil, nil)
	d.SetAllowedActions([]string{"health_check"})

	d.DispatchCommands([]HeartbeatCommand{
		{ID: "cmd1", Action: "custom_action", TargetProject: "myapp"},
	})

	results := d.GetPendingResults()
	require.Len(t, results, 1)
	require.Equal(t, ipc.ResultFailed, results[0].Status)
	require.Contains(t, results[0].Message, "not allowed")
}

func TestDispatcher_DuplicateCommandRejected(t *testing.T) {
	d := NewCommandDispatcher(nil, nil)

	// First dispatch — accepted (will fail because pm is nil, but passes allowlist + dedup).
	d.DispatchCommands([]HeartbeatCommand{
		{ID: "cmd_dup", Action: "restart", TargetProject: "myapp"},
	})
	_ = d.GetPendingResults() // drain

	// Second dispatch with same ID — should be silently skipped.
	d.DispatchCommands([]HeartbeatCommand{
		{ID: "cmd_dup", Action: "restart", TargetProject: "myapp"},
	})
	results := d.GetPendingResults()
	require.Empty(t, results, "duplicate command should produce no result")
}

func TestDispatcher_EmptyIDNotDeduped(t *testing.T) {
	d := NewCommandDispatcher(nil, nil)

	// Commands with empty ID should still be dispatched every time.
	d.DispatchCommands([]HeartbeatCommand{
		{ID: "", Action: "restart", TargetProject: "myapp"},
	})
	d.DispatchCommands([]HeartbeatCommand{
		{ID: "", Action: "restart", TargetProject: "myapp"},
	})
	results := d.GetPendingResults()
	require.Len(t, results, 2, "empty-ID commands should not be deduplicated")
}

func TestDispatcher_AllowedOverride(t *testing.T) {
	d := NewCommandDispatcher(nil, nil)

	// By default, "restart" is allowed.
	require.True(t, d.isAllowed("restart"))

	// Override with a restricted list.
	d.SetAllowedActions([]string{"health_check", "fetch_logs"})
	require.False(t, d.isAllowed("restart"), "restart should be blocked by override")
	require.True(t, d.isAllowed("health_check"))
	require.True(t, d.isAllowed("fetch_logs"))

	// Revert to defaults.
	d.SetAllowedActions(nil)
	require.True(t, d.isAllowed("restart"), "should revert to default allowlist")
}

func TestDispatcher_VPNEnable(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("BEACON_HOME", "")

	d := NewCommandDispatcher(nil, nil)
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
}

func TestDispatcher_VPNUse(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("BEACON_HOME", "")

	d := NewCommandDispatcher(nil, nil)
	d.DispatchCommands([]HeartbeatCommand{
		{ID: "vpn2", Action: "vpn_use", Payload: map[string]any{"peer_device": "home-pi"}},
	})

	results := d.GetPendingResults()
	require.Len(t, results, 1)
	require.Equal(t, ipc.ResultSuccess, results[0].Status)

	uc, err := identity.LoadUserConfig()
	require.NoError(t, err)
	require.NotNil(t, uc.VPN)
	require.Equal(t, "client", uc.VPN.Role)
	require.Equal(t, "home-pi", uc.VPN.PeerDevice)
}

func TestDispatcher_VPNUse_missingPeerDevice(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("BEACON_HOME", "")

	d := NewCommandDispatcher(nil, nil)
	d.DispatchCommands([]HeartbeatCommand{
		{ID: "vpn3", Action: "vpn_use", Payload: map[string]any{}},
	})

	results := d.GetPendingResults()
	require.Len(t, results, 1)
	require.Equal(t, ipc.ResultFailed, results[0].Status)
	require.Contains(t, results[0].Message, "peer_device")
}

func TestDispatcher_VPNDisable(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("BEACON_HOME", "")

	// First enable, then disable.
	d := NewCommandDispatcher(nil, nil)
	d.DispatchCommands([]HeartbeatCommand{
		{ID: "vpn4a", Action: "vpn_enable"},
	})
	_ = d.GetPendingResults()

	d.DispatchCommands([]HeartbeatCommand{
		{ID: "vpn4b", Action: "vpn_disable"},
	})
	results := d.GetPendingResults()
	require.Len(t, results, 1)
	require.Equal(t, ipc.ResultSuccess, results[0].Status)

	uc, err := identity.LoadUserConfig()
	require.NoError(t, err)
	require.Nil(t, uc.VPN, "VPN config should be cleared after vpn_disable")
}

func TestDispatcher_VPNBlockedByAllowlist(t *testing.T) {
	d := NewCommandDispatcher(nil, nil)
	d.SetAllowedActions([]string{"health_check"})

	d.DispatchCommands([]HeartbeatCommand{
		{ID: "vpn5", Action: "vpn_enable"},
	})

	results := d.GetPendingResults()
	require.Len(t, results, 1)
	require.Equal(t, ipc.ResultFailed, results[0].Status)
	require.Contains(t, results[0].Message, "not allowed")
}
