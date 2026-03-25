package ipc

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriter_WriteHealth(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir)
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}

	report := &HealthReport{
		ProjectID:     "test-project",
		Timestamp:     time.Now(),
		Status:        StatusHealthy,
		UptimeSeconds: 3600,
		Checks: []CheckResult{
			{Name: "http_check", Passed: true, LatencyMs: 50},
			{Name: "port_check", Passed: true, LatencyMs: 5},
		},
	}

	if err := w.WriteHealth(report); err != nil {
		t.Fatalf("WriteHealth failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, healthFile)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("health.json was not created")
	}

	// Read it back
	r := NewReader(dir)
	readReport, err := r.ReadHealth()
	if err != nil {
		t.Fatalf("ReadHealth failed: %v", err)
	}

	if readReport.ProjectID != report.ProjectID {
		t.Errorf("ProjectID mismatch: got %s, want %s", readReport.ProjectID, report.ProjectID)
	}
	if readReport.Status != report.Status {
		t.Errorf("Status mismatch: got %s, want %s", readReport.Status, report.Status)
	}
	if len(readReport.Checks) != len(report.Checks) {
		t.Errorf("Checks count mismatch: got %d, want %d", len(readReport.Checks), len(report.Checks))
	}
}

func TestWriter_WriteCommandResult(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir)
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}

	result := &CommandResult{
		CommandID: "cmd_123",
		Status:    ResultSuccess,
		Message:   "Command executed successfully",
		Timestamp: time.Now(),
	}

	if err := w.WriteCommandResult(result); err != nil {
		t.Fatalf("WriteCommandResult failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, commandResultFile)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("command_result.json was not created")
	}
}

func TestWriter_ReadCommand(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir)
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}

	// No command file should return nil, nil
	cmd, err := w.ReadCommand()
	if err != nil {
		t.Fatalf("ReadCommand failed: %v", err)
	}
	if cmd != nil {
		t.Error("Expected nil command when file doesn't exist")
	}

	// Write a command using Reader (simulates master writing)
	r := NewReader(dir)
	testCmd := &Command{
		ID:        "cmd_456",
		Action:    ActionRestart,
		Timestamp: time.Now(),
	}
	if err := r.WriteCommand(testCmd); err != nil {
		t.Fatalf("WriteCommand failed: %v", err)
	}

	// Now read it
	cmd, err = w.ReadCommand()
	if err != nil {
		t.Fatalf("ReadCommand failed: %v", err)
	}
	if cmd == nil {
		t.Fatal("Expected command, got nil")
	}
	if cmd.ID != testCmd.ID {
		t.Errorf("Command ID mismatch: got %s, want %s", cmd.ID, testCmd.ID)
	}
	if cmd.Action != testCmd.Action {
		t.Errorf("Command Action mismatch: got %s, want %s", cmd.Action, testCmd.Action)
	}

	// File should be deleted after read
	if _, err := os.Stat(filepath.Join(dir, commandFile)); !os.IsNotExist(err) {
		t.Error("command.json should be deleted after reading")
	}
}

func TestReader_ReadHealthIfFresh(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir)
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}

	report := &HealthReport{
		ProjectID: "test-project",
		Timestamp: time.Now(),
		Status:    StatusHealthy,
	}

	if err := w.WriteHealth(report); err != nil {
		t.Fatalf("WriteHealth failed: %v", err)
	}

	r := NewReader(dir)

	// Should return report when fresh
	readReport, err := r.ReadHealthIfFresh(time.Minute)
	if err != nil {
		t.Fatalf("ReadHealthIfFresh failed: %v", err)
	}
	if readReport == nil {
		t.Fatal("Expected report, got nil")
	}

	// Should return nil when stale (0 duration means everything is stale)
	readReport, err = r.ReadHealthIfFresh(0)
	if err != nil {
		t.Fatalf("ReadHealthIfFresh failed: %v", err)
	}
	if readReport != nil {
		t.Error("Expected nil for stale report")
	}
}

func TestReader_ReadCommandResult(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir)
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}

	result := &CommandResult{
		CommandID: "cmd_789",
		Status:    ResultSuccess,
		Message:   "Done",
		Timestamp: time.Now(),
	}

	if err := w.WriteCommandResult(result); err != nil {
		t.Fatalf("WriteCommandResult failed: %v", err)
	}

	r := NewReader(dir)
	readResult, err := r.ReadCommandResult()
	if err != nil {
		t.Fatalf("ReadCommandResult failed: %v", err)
	}
	if readResult == nil {
		t.Fatal("Expected result, got nil")
	}
	if readResult.CommandID != result.CommandID {
		t.Errorf("CommandID mismatch: got %s, want %s", readResult.CommandID, result.CommandID)
	}

	// File should be deleted after read
	if _, err := os.Stat(filepath.Join(dir, commandResultFile)); !os.IsNotExist(err) {
		t.Error("command_result.json should be deleted after reading")
	}
}

func TestIPCDir(t *testing.T) {
	dir, err := IPCDir()
	if err != nil {
		t.Fatalf("IPCDir failed: %v", err)
	}
	if dir == "" {
		t.Fatal("IPCDir returned empty string")
	}
	if !filepath.IsAbs(dir) {
		t.Error("IPCDir should return absolute path")
	}
}

func TestProjectIPCDir(t *testing.T) {
	dir, err := ProjectIPCDir("my-project")
	if err != nil {
		t.Fatalf("ProjectIPCDir failed: %v", err)
	}
	if dir == "" {
		t.Fatal("ProjectIPCDir returned empty string")
	}
	if !filepath.IsAbs(dir) {
		t.Error("ProjectIPCDir should return absolute path")
	}
	if filepath.Base(dir) != "my-project" {
		t.Errorf("ProjectIPCDir should end with project ID, got %s", dir)
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	data := map[string]string{"key": "value"}
	if err := atomicWriteJSON(path, data); err != nil {
		t.Fatalf("atomicWriteJSON failed: %v", err)
	}

	// Verify file exists and tmp is cleaned up
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("File was not created")
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("Temp file should not exist after successful write")
	}
}
