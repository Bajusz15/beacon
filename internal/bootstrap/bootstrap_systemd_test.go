package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

// TestSystemdServiceGeneration tests systemd service file generation
func TestSystemdServiceGeneration(t *testing.T) {
	tests := []struct {
		name           string
		config         *BootstrapConfig
		expectedFields []string
		notExpected    []string
	}{
		{
			name: "basic systemd service",
			config: &BootstrapConfig{
				ProjectName:      "test-project",
				ProjectConfigDir: "/home/user/.beacon/config/projects/test-project",
			},
			expectedFields: []string{
				"Description=Beacon Agent for test-project",
				"EnvironmentFile=/home/user/.beacon/config/projects/test-project/env",
				"ExecStart=/usr/local/bin/beacon deploy",
				"Restart=always",
				"RestartSec=5",
				"Type=simple",
				"After=network.target",
				"StandardOutput=journal",
				"StandardError=journal",
				"WantedBy=default.target",
			},
			notExpected: []string{
				"EnvironmentFile=/etc/beacon/test.env",
			},
		},
		{
			name: "systemd service with secure env",
			config: &BootstrapConfig{
				ProjectName:      "test-project",
				ProjectConfigDir: "/home/user/.beacon/config/projects/test-project",
				SecureEnvPath:    "/etc/beacon/test.env",
			},
			expectedFields: []string{
				"Description=Beacon Agent for test-project",
				"EnvironmentFile=/home/user/.beacon/config/projects/test-project/env",
				"EnvironmentFile=/etc/beacon/test.env",
				"ExecStart=/usr/local/bin/beacon deploy",
				"Restart=always",
				"RestartSec=5",
				"Type=simple",
				"After=network.target",
				"StandardOutput=journal",
				"StandardError=journal",
				"WantedBy=default.target",
			},
		},
		{
			name: "systemd service with special characters in project name",
			config: &BootstrapConfig{
				ProjectName:      "test-project-123",
				ProjectConfigDir: "/home/user/.beacon/config/projects/test-project-123",
			},
			expectedFields: []string{
				"Description=Beacon Agent for test-project-123",
				"EnvironmentFile=/home/user/.beacon/config/projects/test-project-123/env",
				"ExecStart=/usr/local/bin/beacon deploy",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse and execute the systemd template
			tmpl, err := template.New("systemd").Parse(systemdTemplate)
			if err != nil {
				t.Fatalf("Failed to parse systemd template: %v", err)
			}

			var result strings.Builder
			err = tmpl.Execute(&result, tt.config)
			if err != nil {
				t.Fatalf("Failed to execute systemd template: %v", err)
			}

			resultStr := result.String()

			// Check expected fields
			for _, field := range tt.expectedFields {
				if !strings.Contains(resultStr, field) {
					t.Errorf("Expected systemd service to contain: %s", field)
				}
			}

			// Check not expected fields
			for _, field := range tt.notExpected {
				if strings.Contains(resultStr, field) {
					t.Errorf("Expected systemd service to NOT contain: %s", field)
				}
			}

			// Verify the service file structure
			lines := strings.Split(resultStr, "\n")
			if len(lines) < 10 {
				t.Errorf("Expected systemd service to have at least 10 lines, got %d", len(lines))
			}

			// Check for required sections
			sections := []string{"[Unit]", "[Service]", "[Install]"}
			for _, section := range sections {
				if !strings.Contains(resultStr, section) {
					t.Errorf("Expected systemd service to contain section: %s", section)
				}
			}
		})
	}
}

// TestSystemdServiceFileCreation tests actual systemd service file creation
func TestSystemdServiceFileCreation(t *testing.T) {
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

	tests := []struct {
		name           string
		config         *BootstrapConfig
		expectedPath   string
		expectedFields []string
	}{
		{
			name: "user systemd service",
			config: &BootstrapConfig{
				ProjectName:      "test-project",
				ProjectConfigDir: filepath.Join(tempDir, ".beacon", "config", "projects", "test-project"),
			},
			expectedPath: filepath.Join(tempDir, ".config", "systemd", "user", "beacon@test-project.service"),
			expectedFields: []string{
				"Description=Beacon Agent for test-project",
				"ExecStart=/usr/local/bin/beacon deploy",
				"Restart=always",
			},
		},
		{
			name: "user systemd service with secure env",
			config: &BootstrapConfig{
				ProjectName:      "test-project",
				ProjectConfigDir: filepath.Join(tempDir, ".beacon", "config", "projects", "test-project"),
				SecureEnvPath:    "/etc/beacon/test.env",
			},
			expectedPath: filepath.Join(tempDir, ".config", "systemd", "user", "beacon@test-project.service"),
			expectedFields: []string{
				"Description=Beacon Agent for test-project",
				"ExecStart=/usr/local/bin/beacon deploy",
				"Restart=always",
				"EnvironmentFile=/etc/beacon/test.env",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the systemd service
			err := createSystemdService(tt.config)
			if err != nil {
				t.Fatalf("createSystemdService() error = %v", err)
			}

			// Verify the service file was created
			if _, err := os.Stat(tt.expectedPath); os.IsNotExist(err) {
				t.Errorf("Expected systemd service file %s to exist", tt.expectedPath)
			}

			// Read and verify the service file contents
			content, err := os.ReadFile(tt.expectedPath)
			if err != nil {
				t.Fatalf("Failed to read systemd service file: %v", err)
			}

			contentStr := string(content)

			// Check expected fields
			for _, field := range tt.expectedFields {
				if !strings.Contains(contentStr, field) {
					t.Errorf("Expected systemd service file to contain: %s", field)
				}
			}

			// Verify the service file is valid systemd syntax
			if !strings.Contains(contentStr, "[Unit]") {
				t.Error("Expected systemd service file to contain [Unit] section")
			}
			if !strings.Contains(contentStr, "[Service]") {
				t.Error("Expected systemd service file to contain [Service] section")
			}
			if !strings.Contains(contentStr, "[Install]") {
				t.Error("Expected systemd service file to contain [Install] section")
			}
		})
	}
}

// TestSystemdServicePermissions tests systemd service file permissions
func TestSystemdServicePermissions(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "beacon-systemd-perms-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Mock HOME environment variable
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	config := &BootstrapConfig{
		ProjectName:      "perms-test-project",
		ProjectConfigDir: filepath.Join(tempDir, ".beacon", "config", "projects", "perms-test-project"),
	}

	// Create the systemd service
	err = createSystemdService(config)
	if err != nil {
		t.Fatalf("createSystemdService() error = %v", err)
	}

	// Check the service file permissions
	servicePath := filepath.Join(tempDir, ".config", "systemd", "user", "beacon@perms-test-project.service")
	info, err := os.Stat(servicePath)
	if err != nil {
		t.Fatalf("Failed to stat systemd service file: %v", err)
	}

	// Check if the file is readable by the user (should be 644)
	if info.Mode()&0777 != 0644 {
		t.Errorf("Expected systemd service file permissions 0644, got %o", info.Mode()&0777)
	}
}

// TestSystemdServiceDirectoryCreation tests systemd service directory creation
func TestSystemdServiceDirectoryCreation(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "beacon-systemd-dir-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Mock HOME environment variable
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempDir)

	config := &BootstrapConfig{
		ProjectName:      "dir-test-project",
		ProjectConfigDir: filepath.Join(tempDir, ".beacon", "config", "projects", "dir-test-project"),
	}

	// Create the systemd service
	err = createSystemdService(config)
	if err != nil {
		t.Fatalf("createSystemdService() error = %v", err)
	}

	// Verify the systemd user directory was created
	systemdUserDir := filepath.Join(tempDir, ".config", "systemd", "user")
	if _, err := os.Stat(systemdUserDir); os.IsNotExist(err) {
		t.Errorf("Expected systemd user directory %s to exist", systemdUserDir)
	}

	// Verify the service file was created in the correct location
	servicePath := filepath.Join(systemdUserDir, "beacon@dir-test-project.service")
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		t.Errorf("Expected systemd service file %s to exist", servicePath)
	}
}

// TestSystemdServiceTemplateValidation tests systemd service template validation
func TestSystemdServiceTemplateValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *BootstrapConfig
		expectError bool
	}{
		{
			name: "valid configuration",
			config: &BootstrapConfig{
				ProjectName:      "valid-project",
				ProjectConfigDir: "/home/user/.beacon/config/projects/valid-project",
			},
			expectError: false,
		},
		{
			name: "configuration with secure env",
			config: &BootstrapConfig{
				ProjectName:      "valid-project",
				ProjectConfigDir: "/home/user/.beacon/config/projects/valid-project",
				SecureEnvPath:    "/etc/beacon/test.env",
			},
			expectError: false,
		},
		{
			name: "configuration with empty project name",
			config: &BootstrapConfig{
				ProjectName:      "",
				ProjectConfigDir: "/home/user/.beacon/config/projects/",
			},
			expectError: false, // Template should handle empty project name
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse the systemd template
			tmpl, err := template.New("systemd").Parse(systemdTemplate)
			if err != nil {
				t.Fatalf("Failed to parse systemd template: %v", err)
			}

			// Execute the template
			var result strings.Builder
			err = tmpl.Execute(&result, tt.config)
			if (err != nil) != tt.expectError {
				t.Errorf("Template execution error = %v, wantErr %v", err, tt.expectError)
			}

			if !tt.expectError {
				// Verify the template executed successfully
				resultStr := result.String()
				if len(resultStr) == 0 {
					t.Error("Expected non-empty template result")
				}

				// Verify basic systemd structure
				if !strings.Contains(resultStr, "[Unit]") {
					t.Error("Expected template result to contain [Unit] section")
				}
				if !strings.Contains(resultStr, "[Service]") {
					t.Error("Expected template result to contain [Service] section")
				}
				if !strings.Contains(resultStr, "[Install]") {
					t.Error("Expected template result to contain [Install] section")
				}
			}
		})
	}
}

// TestSystemdServiceWithSpecialCharacters tests systemd service with special characters
func TestSystemdServiceWithSpecialCharacters(t *testing.T) {
	tests := []struct {
		name           string
		projectName    string
		expectedFields []string
	}{
		{
			name:        "project with hyphens",
			projectName: "my-test-project",
			expectedFields: []string{
				"Description=Beacon Agent for my-test-project",
				"ExecStart=/usr/local/bin/beacon deploy",
			},
		},
		{
			name:        "project with underscores",
			projectName: "my_test_project",
			expectedFields: []string{
				"Description=Beacon Agent for my_test_project",
				"ExecStart=/usr/local/bin/beacon deploy",
			},
		},
		{
			name:        "project with numbers",
			projectName: "project123",
			expectedFields: []string{
				"Description=Beacon Agent for project123",
				"ExecStart=/usr/local/bin/beacon deploy",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &BootstrapConfig{
				ProjectName:      tt.projectName,
				ProjectConfigDir: "/home/user/.beacon/config/projects/" + tt.projectName,
			}

			// Parse and execute the systemd template
			tmpl, err := template.New("systemd").Parse(systemdTemplate)
			if err != nil {
				t.Fatalf("Failed to parse systemd template: %v", err)
			}

			var result strings.Builder
			err = tmpl.Execute(&result, config)
			if err != nil {
				t.Fatalf("Failed to execute systemd template: %v", err)
			}

			resultStr := result.String()

			// Check expected fields
			for _, field := range tt.expectedFields {
				if !strings.Contains(resultStr, field) {
					t.Errorf("Expected systemd service to contain: %s", field)
				}
			}
		})
	}
}

// TestSystemdServiceEnvironmentFiles tests systemd service environment file handling
func TestSystemdServiceEnvironmentFiles(t *testing.T) {
	tests := []struct {
		name          string
		config        *BootstrapConfig
		expectedCount int
		expectedFiles []string
	}{
		{
			name: "basic environment file only",
			config: &BootstrapConfig{
				ProjectName:      "env-test-project",
				ProjectConfigDir: "/home/user/.beacon/config/projects/env-test-project",
			},
			expectedCount: 1,
			expectedFiles: []string{
				"/home/user/.beacon/config/projects/env-test-project/env",
			},
		},
		{
			name: "environment file with secure env",
			config: &BootstrapConfig{
				ProjectName:      "env-test-project",
				ProjectConfigDir: "/home/user/.beacon/config/projects/env-test-project",
				SecureEnvPath:    "/etc/beacon/test.env",
			},
			expectedCount: 2,
			expectedFiles: []string{
				"/home/user/.beacon/config/projects/env-test-project/env",
				"/etc/beacon/test.env",
			},
		},
		{
			name: "only secure env file",
			config: &BootstrapConfig{
				ProjectName:      "env-test-project",
				ProjectConfigDir: "/home/user/.beacon/config/projects/env-test-project",
				SecureEnvPath:    "/etc/beacon/test.env",
			},
			expectedCount: 2,
			expectedFiles: []string{
				"/home/user/.beacon/config/projects/env-test-project/env",
				"/etc/beacon/test.env",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse and execute the systemd template
			tmpl, err := template.New("systemd").Parse(systemdTemplate)
			if err != nil {
				t.Fatalf("Failed to parse systemd template: %v", err)
			}

			var result strings.Builder
			err = tmpl.Execute(&result, tt.config)
			if err != nil {
				t.Fatalf("Failed to execute systemd template: %v", err)
			}

			resultStr := result.String()

			// Count EnvironmentFile directives
			envFileCount := strings.Count(resultStr, "EnvironmentFile=")
			if envFileCount != tt.expectedCount {
				t.Errorf("Expected %d EnvironmentFile directives, got %d", tt.expectedCount, envFileCount)
			}

			// Check for expected environment files
			for _, expectedFile := range tt.expectedFiles {
				if !strings.Contains(resultStr, "EnvironmentFile="+expectedFile) {
					t.Errorf("Expected systemd service to contain EnvironmentFile=%s", expectedFile)
				}
			}
		})
	}
}

// TestSystemdServiceRestartBehavior tests systemd service restart behavior
func TestSystemdServiceRestartBehavior(t *testing.T) {
	config := &BootstrapConfig{
		ProjectName:      "restart-test-project",
		ProjectConfigDir: "/home/user/.beacon/config/projects/restart-test-project",
	}

	// Parse and execute the systemd template
	tmpl, err := template.New("systemd").Parse(systemdTemplate)
	if err != nil {
		t.Fatalf("Failed to parse systemd template: %v", err)
	}

	var result strings.Builder
	err = tmpl.Execute(&result, config)
	if err != nil {
		t.Fatalf("Failed to execute systemd template: %v", err)
	}

	resultStr := result.String()

	// Check restart behavior
	expectedRestartFields := []string{
		"Restart=always",
		"RestartSec=5",
	}

	for _, field := range expectedRestartFields {
		if !strings.Contains(resultStr, field) {
			t.Errorf("Expected systemd service to contain: %s", field)
		}
	}
}

// TestSystemdServiceLogging tests systemd service logging configuration
func TestSystemdServiceLogging(t *testing.T) {
	config := &BootstrapConfig{
		ProjectName:      "logging-test-project",
		ProjectConfigDir: "/home/user/.beacon/config/projects/logging-test-project",
	}

	// Parse and execute the systemd template
	tmpl, err := template.New("systemd").Parse(systemdTemplate)
	if err != nil {
		t.Fatalf("Failed to parse systemd template: %v", err)
	}

	var result strings.Builder
	err = tmpl.Execute(&result, config)
	if err != nil {
		t.Fatalf("Failed to execute systemd template: %v", err)
	}

	resultStr := result.String()

	// Check logging configuration
	expectedLoggingFields := []string{
		"StandardOutput=journal",
		"StandardError=journal",
	}

	for _, field := range expectedLoggingFields {
		if !strings.Contains(resultStr, field) {
			t.Errorf("Expected systemd service to contain: %s", field)
		}
	}
}
