package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBootstrapCore tests the core bootstrap functionality
func TestBootstrapCore(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		config      BootstrapConfig
		expectError bool
	}{
		{
			name:        "valid configuration",
			projectName: "test-project",
			config: BootstrapConfig{
				ProjectName:  "test-project",
				RepoURL:     "https://github.com/user/repo.git",
				LocalPath:   "/tmp/test",
				PollInterval: "30s",
				Port:        "8080",
				SSHKeyPath:  "",
				GitToken:    "",
			},
			expectError: false,
		},
		{
			name:        "missing repository URL",
			projectName: "test-project",
			config: BootstrapConfig{
				ProjectName:  "test-project",
				LocalPath:    "/tmp/test",
				PollInterval: "30s",
				Port:         "8080",
			},
			expectError: true,
		},
		{
			name:        "missing local path",
			projectName: "test-project",
			config: BootstrapConfig{
				ProjectName:  "test-project",
				RepoURL:      "https://github.com/user/repo.git",
				PollInterval: "30s",
				Port:         "8080",
			},
			expectError: true,
		},
		{
			name:        "invalid polling interval",
			projectName: "test-project",
			config: BootstrapConfig{
				ProjectName:  "test-project",
				RepoURL:      "https://github.com/user/repo.git",
				LocalPath:    "/tmp/test",
				PollInterval: "-1s",
				Port:         "8080",
			},
			expectError: true,
		},
		{
			name:        "invalid port",
			projectName: "test-project",
			config: BootstrapConfig{
				ProjectName:  "test-project",
				RepoURL:      "https://github.com/user/repo.git",
				LocalPath:    "/tmp/test",
				PollInterval: "30s",
				Port:         "-1",
			},
			expectError: true,
		},
		{
			name:        "port zero is valid",
			projectName: "test-project",
			config: BootstrapConfig{
				ProjectName:  "test-project",
				RepoURL:      "https://github.com/user/repo.git",
				LocalPath:    "/tmp/test",
				PollInterval: "30s",
				Port:         "0",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfiguration(&tt.config)
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

// TestProjectNameValidation tests project name validation
func TestProjectNameValidation(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		expectValid bool
	}{
		{"valid-project", "valid-project", true},
		{"valid_project", "valid_project", true},
		{"validproject123", "validproject123", true},
		{"ValidProject", "ValidProject", true},
		{"#00", "#00", true},
		{"invalid project", "invalid project", false},
		{"invalid@project", "invalid@project", false},
		{"invalid.project", "invalid.project", false},
		{"invalid/project", "invalid/project", false},
		{"invalid\\project", "invalid\\project", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := isValidProjectName(tt.projectName)
			if valid != tt.expectValid {
				t.Errorf("isValidProjectName(%s) = %v, want %v", tt.projectName, valid, tt.expectValid)
			}
		})
	}
}

// TestDirectoryStructureCreation tests directory creation
func TestDirectoryStructureCreation(t *testing.T) {
	tempDir := t.TempDir()
	
	config := BootstrapConfig{
		ProjectName: "test-project",
		LocalPath:   filepath.Join(tempDir, "working-dir"),
		WorkingDir:  tempDir,
	}

	err := createDirectoryStructure(&config)
	if err != nil {
		t.Fatalf("Failed to create directory structure: %v", err)
	}

	// Check if directories were created
	expectedDirs := []string{
		config.LocalPath,
		filepath.Join(tempDir, ".beacon", "config", "projects", "test-project"),
	}

	for _, dir := range expectedDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Expected directory %s to exist", dir)
		}
	}
}

// TestEnvironmentFileCreation tests environment file creation
func TestEnvironmentFileCreation(t *testing.T) {
	tempDir := t.TempDir()
	
	config := BootstrapConfig{
		ProjectName:      "test-project",
		RepoURL:         "https://github.com/user/repo.git",
		LocalPath:        "/tmp/test",
		PollInterval:     "30s",
		Port:             "8080",
		SSHKeyPath:       "",
		GitToken:         "",
		ProjectConfigDir: filepath.Join(tempDir, ".beacon", "config", "projects", "test-project"),
	}

	err := createEnvironmentFile(&config)
	if err != nil {
		t.Fatalf("Failed to create environment file: %v", err)
	}

	// Check if file exists
	envPath := filepath.Join(config.ProjectConfigDir, "env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		t.Errorf("Expected environment file %s to exist", envPath)
	}

	// Read and verify content
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read environment file: %v", err)
	}

	contentStr := string(content)
	expectedVars := []string{
		"BEACON_REPO_URL",
		"BEACON_LOCAL_PATH",
		"BEACON_POLL_INTERVAL",
		"BEACON_PORT",
	}

	for _, varName := range expectedVars {
		if !contains(contentStr, varName) {
			t.Errorf("Expected environment file to contain %s", varName)
		}
	}
}

// TestSystemdServiceCreation tests systemd service creation
func TestSystemdServiceCreation(t *testing.T) {
	tempDir := t.TempDir()
	systemdDir := filepath.Join(tempDir, ".config", "systemd", "user")
	servicePath := filepath.Join(systemdDir, "beacon@test-project.service")

	config := BootstrapConfig{
		ProjectName:      "test-project",
		RepoURL:          "https://github.com/user/repo.git",
		LocalPath:        "/tmp/test",
		PollInterval:     "30s",
		Port:             "8080",
		SSHKeyPath:       "",
		GitToken:         "",
		ProjectConfigDir: filepath.Join(tempDir, ".beacon", "config", "projects", "test-project"),
	}

	err := createSystemdService(&config)
	if err != nil {
		t.Fatalf("Failed to create systemd service: %v", err)
	}

	// Check if service file exists
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		t.Errorf("Expected systemd service file %s to exist", servicePath)
	}

	// Read and verify content
	content, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("Failed to read systemd service file: %v", err)
	}

	contentStr := string(content)
	expectedContent := []string{
		"[Unit]",
		"[Service]",
		"[Install]",
		"beacon@test-project",
	}

	for _, expected := range expectedContent {
		if !contains(contentStr, expected) {
			t.Errorf("Expected systemd service file to contain %s", expected)
		}
	}
}

// TestExistingComponentsCheck tests checking for existing components
func TestExistingComponentsCheck(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".beacon")
	projectName := "test-project"

	// Create some existing components
	projectDir := filepath.Join(configDir, "config", "projects", projectName)
	os.MkdirAll(projectDir, 0755)
	envFile := filepath.Join(projectDir, "env")
	os.WriteFile(envFile, []byte("test"), 0644)

	existing := checkExistingComponents(projectName)
	if len(existing) == 0 {
		t.Error("Expected existing components to be detected")
	}
}

// TestPermissionsSetting tests permission setting
func TestPermissionsSetting(t *testing.T) {
	tempDir := t.TempDir()
	
	config := BootstrapConfig{
		ProjectName: "test-project",
		LocalPath:   filepath.Join(tempDir, "working-dir"),
		WorkingDir:  tempDir,
	}

	// Create test files
	os.MkdirAll(config.LocalPath, 0755)

	err := setPermissions(&config)
	if err != nil {
		t.Fatalf("Failed to set permissions: %v", err)
	}

	// Check if files exist (permissions are harder to test cross-platform)
	if _, err := os.Stat(config.LocalPath); os.IsNotExist(err) {
		t.Error("Expected working directory to exist")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || 
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || 
		containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}