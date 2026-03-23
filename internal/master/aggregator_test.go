package master

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"beacon/internal/ipc"
)

func TestAggregateProjectHealth_nilProcessManager(t *testing.T) {
	result := AggregateProjectHealth(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestAggregateProjectHealth_noProjects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pm, _ := NewProcessManager(ctx)
	result := AggregateProjectHealth(pm)
	if result != nil {
		t.Errorf("expected nil for no projects, got %v", result)
	}
}

func TestAggregateProjectHealth_withHealthyProject(t *testing.T) {
	// This test verifies aggregation without spawning real children
	// by manually writing health files to IPC directories

	dir := t.TempDir()
	ipcDir := filepath.Join(dir, "ipc", "test-project")

	// Write health file manually
	writer, _ := ipc.NewWriter(ipcDir)
	report := &ipc.HealthReport{
		ProjectID:     "test-project",
		Timestamp:     time.Now(),
		Status:        ipc.StatusHealthy,
		UptimeSeconds: 100,
		Checks: []ipc.CheckResult{
			{Name: "test-check", Passed: true, LatencyMs: 10},
		},
	}
	writer.WriteHealth(report)

	// Test aggregation from reader
	reader := ipc.NewReader(ipcDir)
	health := aggregateFromReader("test-project", reader)

	if health.ProjectID != "test-project" {
		t.Errorf("project ID mismatch: got %s", health.ProjectID)
	}
	if health.Status != ipc.StatusHealthy {
		t.Errorf("status mismatch: got %s", health.Status)
	}
	if len(health.Checks) != 1 {
		t.Errorf("checks count mismatch: got %d", len(health.Checks))
	}
}

func TestAggregateProjectHealth_staleHealth(t *testing.T) {
	dir := t.TempDir()
	ipcDir := filepath.Join(dir, "ipc", "stale-project")
	os.MkdirAll(ipcDir, 0755)

	// Write a health file with old timestamp
	writer, _ := ipc.NewWriter(ipcDir)
	report := &ipc.HealthReport{
		ProjectID: "stale-project",
		Timestamp: time.Now().Add(-2 * time.Minute), // 2 minutes old
		Status:    ipc.StatusHealthy,
	}
	writer.WriteHealth(report)

	// Manually set the file modification time to be old
	healthPath := filepath.Join(ipcDir, "health.json")
	oldTime := time.Now().Add(-2 * time.Minute)
	os.Chtimes(healthPath, oldTime, oldTime)

	// Create a reader and test
	reader := ipc.NewReader(ipcDir)
	health := aggregateFromReader("stale-project", reader)

	if health.Status != ipc.StatusUnknown {
		t.Errorf("expected unknown status for stale health, got %s", health.Status)
	}
}

func TestAggregateProjectHealth_missingHealthFile(t *testing.T) {
	dir := t.TempDir()
	ipcDir := filepath.Join(dir, "ipc", "missing-project")
	os.MkdirAll(ipcDir, 0755)
	// Don't write any health file

	reader := ipc.NewReader(ipcDir)
	health := aggregateFromReader("missing-project", reader)

	if health.Status != ipc.StatusUnknown {
		t.Errorf("expected unknown status for missing health, got %s", health.Status)
	}
	if health.ProjectID != "missing-project" {
		t.Errorf("project ID mismatch: got %s", health.ProjectID)
	}
}

func TestAggregateFromReader_healthyWithChecks(t *testing.T) {
	dir := t.TempDir()
	ipcDir := filepath.Join(dir, "ipc", "healthy-project")

	writer, _ := ipc.NewWriter(ipcDir)
	report := &ipc.HealthReport{
		ProjectID:     "healthy-project",
		Timestamp:     time.Now(),
		Status:        ipc.StatusHealthy,
		UptimeSeconds: 3600,
		Version:       "v1.2.3",
		Checks: []ipc.CheckResult{
			{Name: "http_check", Passed: true, LatencyMs: 50},
			{Name: "port_check", Passed: true, LatencyMs: 5},
		},
		LogsTail: []string{"log line 1", "log line 2"},
	}
	writer.WriteHealth(report)

	reader := ipc.NewReader(ipcDir)
	health := aggregateFromReader("healthy-project", reader)

	if health.Status != ipc.StatusHealthy {
		t.Errorf("status mismatch: got %s", health.Status)
	}
	if health.UptimeSeconds != 3600 {
		t.Errorf("uptime mismatch: got %d", health.UptimeSeconds)
	}
	if health.Version != "v1.2.3" {
		t.Errorf("version mismatch: got %s", health.Version)
	}
	if len(health.Checks) != 2 {
		t.Errorf("checks count mismatch: got %d", len(health.Checks))
	}
	if len(health.LogsTail) != 2 {
		t.Errorf("logs tail count mismatch: got %d", len(health.LogsTail))
	}
}

func TestAggregateFromReader_degradedStatus(t *testing.T) {
	dir := t.TempDir()
	ipcDir := filepath.Join(dir, "ipc", "degraded-project")

	writer, _ := ipc.NewWriter(ipcDir)
	report := &ipc.HealthReport{
		ProjectID: "degraded-project",
		Timestamp: time.Now(),
		Status:    ipc.StatusDegraded,
		Checks: []ipc.CheckResult{
			{Name: "http_check", Passed: false, LatencyMs: 5000, Error: "timeout"},
			{Name: "port_check", Passed: true, LatencyMs: 5},
		},
	}
	writer.WriteHealth(report)

	reader := ipc.NewReader(ipcDir)
	health := aggregateFromReader("degraded-project", reader)

	if health.Status != ipc.StatusDegraded {
		t.Errorf("status mismatch: got %s", health.Status)
	}
	if len(health.Checks) != 2 {
		t.Errorf("checks count mismatch: got %d", len(health.Checks))
	}
	if health.Checks[0].Passed {
		t.Error("first check should be failed")
	}
}
