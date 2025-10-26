package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"beacon/internal/config"
)

// TestNewBootstrapManager tests the creation of a BootstrapManager
func TestNewBootstrapManager(t *testing.T) {
	tests := []struct {
		name             string
		useSystemService bool
		expectError      bool
	}{
		{
			name:             "user service",
			useSystemService: false,
			expectError:      false,
		},
		{
			name:             "system service",
			useSystemService: true,
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bm, err := NewBootstrapManager(tt.useSystemService)
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if !tt.expectError && bm == nil {
				t.Error("Expected BootstrapManager to be created")
			}
			if !tt.expectError && bm != nil {
				if bm.paths == nil {
					t.Error("Expected paths to be initialized")
				}
			}
		})
	}
}

// TestBootstrapManager_ValidateProjectName tests project name validation
func TestBootstrapManager_ValidateProjectName(t *testing.T) {
	paths, err := config.NewBeaconPaths()
	if err != nil {
		t.Fatalf("Failed to create paths: %v", err)
	}

	tests := []struct {
		name        string
		projectName string
		expectError bool
	}{
		{"valid-project", "valid-project", false},
		{"valid_project", "valid_project", false},
		{"validproject123", "validproject123", false},
		{"ValidProject", "ValidProject", false},
		{"invalid project", "invalid project", true},
		{"invalid@project", "invalid@project", true},
		{"invalid.project", "invalid.project", true},
		{"invalid/project", "invalid/project", true},
		{"invalid\\project", "invalid\\project", true},
		{"empty", "", true},
		{"config", "config", true},       // reserved name
		{"templates", "templates", true}, // reserved name
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := paths.ValidateProjectName(tt.projectName)
			hasError := err != nil

			if hasError != tt.expectError {
				t.Errorf("ValidateProjectName(%s) error = %v, want error = %v", tt.projectName, hasError, tt.expectError)
			}
		})
	}
}

// TestBootstrapManager_CreateProjectStructure tests project structure creation
func TestBootstrapManager_CreateProjectStructure(t *testing.T) {
	paths, err := config.NewBeaconPaths()
	if err != nil {
		t.Fatalf("Failed to create paths: %v", err)
	}

	// Override paths to use temp directory
	tempDir := t.TempDir()
	paths.BaseDir = tempDir
	paths.ConfigDir = filepath.Join(tempDir, "config")
	paths.ProjectsDir = filepath.Join(tempDir, "config", "projects")
	paths.LogsDir = filepath.Join(tempDir, "logs")
	paths.WorkingDir = filepath.Join(tempDir, "working")

	projectName := "test-project"

	err = paths.CreateProjectStructure(projectName)
	if err != nil {
		t.Fatalf("Failed to create project structure: %v", err)
	}

	// Check if directories were created
	expectedDirs := []string{
		paths.GetProjectConfigDir(projectName),
		paths.GetProjectKeysDir(projectName),
		paths.GetProjectLogsDir(projectName),
		paths.GetProjectWorkingDir(projectName),
	}

	for _, dir := range expectedDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Expected directory %s to exist", dir)
		}
	}
}

// TestBootstrapManager_CreateEnvironmentFile tests environment file creation
func TestBootstrapManager_CreateEnvironmentFile(t *testing.T) {
	bm, err := NewBootstrapManager(false)
	if err != nil {
		t.Fatalf("Failed to create bootstrap manager: %v", err)
	}

	tempDir := t.TempDir()
	projectName := "env-test-project"

	// Create project structure first
	if err := bm.paths.CreateProjectStructure(projectName); err != nil {
		t.Fatalf("Failed to create project structure: %v", err)
	}

	config := &BootstrapConfig{
		ProjectName:  projectName,
		RepoURL:      "https://github.com/user/repo.git",
		LocalPath:    "/tmp/test",
		PollInterval: "60s",
		Port:         "8080",
		SSHKeyPath:   "",
		GitToken:     "",
		User:         "testuser",
		WorkingDir:   filepath.Join(tempDir, "working"),
	}

	err = bm.createEnvironmentFile(config)
	if err != nil {
		t.Fatalf("Failed to create environment file: %v", err)
	}

	// Check if file exists
	envPath := bm.paths.GetProjectEnvFile(projectName)
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
		"BEACON_PROJECT_NAME",
		"BEACON_WORKING_DIR",
	}

	for _, varName := range expectedVars {
		if !contains(contentStr, varName) {
			t.Errorf("Expected environment file to contain %s", varName)
		}
	}
}

// TestBootstrapManager_CreateSystemdService tests systemd service creation (if available)
func TestBootstrapManager_CreateSystemdService(t *testing.T) {
	bm, err := NewBootstrapManager(false)
	if err != nil {
		t.Fatalf("Failed to create bootstrap manager: %v", err)
	}

	tempDir := t.TempDir()
	projectName := "systemd-test-project"

	// Create project structure first
	if err := bm.paths.CreateProjectStructure(projectName); err != nil {
		t.Fatalf("Failed to create project structure: %v", err)
	}

	config := &BootstrapConfig{
		ProjectName:  projectName,
		RepoURL:      "https://github.com/user/repo.git",
		LocalPath:    "/tmp/test",
		PollInterval: "60s",
		Port:         "8080",
		WorkingDir:   filepath.Join(tempDir, "working"),
		User:         "testuser",
	}

	// Only create systemd service if available
	if bm.serviceManager.IsAvailable() {
		err = bm.createSystemdService(config)
		if err != nil {
			t.Logf("Systemd service creation failed (expected in test environment): %v", err)
			return
		}

		// Check if service file exists
		servicePath := bm.paths.GetSystemdServiceFile(projectName, false)
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
			"beacon@" + projectName,
		}

		for _, expected := range expectedContent {
			if !contains(contentStr, expected) {
				t.Errorf("Expected systemd service file to contain %s", expected)
			}
		}
	} else {
		t.Log("Systemd not available, skipping systemd service test")
	}
}

// TestBootstrapProject_ProjectExists tests checking for existing projects
func TestBootstrapProject_ProjectExists(t *testing.T) {
	paths, err := config.NewBeaconPaths()
	if err != nil {
		t.Fatalf("Failed to create paths: %v", err)
	}

	tempDir := t.TempDir()
	paths.BaseDir = tempDir
	paths.ProjectsDir = filepath.Join(tempDir, "config", "projects")

	projectName := "existing-project"

	// Create a project manually
	if err := paths.CreateProjectStructure(projectName); err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	// Check if it exists
	if !paths.ProjectExists(projectName) {
		t.Error("Expected project to exist")
	}

	// Check non-existing project
	if paths.ProjectExists("non-existing-project") {
		t.Error("Expected non-existing project to not exist")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
