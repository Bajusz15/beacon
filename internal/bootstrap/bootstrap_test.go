package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

// TestBootstrapConfig tests the BootstrapConfig struct and its validation
func TestBootstrapConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *BootstrapConfig
		wantErr bool
	}{
		{
			name: "valid configuration",
			config: &BootstrapConfig{
				ProjectName:  "test-project",
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
				ProjectName:  "test-project",
				LocalPath:    "/tmp/test",
				PollInterval: "60s",
				Port:         "8080",
			},
			wantErr: true,
		},
		{
			name: "missing local path",
			config: &BootstrapConfig{
				ProjectName:  "test-project",
				RepoURL:      "https://github.com/user/repo.git",
				PollInterval: "60s",
				Port:         "8080",
			},
			wantErr: true,
		},
		{
			name: "invalid polling interval",
			config: &BootstrapConfig{
				ProjectName:  "test-project",
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
				ProjectName:  "test-project",
				RepoURL:      "https://github.com/user/repo.git",
				LocalPath:    "/tmp/test",
				PollInterval: "60s",
				Port:         "99999",
			},
			wantErr: true,
		},
		{
			name: "port zero is valid",
			config: &BootstrapConfig{
				ProjectName:  "test-project",
				RepoURL:      "https://github.com/user/repo.git",
				LocalPath:    "/tmp/test",
				PollInterval: "60s",
				Port:         "0",
			},
			wantErr: false,
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

// TestIsValidProjectName tests project name validation
func TestIsValidProjectName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"valid-project", true},
		{"valid_project", true},
		{"validproject123", true},
		{"ValidProject", true},
		{"", false},
		{"invalid project", false},
		{"invalid@project", false},
		{"invalid.project", false},
		{"invalid/project", false},
		{"invalid\\project", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidProjectName(tt.name); got != tt.want {
				t.Errorf("isValidProjectName() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCreateDirectoryStructure tests directory creation with temporary directories
func TestCreateDirectoryStructure(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "beacon-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Mock HOME environment variable
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	config := &BootstrapConfig{
		ProjectName: "test-project",
		LocalPath:   filepath.Join(tempDir, "local-path"),
		WorkingDir:  filepath.Join(tempDir, "working-dir"),
	}

	err = createDirectoryStructure(config)
	if err != nil {
		t.Fatalf("createDirectoryStructure() error = %v", err)
	}

	// Check if directories were created
	expectedDirs := []string{
		filepath.Join(tempDir, ".beacon", "config", "projects", "test-project"),
		filepath.Join(tempDir, "local-path"),
		filepath.Join(tempDir, "working-dir"),
		filepath.Join(tempDir, ".beacon"),
	}

	for _, dir := range expectedDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Expected directory %s to exist", dir)
		}
	}

	// Check if ProjectConfigDir was set
	if config.ProjectConfigDir == "" {
		t.Error("Expected ProjectConfigDir to be set")
	}
}

// TestCreateEnvironmentFile tests environment file creation
func TestCreateEnvironmentFile(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "beacon-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	config := &BootstrapConfig{
		ProjectName:      "test-project",
		RepoURL:          "https://github.com/user/repo.git",
		LocalPath:        "/tmp/test",
		DeployCommand:    "make deploy",
		PollInterval:     "60s",
		Port:             "8080",
		SSHKeyPath:       "/home/user/.ssh/id_rsa",
		GitToken:         "ghp_token123",
		SecureEnvPath:    "/etc/beacon/test.env",
		ProjectConfigDir: tempDir,
	}

	err = createEnvironmentFile(config)
	if err != nil {
		t.Fatalf("createEnvironmentFile() error = %v", err)
	}

	// Check if file was created
	envPath := filepath.Join(tempDir, "env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		t.Errorf("Expected environment file %s to exist", envPath)
	}

	// Read and verify file contents
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read environment file: %v", err)
	}

	contentStr := string(content)

	// Check for required fields
	requiredFields := []string{
		"BEACON_REPO_URL=https://github.com/user/repo.git",
		"BEACON_LOCAL_PATH=/tmp/test",
		"BEACON_DEPLOY_CMD=make deploy",
		"BEACON_POLL_INTERVAL=60s",
		"BEACON_PORT=8080",
		"BEACON_SSH_KEY_PATH=/home/user/.ssh/id_rsa",
		"BEACON_GIT_TOKEN=ghp_token123",
		"BEACON_SECURE_ENV_PATH=/etc/beacon/test.env",
	}

	for _, field := range requiredFields {
		if !strings.Contains(contentStr, field) {
			t.Errorf("Expected environment file to contain: %s", field)
		}
	}
}

// TestCreateSystemdService tests systemd service file creation
func TestCreateSystemdService(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "beacon-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Mock HOME environment variable
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	config := &BootstrapConfig{
		ProjectName:      "test-project",
		ProjectConfigDir: filepath.Join(tempDir, ".beacon", "config", "projects", "test-project"),
		SecureEnvPath:    "/etc/beacon/test.env",
	}

	err = createSystemdService(config)
	if err != nil {
		t.Fatalf("createSystemdService() error = %v", err)
	}

	// Check if service file was created
	servicePath := filepath.Join(tempDir, ".config", "systemd", "user", "beacon@test-project.service")
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		t.Errorf("Expected systemd service file %s to exist", servicePath)
	}

	// Read and verify file contents
	content, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("Failed to read systemd service file: %v", err)
	}

	contentStr := string(content)

	// Check for required fields
	requiredFields := []string{
		"Description=Beacon Agent for test-project",
		"EnvironmentFile=" + config.ProjectConfigDir + "/env",
		"EnvironmentFile=/etc/beacon/test.env",
		"ExecStart=/usr/local/bin/beacon deploy",
		"Restart=always",
	}

	for _, field := range requiredFields {
		if !strings.Contains(contentStr, field) {
			t.Errorf("Expected systemd service file to contain: %s", field)
		}
	}
}

// TestTemplateGeneration tests template parsing and execution
func TestTemplateGeneration(t *testing.T) {
	tests := []struct {
		name     string
		template string
		config   *BootstrapConfig
		expected []string
	}{
		{
			name:     "environment template",
			template: envTemplate,
			config: &BootstrapConfig{
				ProjectName:   "test-project",
				RepoURL:       "https://github.com/user/repo.git",
				LocalPath:     "/tmp/test",
				DeployCommand: "make deploy",
				PollInterval:  "60s",
				Port:          "8080",
				SSHKeyPath:    "/home/user/.ssh/id_rsa",
				GitToken:      "ghp_token123",
				SecureEnvPath: "/etc/beacon/test.env",
			},
			expected: []string{
				"BEACON_REPO_URL=https://github.com/user/repo.git",
				"BEACON_LOCAL_PATH=/tmp/test",
				"BEACON_DEPLOY_CMD=make deploy",
				"BEACON_POLL_INTERVAL=60s",
				"BEACON_PORT=8080",
				"BEACON_SSH_KEY_PATH=/home/user/.ssh/id_rsa",
				"BEACON_GIT_TOKEN=ghp_token123",
				"BEACON_SECURE_ENV_PATH=/etc/beacon/test.env",
			},
		},
		{
			name:     "systemd template",
			template: systemdTemplate,
			config: &BootstrapConfig{
				ProjectName:      "test-project",
				ProjectConfigDir: "/home/user/.beacon/config/projects/test-project",
				SecureEnvPath:    "/etc/beacon/test.env",
			},
			expected: []string{
				"Description=Beacon Agent for test-project",
				"EnvironmentFile=/home/user/.beacon/config/projects/test-project/env",
				"EnvironmentFile=/etc/beacon/test.env",
				"ExecStart=/usr/local/bin/beacon deploy",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := template.New("test").Parse(tt.template)
			if err != nil {
				t.Fatalf("Failed to parse template: %v", err)
			}

			var result strings.Builder
			err = tmpl.Execute(&result, tt.config)
			if err != nil {
				t.Fatalf("Failed to execute template: %v", err)
			}

			resultStr := result.String()

			for _, expected := range tt.expected {
				if !strings.Contains(resultStr, expected) {
					t.Errorf("Expected template output to contain: %s", expected)
				}
			}
		})
	}
}

// TestCheckExistingComponents tests detection of existing components
func TestCheckExistingComponents(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "beacon-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Mock HOME environment variable
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	projectName := "test-project"

	// Initially, no components should exist
	existing := checkExistingComponents(projectName)
	if len(existing) != 0 {
		t.Errorf("Expected no existing components, got: %v", existing)
	}

	// Create project config directory
	projectConfigDir := filepath.Join(tempDir, ".beacon", "config", "projects", projectName)
	err = os.MkdirAll(projectConfigDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create project config dir: %v", err)
	}

	// Check again - should detect config directory
	existing = checkExistingComponents(projectName)
	if len(existing) != 1 || existing[0] != "Project config directory" {
		t.Errorf("Expected 'Project config directory', got: %v", existing)
	}

	// Create environment file
	envPath := filepath.Join(projectConfigDir, "env")
	err = os.WriteFile(envPath, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create env file: %v", err)
	}

	// Check again - should detect both
	existing = checkExistingComponents(projectName)
	if len(existing) != 2 {
		t.Errorf("Expected 2 existing components, got: %v", existing)
	}

	// Create systemd service file
	serviceDir := filepath.Join(tempDir, ".config", "systemd", "user")
	err = os.MkdirAll(serviceDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create service dir: %v", err)
	}

	servicePath := filepath.Join(serviceDir, "beacon@test-project.service")
	err = os.WriteFile(servicePath, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create service file: %v", err)
	}

	// Check again - should detect all three
	existing = checkExistingComponents(projectName)
	if len(existing) != 3 {
		t.Errorf("Expected 3 existing components, got: %v", existing)
	}
}

// TestSetPermissions tests permission setting
func TestSetPermissions(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "beacon-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	config := &BootstrapConfig{
		ProjectName:      "test-project",
		WorkingDir:       filepath.Join(tempDir, "working-dir"),
		ProjectConfigDir: filepath.Join(tempDir, "config"),
	}

	// Create directories
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

	// Set permissions
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

// Benchmark tests for performance
func BenchmarkCreateDirectoryStructure(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "beacon-bench-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Mock HOME environment variable
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	config := &BootstrapConfig{
		ProjectName: "bench-project",
		LocalPath:   filepath.Join(tempDir, "local-path"),
		WorkingDir:  filepath.Join(tempDir, "working-dir"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Clean up previous run
		os.RemoveAll(filepath.Join(tempDir, ".beacon"))
		os.RemoveAll(config.LocalPath)
		os.RemoveAll(config.WorkingDir)

		err := createDirectoryStructure(config)
		if err != nil {
			b.Fatalf("createDirectoryStructure() error = %v", err)
		}
	}
}
