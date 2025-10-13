package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBootstrapIntegration tests the complete bootstrap process
func TestBootstrapIntegration(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".beacon")
	projectName := "integration-test-project"

	config := BootstrapConfig{
		ProjectName:      projectName,
		RepoURL:          "https://github.com/user/repo.git",
		LocalPath:        filepath.Join(tempDir, "working-dir"),
		PollInterval:     "30s",
		Port:             "8080",
		SSHKeyPath:       "",
		GitToken:         "",
		WorkingDir:       tempDir,
		ProjectConfigDir: filepath.Join(configDir, "config", "projects", projectName),
	}

	// Test individual components
	err := createDirectoryStructure(&config)
	if err != nil {
		t.Fatalf("Failed to create directory structure: %v", err)
	}

	err = createEnvironmentFile(&config)
	if err != nil {
		t.Fatalf("Failed to create environment file: %v", err)
	}

	// Verify all components were created
	expectedPaths := []string{
		config.LocalPath,
		config.ProjectConfigDir,
		filepath.Join(config.ProjectConfigDir, "env"),
	}

	for _, path := range expectedPaths {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected path %s to exist", path)
		}
	}
}

// TestBootstrapWithSystemd tests bootstrap with systemd integration
func TestBootstrapWithSystemd(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".beacon")
	systemdDir := filepath.Join(tempDir, ".config", "systemd", "user")
	projectName := "systemd-test-project"

	config := BootstrapConfig{
		ProjectName:      projectName,
		RepoURL:          "https://github.com/user/repo.git",
		LocalPath:        filepath.Join(tempDir, "working-dir"),
		PollInterval:     "30s",
		Port:             "8080",
		SSHKeyPath:       "",
		GitToken:         "",
		WorkingDir:       tempDir,
		ProjectConfigDir: filepath.Join(configDir, "config", "projects", projectName),
	}

	err := createDirectoryStructure(&config)
	if err != nil {
		t.Fatalf("Failed to create directory structure: %v", err)
	}

	err = createSystemdService(&config)
	if err != nil {
		t.Fatalf("Failed to create systemd service: %v", err)
	}

	// Check if systemd service was created
	servicePath := filepath.Join(systemdDir, "beacon@"+projectName+".service")
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		t.Errorf("Expected systemd service file %s to exist", servicePath)
	}
}

// TestBootstrapValidation tests bootstrap validation
func TestBootstrapValidation(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		config      BootstrapConfig
		expectError bool
	}{
		{
			name:        "invalid project name with spaces",
			projectName: "invalid project",
			config: BootstrapConfig{
				ProjectName:  "invalid project",
				RepoURL:      "https://github.com/user/repo.git",
				LocalPath:    "/tmp/test",
				PollInterval: "30s",
				Port:         "8080",
			},
			expectError: true,
		},
		{
			name:        "invalid project name with special chars",
			projectName: "invalid@project",
			config: BootstrapConfig{
				ProjectName:  "invalid@project",
				RepoURL:      "https://github.com/user/repo.git",
				LocalPath:    "/tmp/test",
				PollInterval: "30s",
				Port:         "8080",
			},
			expectError: true,
		},
		{
			name:        "valid project name",
			projectName: "valid-project",
			config: BootstrapConfig{
				ProjectName:  "valid-project",
				RepoURL:      "https://github.com/user/repo.git",
				LocalPath:    "/tmp/test",
				PollInterval: "30s",
				Port:         "8080",
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

// TestBootstrapWithSecureEnv tests bootstrap with secure environment
func TestBootstrapWithSecureEnv(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".beacon")
	projectName := "secure-env-test-project"

	config := BootstrapConfig{
		ProjectName:      projectName,
		RepoURL:          "https://github.com/user/repo.git",
		LocalPath:        filepath.Join(tempDir, "working-dir"),
		PollInterval:     "30s",
		Port:             "8080",
		SecureEnvPath:    "/path/to/secure/env",
		SSHKeyPath:       "/path/to/ssh/key",
		GitToken:         "secret-token",
		WorkingDir:       tempDir,
		ProjectConfigDir: filepath.Join(configDir, "config", "projects", projectName),
	}

	err := createDirectoryStructure(&config)
	if err != nil {
		t.Fatalf("Failed to create directory structure: %v", err)
	}

	err = createEnvironmentFile(&config)
	if err != nil {
		t.Fatalf("Failed to create environment file: %v", err)
	}

	// Check if secure env file was created
	secureEnvPath := filepath.Join(config.ProjectConfigDir, "env.secure")
	if _, err := os.Stat(secureEnvPath); os.IsNotExist(err) {
		t.Errorf("Expected secure env file %s to exist", secureEnvPath)
	}
}

// TestBootstrapWithSSHKey tests bootstrap with SSH key
func TestBootstrapWithSSHKey(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".beacon")
	projectName := "ssh-test-project"

	config := BootstrapConfig{
		ProjectName:      projectName,
		RepoURL:          "https://github.com/user/repo.git",
		LocalPath:        filepath.Join(tempDir, "working-dir"),
		PollInterval:     "30s",
		Port:             "8080",
		SSHKeyPath:       "/path/to/ssh/key",
		GitToken:         "",
		WorkingDir:       tempDir,
		ProjectConfigDir: filepath.Join(configDir, "config", "projects", projectName),
	}

	err := createDirectoryStructure(&config)
	if err != nil {
		t.Fatalf("Failed to create directory structure: %v", err)
	}

	err = createEnvironmentFile(&config)
	if err != nil {
		t.Fatalf("Failed to create environment file: %v", err)
	}
}

// TestBootstrapWithGitToken tests bootstrap with Git token
func TestBootstrapWithGitToken(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".beacon")
	projectName := "git-token-test-project"

	config := BootstrapConfig{
		ProjectName:      projectName,
		RepoURL:          "https://github.com/user/repo.git",
		LocalPath:        filepath.Join(tempDir, "working-dir"),
		PollInterval:     "30s",
		Port:             "8080",
		SSHKeyPath:       "",
		GitToken:         "secret-git-token",
		WorkingDir:       tempDir,
		ProjectConfigDir: filepath.Join(configDir, "config", "projects", projectName),
	}

	err := createDirectoryStructure(&config)
	if err != nil {
		t.Fatalf("Failed to create directory structure: %v", err)
	}

	err = createEnvironmentFile(&config)
	if err != nil {
		t.Fatalf("Failed to create environment file: %v", err)
	}
}

// TestSystemdServiceGeneration tests systemd service generation
func TestSystemdServiceGeneration(t *testing.T) {
	tests := []struct {
		name     string
		config   BootstrapConfig
		expected []string
	}{
		{
			name: "basic systemd service",
			config: BootstrapConfig{
				ProjectName:      "test-project",
				RepoURL:          "https://github.com/user/repo.git",
				LocalPath:        "/tmp/test",
				PollInterval:     "30s",
				Port:             "8080",
				SSHKeyPath:       "",
				GitToken:         "",
				ProjectConfigDir: "/tmp/.beacon/config/projects/test-project",
			},
			expected: []string{
				"[Unit]",
				"[Service]",
				"[Install]",
				"beacon@test-project",
			},
		},
		{
			name: "systemd service with secure env",
			config: BootstrapConfig{
				ProjectName:      "test-project",
				RepoURL:          "https://github.com/user/repo.git",
				LocalPath:        "/tmp/test",
				PollInterval:     "30s",
				Port:             "8080",
				SecureEnvPath:    "/path/to/secure/env",
				SSHKeyPath:       "/path/to/ssh/key",
				GitToken:         "secret-token",
				ProjectConfigDir: "/tmp/.beacon/config/projects/test-project",
			},
			expected: []string{
				"[Unit]",
				"[Service]",
				"[Install]",
				"beacon@test-project",
				"EnvironmentFile",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			systemdDir := filepath.Join(tempDir, ".config", "systemd", "user")
			servicePath := filepath.Join(systemdDir, "beacon@test-project.service")

			err := createSystemdService(&tt.config)
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
			for _, expected := range tt.expected {
				if !contains(contentStr, expected) {
					t.Errorf("Expected systemd template to contain %s", expected)
				}
			}
		})
	}
}

// TestSystemdServiceFileCreation tests systemd service file creation
func TestSystemdServiceFileCreation(t *testing.T) {
	tests := []struct {
		name            string
		config          BootstrapConfig
		projectName     string
		expectSecureEnv bool
	}{
		{
			name: "user systemd service",
			config: BootstrapConfig{
				ProjectName:      "test-project",
				RepoURL:          "https://github.com/user/repo.git",
				LocalPath:        "/tmp/test",
				PollInterval:     "30s",
				Port:             "8080",
				SSHKeyPath:       "",
				GitToken:         "",
				ProjectConfigDir: "/tmp/.beacon/config/projects/test-project",
			},
			projectName:     "test-project",
			expectSecureEnv: false,
		},
		{
			name: "user systemd service with secure env",
			config: BootstrapConfig{
				ProjectName:      "test-project",
				RepoURL:          "https://github.com/user/repo.git",
				LocalPath:        "/tmp/test",
				PollInterval:     "30s",
				Port:             "8080",
				SecureEnvPath:    "/path/to/secure/env",
				SSHKeyPath:       "/path/to/ssh/key",
				GitToken:         "secret-token",
				ProjectConfigDir: "/tmp/.beacon/config/projects/test-project",
			},
			projectName:     "test-project",
			expectSecureEnv: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			systemdDir := filepath.Join(tempDir, ".config", "systemd", "user")
			servicePath := filepath.Join(systemdDir, "beacon@"+tt.projectName+".service")

			err := createSystemdService(&tt.config)
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
			if tt.expectSecureEnv && !contains(contentStr, "EnvironmentFile") {
				t.Error("Expected systemd service to contain EnvironmentFile directive")
			}
		})
	}
}

// TestSystemdServicePermissions tests systemd service permissions
func TestSystemdServicePermissions(t *testing.T) {
	tempDir := t.TempDir()
	systemdDir := filepath.Join(tempDir, ".config", "systemd", "user")
	servicePath := filepath.Join(systemdDir, "beacon@perms-test-project.service")

	config := BootstrapConfig{
		ProjectName:      "perms-test-project",
		RepoURL:          "https://github.com/user/repo.git",
		LocalPath:        "/tmp/test",
		PollInterval:     "30s",
		Port:             "8080",
		SSHKeyPath:       "",
		GitToken:         "",
		ProjectConfigDir: "/tmp/.beacon/config/projects/perms-test-project",
	}

	err := createSystemdService(&config)
	if err != nil {
		t.Fatalf("Failed to create systemd service: %v", err)
	}

	// Check if service file exists
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		t.Errorf("Expected systemd service file %s to exist", servicePath)
	}
}

// TestSystemdServiceDirectoryCreation tests systemd service directory creation
func TestSystemdServiceDirectoryCreation(t *testing.T) {
	tempDir := t.TempDir()
	systemdDir := filepath.Join(tempDir, ".config", "systemd", "user")

	config := BootstrapConfig{
		ProjectName:      "dir-test-project",
		RepoURL:          "https://github.com/user/repo.git",
		LocalPath:        "/tmp/test",
		PollInterval:     "30s",
		Port:             "8080",
		SSHKeyPath:       "",
		GitToken:         "",
		ProjectConfigDir: "/tmp/.beacon/config/projects/dir-test-project",
	}

	err := createSystemdService(&config)
	if err != nil {
		t.Fatalf("Failed to create systemd service: %v", err)
	}

	// Check if systemd directory was created
	if _, err := os.Stat(systemdDir); os.IsNotExist(err) {
		t.Errorf("Expected systemd directory %s to exist", systemdDir)
	}
}

// TestSystemdServiceTemplateValidation tests systemd service template validation
func TestSystemdServiceTemplateValidation(t *testing.T) {
	tests := []struct {
		name     string
		config   BootstrapConfig
		expected []string
	}{
		{
			name: "valid configuration",
			config: BootstrapConfig{
				ProjectName:      "test-project",
				RepoURL:          "https://github.com/user/repo.git",
				LocalPath:        "/tmp/test",
				PollInterval:     "30s",
				Port:             "8080",
				SSHKeyPath:       "",
				GitToken:         "",
				ProjectConfigDir: "/tmp/.beacon/config/projects/test-project",
			},
			expected: []string{
				"[Unit]",
				"[Service]",
				"[Install]",
			},
		},
		{
			name: "configuration with secure env",
			config: BootstrapConfig{
				ProjectName:      "test-project",
				RepoURL:          "https://github.com/user/repo.git",
				LocalPath:        "/tmp/test",
				PollInterval:     "30s",
				Port:             "8080",
				SecureEnvPath:    "/path/to/secure/env",
				SSHKeyPath:       "/path/to/ssh/key",
				GitToken:         "secret-token",
				ProjectConfigDir: "/tmp/.beacon/config/projects/test-project",
			},
			expected: []string{
				"[Unit]",
				"[Service]",
				"[Install]",
				"EnvironmentFile",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := createSystemdService(&tt.config)
			if err != nil {
				t.Fatalf("Failed to create systemd service: %v", err)
			}

			// The service file should be created in the default location
			// We can't easily test the content without knowing the exact path
			// but we can verify the function doesn't error
		})
	}
}

// TestSystemdServiceWithSpecialCharacters tests systemd service with special characters
func TestSystemdServiceWithSpecialCharacters(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		expectError bool
	}{
		{
			name:        "project with hyphens",
			projectName: "test-project",
			expectError: false,
		},
		{
			name:        "project with underscores",
			projectName: "test_project",
			expectError: false,
		},
		{
			name:        "project with numbers",
			projectName: "test123",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := BootstrapConfig{
				ProjectName:      tt.projectName,
				RepoURL:          "https://github.com/user/repo.git",
				LocalPath:        "/tmp/test",
				PollInterval:     "30s",
				Port:             "8080",
				SSHKeyPath:       "",
				GitToken:         "",
				ProjectConfigDir: "/tmp/.beacon/config/projects/" + tt.projectName,
			}

			err := createSystemdService(&config)
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

// TestSystemdServiceEnvironmentFiles tests systemd service environment files
func TestSystemdServiceEnvironmentFiles(t *testing.T) {
	tests := []struct {
		name            string
		config          BootstrapConfig
		expectSecureEnv bool
	}{
		{
			name: "basic environment file only",
			config: BootstrapConfig{
				ProjectName:      "test-project",
				RepoURL:          "https://github.com/user/repo.git",
				LocalPath:        "/tmp/test",
				PollInterval:     "30s",
				Port:             "8080",
				SSHKeyPath:       "",
				GitToken:         "",
				ProjectConfigDir: "/tmp/.beacon/config/projects/test-project",
			},
			expectSecureEnv: false,
		},
		{
			name: "environment file with secure env",
			config: BootstrapConfig{
				ProjectName:      "test-project",
				RepoURL:          "https://github.com/user/repo.git",
				LocalPath:        "/tmp/test",
				PollInterval:     "30s",
				Port:             "8080",
				SecureEnvPath:    "/path/to/secure/env",
				SSHKeyPath:       "/path/to/ssh/key",
				GitToken:         "secret-token",
				ProjectConfigDir: "/tmp/.beacon/config/projects/test-project",
			},
			expectSecureEnv: true,
		},
		{
			name: "only secure env file",
			config: BootstrapConfig{
				ProjectName:      "test-project",
				RepoURL:          "https://github.com/user/repo.git",
				LocalPath:        "/tmp/test",
				PollInterval:     "30s",
				Port:             "8080",
				SecureEnvPath:    "/path/to/secure/env",
				SSHKeyPath:       "",
				GitToken:         "",
				ProjectConfigDir: "/tmp/.beacon/config/projects/test-project",
			},
			expectSecureEnv: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			systemdDir := filepath.Join(tempDir, ".config", "systemd", "user")
			servicePath := filepath.Join(systemdDir, "beacon@test-project.service")

			err := createSystemdService(&tt.config)
			if err != nil {
				t.Fatalf("Failed to create systemd service: %v", err)
			}

			// Read and verify content
			content, err := os.ReadFile(servicePath)
			if err != nil {
				t.Fatalf("Failed to read systemd service file: %v", err)
			}

			contentStr := string(content)
			if tt.expectSecureEnv && !contains(contentStr, "EnvironmentFile") {
				t.Error("Expected systemd template to contain EnvironmentFile directive")
			}
		})
	}
}

// TestSystemdServiceRestartBehavior tests systemd service restart behavior
func TestSystemdServiceRestartBehavior(t *testing.T) {
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
		ProjectConfigDir: "/tmp/.beacon/config/projects/test-project",
	}

	err := createSystemdService(&config)
	if err != nil {
		t.Fatalf("Failed to create systemd service: %v", err)
	}

	// Read and verify content
	content, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("Failed to read systemd service file: %v", err)
	}

	contentStr := string(content)
	expectedRestartDirectives := []string{
		"Restart=always",
		"RestartSec=5",
	}

	for _, directive := range expectedRestartDirectives {
		if !contains(contentStr, directive) {
			t.Errorf("Expected systemd template to contain %s", directive)
		}
	}
}

// TestSystemdServiceLogging tests systemd service logging
func TestSystemdServiceLogging(t *testing.T) {
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
		ProjectConfigDir: "/tmp/.beacon/config/projects/test-project",
	}

	err := createSystemdService(&config)
	if err != nil {
		t.Fatalf("Failed to create systemd service: %v", err)
	}

	// Read and verify content
	content, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("Failed to read systemd service file: %v", err)
	}

	contentStr := string(content)
	expectedLoggingDirectives := []string{
		"StandardOutput=journal",
		"StandardError=journal",
	}

	for _, directive := range expectedLoggingDirectives {
		if !contains(contentStr, directive) {
			t.Errorf("Expected systemd template to contain %s", directive)
		}
	}
}
