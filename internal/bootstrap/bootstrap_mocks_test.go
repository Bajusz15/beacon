package bootstrap

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

// MockUser provides a mock user for testing
type MockUser struct {
	Uid      string
	Gid      string
	Username string
	Name     string
	HomeDir  string
}

func (u *MockUser) Current() (*user.User, error) {
	return &user.User{
		Uid:      u.Uid,
		Gid:      u.Gid,
		Username: u.Username,
		Name:     u.Name,
		HomeDir:  u.HomeDir,
	}, nil
}

// TestBootstrapWithMockUser tests bootstrap with mocked user
func TestBootstrapWithMockUser(t *testing.T) {
	mockUser := &MockUser{
		Uid:      "1000",
		Gid:      "1000",
		Username: "testuser",
		Name:     "Test User",
		HomeDir:  "/home/testuser",
	}

	// Test user validation
	user, err := mockUser.Current()
	if err != nil {
		t.Fatalf("MockUser.Current() error = %v", err)
	}

	if user.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", user.Username)
	}

	if user.HomeDir != "/home/testuser" {
		t.Errorf("Expected home directory '/home/testuser', got '%s'", user.HomeDir)
	}
}

// TestBootstrapWithMockTemplates tests template generation with mocks
func TestBootstrapWithMockTemplates(t *testing.T) {
	// Test environment template
	config := &BootstrapConfig{
		ProjectName:   "template-test-project",
		RepoURL:       "https://github.com/user/repo.git",
		LocalPath:     "/tmp/test",
		DeployCommand: "make deploy",
		PollInterval:  "60s",
		Port:          "8080",
		SSHKeyPath:    "/home/user/.ssh/id_rsa",
		GitToken:      "ghp_token123",
		SecureEnvPath: "/etc/beacon/test.env",
	}

	// Test template parsing
	tmpl, err := template.New("test").Parse(envTemplate)
	if err != nil {
		t.Fatalf("Failed to parse template: %v", err)
	}

	var result strings.Builder
	err = tmpl.Execute(&result, config)
	if err != nil {
		t.Fatalf("Failed to execute template: %v", err)
	}

	resultStr := result.String()

	// Verify template output
	expectedFields := []string{
		"BEACON_REPO_URL=https://github.com/user/repo.git",
		"BEACON_LOCAL_PATH=/tmp/test",
		"BEACON_DEPLOY_CMD=make deploy",
		"BEACON_POLL_INTERVAL=60s",
		"BEACON_PORT=8080",
		"BEACON_SSH_KEY_PATH=/home/user/.ssh/id_rsa",
		"BEACON_GIT_TOKEN=ghp_token123",
		"BEACON_SECURE_ENV_PATH=/etc/beacon/test.env",
	}

	for _, field := range expectedFields {
		if !strings.Contains(resultStr, field) {
			t.Errorf("Expected template output to contain: %s", field)
		}
	}
}

// TestBootstrapWithMockSystemd tests systemd service creation with mocks
func TestBootstrapWithMockSystemd(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "beacon-mock-systemd-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Mock HOME environment variable
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	config := &BootstrapConfig{
		ProjectName:      "systemd-test-project",
		ProjectConfigDir: filepath.Join(tempDir, ".beacon", "config", "projects", "systemd-test-project"),
		SecureEnvPath:    "/etc/beacon/test.env",
	}

	// Test systemd service creation
	err = createSystemdService(config)
	if err != nil {
		t.Fatalf("createSystemdService() error = %v", err)
	}

	// Verify service file was created
	servicePath := filepath.Join(tempDir, ".config", "systemd", "user", "beacon@systemd-test-project.service")
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		t.Errorf("Expected systemd service file %s to exist", servicePath)
	}

	// Verify service file contents
	content, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("Failed to read systemd service file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "Description=Beacon Agent for systemd-test-project") {
		t.Error("Expected systemd service file to contain description")
	}

	if !strings.Contains(contentStr, "ExecStart=/usr/local/bin/beacon deploy") {
		t.Error("Expected systemd service file to contain ExecStart")
	}
}

// TestBootstrapWithMockPermissions tests permission setting with mocks
func TestBootstrapWithMockPermissions(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "beacon-mock-permissions-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	config := &BootstrapConfig{
		ProjectName:      "permissions-test-project",
		WorkingDir:       filepath.Join(tempDir, "working-dir"),
		ProjectConfigDir: filepath.Join(tempDir, "config"),
	}

	// Create mock files
	err = os.MkdirAll(config.WorkingDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create working dir: %v", err)
	}

	err = os.MkdirAll(config.ProjectConfigDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Create environment file
	envPath := filepath.Join(config.ProjectConfigDir, "env")
	err = os.WriteFile(envPath, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create env file: %v", err)
	}

	// Test permission setting
	err = setPermissions(config)
	if err != nil {
		t.Fatalf("setPermissions() error = %v", err)
	}

	// Check environment file permissions
	info, err := os.Stat(envPath)
	if err != nil {
		t.Fatalf("Failed to stat env file: %v", err)
	}

	// Check if file is readable by all users (0644)
	if info.Mode()&0777 != 0644 {
		t.Errorf("Expected env file permissions 0644, got %o", info.Mode()&0777)
	}
}

// TestBootstrapWithMockValidation tests validation with mocks
func TestBootstrapWithMockValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *BootstrapConfig
		wantErr bool
	}{
		{
			name: "valid configuration",
			config: &BootstrapConfig{
				ProjectName:  "valid-project",
				RepoURL:      "https://github.com/user/repo.git",
				LocalPath:    "/tmp/test",
				PollInterval: "60s",
				Port:         "8080",
			},
			wantErr: false,
		},
		{
			name: "missing repository URL",
			config: &BootstrapConfig{
				ProjectName:  "invalid-project",
				LocalPath:    "/tmp/test",
				PollInterval: "60s",
				Port:         "8080",
			},
			wantErr: true,
		},
		{
			name: "invalid polling interval",
			config: &BootstrapConfig{
				ProjectName:  "invalid-project",
				RepoURL:      "https://github.com/user/repo.git",
				LocalPath:    "/tmp/test",
				PollInterval: "invalid",
				Port:         "8080",
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			config: &BootstrapConfig{
				ProjectName:  "invalid-project",
				RepoURL:      "https://github.com/user/repo.git",
				LocalPath:    "/tmp/test",
				PollInterval: "60s",
				Port:         "99999",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfiguration(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfiguration() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestBootstrapWithMockExistingComponents tests existing components detection with mocks
func TestBootstrapWithMockExistingComponents(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "beacon-mock-existing-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Mock HOME environment variable
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	projectName := "existing-test-project"

	// Initially, no components should exist
	existing := checkExistingComponents(projectName)
	if len(existing) != 0 {
		t.Errorf("Expected no existing components, got: %v", existing)
	}

	// Create mock existing components
	projectConfigDir := filepath.Join(tempDir, ".beacon", "config", "projects", projectName)
	err = os.MkdirAll(projectConfigDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create project config dir: %v", err)
	}

	envPath := filepath.Join(projectConfigDir, "env")
	err = os.WriteFile(envPath, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create env file: %v", err)
	}

	// Create systemd service file
	serviceDir := filepath.Join(tempDir, ".config", "systemd", "user")
	err = os.MkdirAll(serviceDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create service dir: %v", err)
	}

	servicePath := filepath.Join(serviceDir, "beacon@existing-test-project.service")
	err = os.WriteFile(servicePath, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create service file: %v", err)
	}

	// Check again - should detect existing components
	existing = checkExistingComponents(projectName)
	if len(existing) != 3 {
		t.Errorf("Expected 3 existing components, got: %v", existing)
	}
}
