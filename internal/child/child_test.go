package child

import (
	"beacon/internal/ipc"
	"beacon/internal/monitor"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew_validation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr string
	}{
		{
			name:    "missing project-id",
			cfg:     &Config{ConfigPath: "/tmp/test.yml", IPCDir: "/tmp/ipc"},
			wantErr: "project-id is required",
		},
		{
			name:    "missing config path",
			cfg:     &Config{ProjectID: "test", IPCDir: "/tmp/ipc"},
			wantErr: "config path is required",
		},
		{
			name:    "missing ipc-dir",
			cfg:     &Config{ProjectID: "test", ConfigPath: "/tmp/test.yml"},
			wantErr: "ipc-dir is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.cfg)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("error mismatch: got %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestNew_loadConfig(t *testing.T) {
	// Create a temporary config file
	dir := t.TempDir()
	configPath := filepath.Join(dir, "monitor.yml")
	ipcDir := filepath.Join(dir, "ipc")

	configContent := `
device:
  name: "test-device"

checks:
  - name: "test-http"
    type: http
    url: "http://localhost:9999/health"
    interval: 30s
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg := &Config{
		ProjectID:  "test-project",
		ConfigPath: configPath,
		IPCDir:     ipcDir,
	}

	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if c.monitorCfg.Device.Name != "test-device" {
		t.Errorf("device name mismatch: got %s, want test-device", c.monitorCfg.Device.Name)
	}
	if len(c.monitorCfg.Checks) != 1 {
		t.Errorf("checks count mismatch: got %d, want 1", len(c.monitorCfg.Checks))
	}
}

func TestExecuteHTTPCheck_success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := &Child{
		ctx: context.Background(),
	}

	check := monitor.CheckConfig{
		Name: "test-http",
		Type: "http",
		URL:  server.URL,
	}

	result := c.executeHTTPCheck(check)
	if !result.Passed {
		t.Errorf("expected check to pass, got error: %s", result.Error)
	}
}

func TestExecuteHTTPCheck_failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := &Child{
		ctx: context.Background(),
	}

	check := monitor.CheckConfig{
		Name: "test-http",
		Type: "http",
		URL:  server.URL,
	}

	result := c.executeHTTPCheck(check)
	if result.Passed {
		t.Error("expected check to fail")
	}
}

func TestExecuteHTTPCheck_expectStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted) // 202
	}))
	defer server.Close()

	c := &Child{
		ctx: context.Background(),
	}

	check := monitor.CheckConfig{
		Name:         "test-http",
		Type:         "http",
		URL:          server.URL,
		ExpectStatus: 202,
	}

	result := c.executeHTTPCheck(check)
	if !result.Passed {
		t.Errorf("expected check to pass with expected status, got error: %s", result.Error)
	}
}

func TestExecutePortCheck_success(t *testing.T) {
	// Create a listener on a random port
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	// Extract host and port from server URL
	// server.URL is like "http://127.0.0.1:12345"
	host := "127.0.0.1"
	var port int
	_, _ = host, port
	// Parse the URL to get port
	u := server.URL
	for i := len(u) - 1; i >= 0; i-- {
		if u[i] == ':' {
			portStr := u[i+1:]
			for _, c := range portStr {
				port = port*10 + int(c-'0')
			}
			break
		}
	}

	c := &Child{
		ctx: context.Background(),
	}

	check := monitor.CheckConfig{
		Name: "test-port",
		Type: "port",
		Host: host,
		Port: port,
	}

	result := c.executePortCheck(check)
	if !result.Passed {
		t.Errorf("expected port check to pass, got error: %s", result.Error)
	}
}

func TestExecuteCommandCheck_success(t *testing.T) {
	c := &Child{
		ctx: context.Background(),
	}

	check := monitor.CheckConfig{
		Name: "test-cmd",
		Type: "command",
		Cmd:  "echo hello",
	}

	result := c.executeCommandCheck(check)
	if !result.Passed {
		t.Errorf("expected command check to pass, got error: %s", result.Error)
	}
}

func TestExecuteCommandCheck_failure(t *testing.T) {
	c := &Child{
		ctx: context.Background(),
	}

	check := monitor.CheckConfig{
		Name: "test-cmd",
		Type: "command",
		Cmd:  "exit 1",
	}

	result := c.executeCommandCheck(check)
	if result.Passed {
		t.Error("expected command check to fail")
	}
}

func TestWriteHealthReport(t *testing.T) {
	dir := t.TempDir()
	ipcDir := filepath.Join(dir, "ipc")

	ipcWriter, err := ipc.NewWriter(ipcDir)
	if err != nil {
		t.Fatalf("failed to create IPC writer: %v", err)
	}

	c := &Child{
		cfg:       &Config{ProjectID: "test-project"},
		ipcWriter: ipcWriter,
		startedAt: time.Now(),
		results:   make(map[string]*checkResult),
	}

	// Add some results
	c.results["check1"] = &checkResult{Name: "check1", Passed: true, LatencyMs: 50}
	c.results["check2"] = &checkResult{Name: "check2", Passed: false, LatencyMs: 100, Error: "connection refused"}

	c.writeHealthReport()

	// Verify health file was written
	reader := ipc.NewReader(ipcDir)
	report, err := reader.ReadHealth()
	if err != nil {
		t.Fatalf("failed to read health report: %v", err)
	}
	if report == nil {
		t.Fatal("health report is nil")
	}

	if report.ProjectID != "test-project" {
		t.Errorf("project ID mismatch: got %s, want test-project", report.ProjectID)
	}
	if report.Status != ipc.StatusDegraded {
		t.Errorf("status mismatch: got %s, want %s", report.Status, ipc.StatusDegraded)
	}
	if len(report.Checks) != 2 {
		t.Errorf("checks count mismatch: got %d, want 2", len(report.Checks))
	}
}

func TestDetermineStatus(t *testing.T) {
	tests := []struct {
		name     string
		results  map[string]*checkResult
		expected string
	}{
		{
			name:     "no checks",
			results:  map[string]*checkResult{},
			expected: ipc.StatusUnknown,
		},
		{
			name: "all passing",
			results: map[string]*checkResult{
				"c1": {Passed: true},
				"c2": {Passed: true},
			},
			expected: ipc.StatusHealthy,
		},
		{
			name: "all failing",
			results: map[string]*checkResult{
				"c1": {Passed: false},
				"c2": {Passed: false},
			},
			expected: ipc.StatusDown,
		},
		{
			name: "mixed",
			results: map[string]*checkResult{
				"c1": {Passed: true},
				"c2": {Passed: false},
			},
			expected: ipc.StatusDegraded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			ipcWriter, _ := ipc.NewWriter(dir)

			c := &Child{
				cfg:       &Config{ProjectID: "test"},
				ipcWriter: ipcWriter,
				startedAt: time.Now(),
				results:   tt.results,
			}

			c.writeHealthReport()

			reader := ipc.NewReader(dir)
			report, err := reader.ReadHealth()
			if err != nil {
				t.Fatalf("failed to read: %v", err)
			}
			if report.Status != tt.expected {
				t.Errorf("status mismatch: got %s, want %s", report.Status, tt.expected)
			}
		})
	}
}

func TestExecuteCommand_healthCheck(t *testing.T) {
	dir := t.TempDir()
	ipcWriter, _ := ipc.NewWriter(dir)

	c := &Child{
		cfg: &Config{ProjectID: "test"},
		monitorCfg: &monitor.Config{
			Checks: []monitor.CheckConfig{},
		},
		ipcWriter: ipcWriter,
		startedAt: time.Now(),
		results:   make(map[string]*checkResult),
		ctx:       context.Background(),
	}

	cmd := &ipc.Command{
		ID:     "cmd_1",
		Action: ipc.ActionHealthCheck,
	}

	result := c.executeCommand(cmd)
	if result.Status != ipc.ResultSuccess {
		t.Errorf("expected success, got %s: %s", result.Status, result.Message)
	}
}

func TestExecuteCommand_fetchLogs(t *testing.T) {
	dir := t.TempDir()
	ipcWriter, _ := ipc.NewWriter(dir)

	c := &Child{
		cfg:       &Config{ProjectID: "test"},
		ipcWriter: ipcWriter,
		startedAt: time.Now(),
		results:   make(map[string]*checkResult),
		ctx:       context.Background(),
	}

	cmd := &ipc.Command{
		ID:      "cmd_2",
		Action:  ipc.ActionFetchLogs,
		Payload: map[string]any{"lines": float64(50)},
	}

	result := c.executeCommand(cmd)
	if result.Status != ipc.ResultSuccess {
		t.Errorf("expected success, got %s: %s", result.Status, result.Message)
	}
}

func TestExecuteCommand_unknown(t *testing.T) {
	dir := t.TempDir()
	ipcWriter, _ := ipc.NewWriter(dir)

	c := &Child{
		cfg:       &Config{ProjectID: "test"},
		ipcWriter: ipcWriter,
		startedAt: time.Now(),
		results:   make(map[string]*checkResult),
		ctx:       context.Background(),
	}

	cmd := &ipc.Command{
		ID:     "cmd_3",
		Action: "unknown_action",
	}

	result := c.executeCommand(cmd)
	if result.Status != ipc.ResultFailed {
		t.Errorf("expected failed, got %s", result.Status)
	}
}
