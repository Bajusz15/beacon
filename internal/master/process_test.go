package master

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"beacon/internal/identity"
)

func TestNewProcessManager(t *testing.T) {
	ctx := context.Background()
	pm, err := NewProcessManager(ctx)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}
	if pm == nil {
		t.Fatal("ProcessManager is nil")
	}
	if pm.ipcBase == "" {
		t.Error("IPC base dir is empty")
	}
}

func TestProcessManager_isProjectEnabled(t *testing.T) {
	ctx := context.Background()
	pm, _ := NewProcessManager(ctx)

	tests := []struct {
		name     string
		project  identity.ProjectConfig
		expected bool
	}{
		{
			name:     "valid project",
			project:  identity.ProjectConfig{ID: "test", ConfigPath: "/path/to/config.yml"},
			expected: true,
		},
		{
			name:     "missing ID",
			project:  identity.ProjectConfig{ConfigPath: "/path/to/config.yml"},
			expected: false,
		},
		{
			name:     "missing config path",
			project:  identity.ProjectConfig{ID: "test"},
			expected: false,
		},
		{
			name:     "empty project",
			project:  identity.ProjectConfig{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pm.isProjectEnabled(tt.project)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestProcessManager_SpawnAndShutdown(t *testing.T) {
	// Create a test config file
	dir := t.TempDir()
	configPath := filepath.Join(dir, "monitor.yml")
	configContent := `
device:
  name: "test-device"
checks:
  - name: "test-check"
    type: command
    cmd: "sleep 60"
    interval: 30s
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pm, err := NewProcessManager(ctx)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	project := identity.ProjectConfig{
		ID:         "test-project",
		ConfigPath: configPath,
	}

	// Spawn child
	if err := pm.Spawn(project); err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	// Verify child is tracked
	children := pm.GetChildren()
	if len(children) != 1 {
		t.Errorf("expected 1 child, got %d", len(children))
	}
	child, ok := children["test-project"]
	if !ok {
		t.Fatal("child not found in map")
	}
	if child.ProjectID != "test-project" {
		t.Errorf("project ID mismatch: got %s", child.ProjectID)
	}

	// Verify IPC directory was created
	ipcDir := filepath.Join(pm.ipcBase, "test-project")
	if _, err := os.Stat(ipcDir); os.IsNotExist(err) {
		t.Error("IPC directory was not created")
	}

	// Shutdown
	pm.Shutdown()

	// Give some time for cleanup
	time.Sleep(100 * time.Millisecond)
}

func TestProcessManager_SpawnDuplicate(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "monitor.yml")
	configContent := `
device:
  name: "test-device"
checks: []
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pm, _ := NewProcessManager(ctx)
	defer pm.Shutdown()

	project := identity.ProjectConfig{
		ID:         "test-project",
		ConfigPath: configPath,
	}

	// First spawn should succeed
	if err := pm.Spawn(project); err != nil {
		t.Fatalf("first Spawn failed: %v", err)
	}

	// Second spawn should fail (already running)
	if err := pm.Spawn(project); err == nil {
		t.Error("expected error for duplicate spawn")
	}
}

func TestProcessManager_SpawnAll(t *testing.T) {
	dir := t.TempDir()

	// Create two config files
	config1 := filepath.Join(dir, "project1.yml")
	config2 := filepath.Join(dir, "project2.yml")
	configContent := `
device:
  name: "test"
checks: []
`
	os.WriteFile(config1, []byte(configContent), 0644)
	os.WriteFile(config2, []byte(configContent), 0644)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pm, _ := NewProcessManager(ctx)
	defer pm.Shutdown()

	projects := []identity.ProjectConfig{
		{ID: "project1", ConfigPath: config1},
		{ID: "project2", ConfigPath: config2},
		{ID: "", ConfigPath: config1}, // Invalid - should be skipped
	}

	pm.SpawnAll(projects)

	children := pm.GetChildren()
	if len(children) != 2 {
		t.Errorf("expected 2 children, got %d", len(children))
	}
}

func TestProcessManager_GetIPCReaders(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "monitor.yml")
	configContent := `
device:
  name: "test"
checks: []
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pm, _ := NewProcessManager(ctx)
	defer pm.Shutdown()

	pm.Spawn(identity.ProjectConfig{ID: "test-project", ConfigPath: configPath})

	readers := pm.GetIPCReaders()
	if len(readers) != 1 {
		t.Errorf("expected 1 reader, got %d", len(readers))
	}
	if _, ok := readers["test-project"]; !ok {
		t.Error("reader for test-project not found")
	}
}
