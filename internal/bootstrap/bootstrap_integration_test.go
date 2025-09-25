package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBootstrapIntegration tests the complete bootstrap process
func TestBootstrapIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "beacon-integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Mock HOME environment variable
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	// Test the complete bootstrap process
	projectName := "integration-test-project"

	// Test directory structure creation
	config := &BootstrapConfig{
		ProjectName:   projectName,
		RepoURL:       "https://github.com/testuser/testrepo.git",
		LocalPath:     filepath.Join(tempDir, "local-path"),
		DeployCommand: "make deploy",
		PollInterval:  "30s",
		Port:          "8080",
		WorkingDir:    filepath.Join(tempDir, "working-dir"),
	}

	// Test directory creation
	err = createDirectoryStructure(config)
	if err != nil {
		t.Fatalf("createDirectoryStructure() error = %v", err)
	}

	// Verify that directories were created
	expectedDirs := []string{
		filepath.Join(tempDir, ".beacon", "config", "projects", projectName),
		filepath.Join(tempDir, "local-path"),
		filepath.Join(tempDir, ".beacon"),
	}

	for _, dir := range expectedDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Expected directory %s to exist", dir)
		}
	}

	// Test environment file creation
	err = createEnvironmentFile(config)
	if err != nil {
		t.Fatalf("createEnvironmentFile() error = %v", err)
	}

	// Verify environment file was created
	envPath := filepath.Join(tempDir, ".beacon", "config", "projects", projectName, "env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		t.Errorf("Expected environment file %s to exist", envPath)
	}

	// Verify environment file contents
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read environment file: %v", err)
	}

	contentStr := string(content)
	expectedContent := []string{
		"BEACON_REPO_URL=https://github.com/testuser/testrepo.git",
		"BEACON_LOCAL_PATH=" + filepath.Join(tempDir, "local-path"),
		"BEACON_DEPLOY_CMD=make deploy",
		"BEACON_POLL_INTERVAL=30s",
		"BEACON_PORT=8080",
	}

	for _, expected := range expectedContent {
		if !strings.Contains(contentStr, expected) {
			t.Errorf("Expected environment file to contain: %s", expected)
		}
	}
}

// TestBootstrapWithSystemd tests bootstrap with systemd service creation
func TestBootstrapWithSystemd(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "beacon-systemd-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Mock HOME environment variable
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	// Test systemd service creation
	config := &BootstrapConfig{
		ProjectName:      "systemd-test-project",
		ProjectConfigDir: filepath.Join(tempDir, ".beacon", "config", "projects", "systemd-test-project"),
		SecureEnvPath:    "/etc/beacon/test.env",
	}

	// Create the systemd service
	err = createSystemdService(config)
	if err != nil {
		t.Fatalf("createSystemdService() error = %v", err)
	}

	// Verify systemd service file was created
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
	expectedContent := []string{
		"Description=Beacon Agent for systemd-test-project",
		"ExecStart=/usr/local/bin/beacon deploy",
		"Restart=always",
		"Type=simple",
	}

	for _, expected := range expectedContent {
		if !strings.Contains(contentStr, expected) {
			t.Errorf("Expected systemd service file to contain: %s", expected)
		}
	}
}

// TestBootstrapForceOverwrite tests the force overwrite functionality
func TestBootstrapForceOverwrite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "beacon-force-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Mock HOME environment variable
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	projectName := "force-test-project"

	// First, create some existing components
	projectConfigDir := filepath.Join(tempDir, ".beacon", "config", "projects", projectName)
	err = os.MkdirAll(projectConfigDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create project config dir: %v", err)
	}

	envPath := filepath.Join(projectConfigDir, "env")
	err = os.WriteFile(envPath, []byte("old content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create env file: %v", err)
	}

	// Test environment file overwrite
	config := &BootstrapConfig{
		ProjectName:      projectName,
		RepoURL:          "https://github.com/testuser/testrepo.git",
		LocalPath:        filepath.Join(tempDir, "local-path"),
		DeployCommand:    "make deploy",
		PollInterval:     "60s",
		Port:             "8080",
		ProjectConfigDir: projectConfigDir,
	}

	// Overwrite the environment file
	err = createEnvironmentFile(config)
	if err != nil {
		t.Fatalf("createEnvironmentFile() error = %v", err)
	}

	// Verify that the environment file was overwritten
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read environment file: %v", err)
	}

	contentStr := string(content)
	if strings.Contains(contentStr, "old content") {
		t.Error("Expected environment file to be overwritten, but old content still exists")
	}

	if !strings.Contains(contentStr, "BEACON_REPO_URL=https://github.com/testuser/testrepo.git") {
		t.Error("Expected environment file to contain new content")
	}
}

// TestBootstrapValidation tests various validation scenarios
func TestBootstrapValidation(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		expectError bool
	}{
		{
			name:        "invalid project name with spaces",
			projectName: "invalid project name",
			expectError: true,
		},
		{
			name:        "invalid project name with special chars",
			projectName: "invalid@project",
			expectError: true,
		},
		{
			name:        "valid project name",
			projectName: "valid-project",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test project name validation
			isValid := isValidProjectName(tt.projectName)
			if isValid == tt.expectError {
				t.Errorf("isValidProjectName() = %v, want %v", isValid, !tt.expectError)
			}
		})
	}
}

// TestBootstrapWithSecureEnv tests bootstrap with secure environment file
func TestBootstrapWithSecureEnv(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "beacon-secure-env-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Mock HOME environment variable
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	projectName := "secure-env-test-project"
	secureEnvPath := filepath.Join(tempDir, "secure.env")

	// Test environment file creation with secure env path
	config := &BootstrapConfig{
		ProjectName:      projectName,
		RepoURL:          "https://github.com/testuser/testrepo.git",
		LocalPath:        filepath.Join(tempDir, "local-path"),
		DeployCommand:    "make deploy",
		PollInterval:     "60s",
		Port:             "8080",
		SecureEnvPath:    secureEnvPath,
		WorkingDir:       filepath.Join(tempDir, "working-dir"),
		ProjectConfigDir: filepath.Join(tempDir, ".beacon", "config", "projects", projectName),
	}

	// Create directory structure first
	err = createDirectoryStructure(config)
	if err != nil {
		t.Fatalf("createDirectoryStructure() error = %v", err)
	}

	// Create the environment file
	err = createEnvironmentFile(config)
	if err != nil {
		t.Fatalf("createEnvironmentFile() error = %v", err)
	}

	// Verify environment file contains secure env path
	envPath := filepath.Join(tempDir, ".beacon", "config", "projects", projectName, "env")
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read environment file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "BEACON_SECURE_ENV_PATH="+secureEnvPath) {
		t.Errorf("Expected environment file to contain secure env path: %s", secureEnvPath)
	}
}

// TestBootstrapWithSSHKey tests bootstrap with SSH key configuration
func TestBootstrapWithSSHKey(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "beacon-ssh-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Mock HOME environment variable
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	projectName := "ssh-test-project"
	sshKeyPath := filepath.Join(tempDir, ".ssh", "id_rsa")

	// Test environment file creation with SSH key path
	config := &BootstrapConfig{
		ProjectName:      projectName,
		RepoURL:          "git@github.com:testuser/testrepo.git",
		LocalPath:        filepath.Join(tempDir, "local-path"),
		DeployCommand:    "make deploy",
		PollInterval:     "60s",
		Port:             "8080",
		SSHKeyPath:       sshKeyPath,
		WorkingDir:       filepath.Join(tempDir, "working-dir"),
		ProjectConfigDir: filepath.Join(tempDir, ".beacon", "config", "projects", projectName),
	}

	// Create directory structure first
	err = createDirectoryStructure(config)
	if err != nil {
		t.Fatalf("createDirectoryStructure() error = %v", err)
	}

	// Create the environment file
	err = createEnvironmentFile(config)
	if err != nil {
		t.Fatalf("createEnvironmentFile() error = %v", err)
	}

	// Verify environment file contains SSH key path
	envPath := filepath.Join(tempDir, ".beacon", "config", "projects", projectName, "env")
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read environment file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "BEACON_SSH_KEY_PATH="+sshKeyPath) {
		t.Errorf("Expected environment file to contain SSH key path: %s", sshKeyPath)
	}
}

// TestBootstrapWithGitToken tests bootstrap with Git token configuration
func TestBootstrapWithGitToken(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "beacon-git-token-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Mock HOME environment variable
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	projectName := "git-token-test-project"
	gitToken := "ghp_1234567890abcdef"

	// Test environment file creation with Git token
	config := &BootstrapConfig{
		ProjectName:      projectName,
		RepoURL:          "https://github.com/testuser/testrepo.git",
		LocalPath:        filepath.Join(tempDir, "local-path"),
		DeployCommand:    "make deploy",
		PollInterval:     "60s",
		Port:             "8080",
		GitToken:         gitToken,
		WorkingDir:       filepath.Join(tempDir, "working-dir"),
		ProjectConfigDir: filepath.Join(tempDir, ".beacon", "config", "projects", projectName),
	}

	// Create directory structure first
	err = createDirectoryStructure(config)
	if err != nil {
		t.Fatalf("createDirectoryStructure() error = %v", err)
	}

	// Create the environment file
	err = createEnvironmentFile(config)
	if err != nil {
		t.Fatalf("createEnvironmentFile() error = %v", err)
	}

	// Verify environment file contains Git token
	envPath := filepath.Join(tempDir, ".beacon", "config", "projects", projectName, "env")
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read environment file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "BEACON_GIT_TOKEN="+gitToken) {
		t.Errorf("Expected environment file to contain Git token: %s", gitToken)
	}
}
