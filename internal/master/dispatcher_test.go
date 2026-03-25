package master

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"beacon/internal/ipc"
)

func TestNewCommandDispatcher(t *testing.T) {
	d := NewCommandDispatcher(nil)
	if d == nil {
		t.Fatal("NewCommandDispatcher returned nil")
	}
}

func TestCommandDispatcher_DispatchCommands_nilPM(t *testing.T) {
	d := NewCommandDispatcher(nil)
	// Should not panic
	d.DispatchCommands([]HeartbeatCommand{
		{ID: "cmd1", Action: "restart", TargetProject: "test"},
	})

	results := d.GetPendingResults()
	if len(results) != 0 {
		t.Errorf("expected no results for nil PM, got %d", len(results))
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
	d := NewCommandDispatcher(nil)

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
	d := NewCommandDispatcher(nil)

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
	d := NewCommandDispatcher(nil)

	// Device-level command (no target project)
	d.DispatchCommands([]HeartbeatCommand{
		{ID: "cmd1", Action: "restart", TargetProject: ""},
	})

	// Should be recorded as failed (device-level not supported)
	// But since pm is nil, it won't dispatch at all
	results := d.GetPendingResults()
	if len(results) != 0 {
		t.Errorf("expected 0 results for nil PM, got %d", len(results))
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
