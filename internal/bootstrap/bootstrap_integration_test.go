package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"beacon/internal/config"
)

// TestBootstrapProject_FullIntegration tests the complete bootstrap process
func TestBootstrapProject_FullIntegration(t *testing.T) {
	bm, err := NewBootstrapManager(false)
	if err != nil {
		t.Fatalf("Failed to create bootstrap manager: %v", err)
	}

	tempDir := t.TempDir()
	projectName := "integration-test-project"

	// Create a simple config
	configContent := `project_name: ` + projectName + `
repo_url: https://github.com/testuser/testrepo.git
local_path: ` + tempDir + `/working
poll_interval: 60s
port: "8080"
ssh_key_path: ""
git_token: ""
`

	// Create config file
	configFile := filepath.Join(tempDir, "config.yml")
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Test BootstrapProjectFromConfig
	err = bm.BootstrapProjectFromConfig(projectName, configFile, true) // skip systemd
	if err != nil {
		t.Fatalf("Failed to bootstrap project from config: %v", err)
	}

	// Verify all components were created
	expectedPaths := []string{
		bm.paths.GetProjectConfigDir(projectName),
		bm.paths.GetProjectKeysDir(projectName),
		bm.paths.GetProjectLogsDir(projectName),
		bm.paths.GetProjectWorkingDir(projectName),
		bm.paths.GetProjectEnvFile(projectName),
	}

	for _, path := range expectedPaths {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected path %s to exist", path)
		}
	}

	// Verify project exists
	if !bm.paths.ProjectExists(projectName) {
		t.Error("Expected project to exist")
	}
}

// TestBootstrapProject_DirectoryStructure tests complete directory structure creation
func TestBootstrapProject_DirectoryStructure(t *testing.T) {
	paths, err := config.NewBeaconPaths()
	if err != nil {
		t.Fatalf("Failed to create paths: %v", err)
	}

	tempDir := t.TempDir()
	paths.BaseDir = tempDir
	paths.ConfigDir = filepath.Join(tempDir, "config")
	paths.ProjectsDir = filepath.Join(tempDir, "config", "projects")
	paths.LogsDir = filepath.Join(tempDir, "logs")
	paths.WorkingDir = filepath.Join(tempDir, "working")

	projectName := "structure-test-project"

	err = paths.CreateProjectStructure(projectName)
	if err != nil {
		t.Fatalf("Failed to create project structure: %v", err)
	}

	// Verify all expected directories exist
	expectedDirs := []string{
		paths.GetProjectConfigDir(projectName),
		paths.GetProjectKeysDir(projectName),
		paths.GetProjectLogsDir(projectName),
		paths.GetProjectWorkingDir(projectName),
	}

	for _, dir := range expectedDirs {
		info, err := os.Stat(dir)
		if os.IsNotExist(err) {
			t.Errorf("Expected directory %s to exist", dir)
		}
		if !info.IsDir() {
			t.Errorf("Expected %s to be a directory", dir)
		}
	}
}

// TestBootstrapProject_ListProjects tests project listing functionality
func TestBootstrapProject_ListProjects(t *testing.T) {
	paths, err := config.NewBeaconPaths()
	if err != nil {
		t.Fatalf("Failed to create paths: %v", err)
	}

	tempDir := t.TempDir()
	paths.BaseDir = tempDir
	paths.ConfigDir = filepath.Join(tempDir, "config")
	paths.ProjectsDir = filepath.Join(tempDir, "config", "projects")

	projectName1 := "project1"
	projectName2 := "project2"

	// Create multiple projects
	if err := paths.CreateProjectStructure(projectName1); err != nil {
		t.Fatalf("Failed to create project1: %v", err)
	}
	if err := paths.CreateProjectStructure(projectName2); err != nil {
		t.Fatalf("Failed to create project2: %v", err)
	}

	// List projects
	projects, err := paths.ListProjects()
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}

	// Verify projects were found
	if len(projects) < 2 {
		t.Errorf("Expected at least 2 projects, got %d", len(projects))
	}

	// Check for both projects in the list
	found1, found2 := false, false
	for _, proj := range projects {
		if proj == projectName1 {
			found1 = true
		}
		if proj == projectName2 {
			found2 = true
		}
	}

	if !found1 {
		t.Error("Expected to find project1 in list")
	}
	if !found2 {
		t.Error("Expected to find project2 in list")
	}
}

// TestBootstrapProject_RemoveProject tests project removal
func TestBootstrapProject_RemoveProject(t *testing.T) {
	paths, err := config.NewBeaconPaths()
	if err != nil {
		t.Fatalf("Failed to create paths: %v", err)
	}

	tempDir := t.TempDir()
	paths.BaseDir = tempDir
	paths.ConfigDir = filepath.Join(tempDir, "config")
	paths.ProjectsDir = filepath.Join(tempDir, "config", "projects")
	paths.LogsDir = filepath.Join(tempDir, "logs")
	paths.WorkingDir = filepath.Join(tempDir, "working")

	projectName := "remove-test-project"

	// Create project
	if err := paths.CreateProjectStructure(projectName); err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	// Verify it exists
	if !paths.ProjectExists(projectName) {
		t.Fatal("Expected project to exist before removal")
	}

	// Remove project
	err = paths.RemoveProject(projectName)
	if err != nil {
		t.Fatalf("Failed to remove project: %v", err)
	}

	// Verify it no longer exists
	if paths.ProjectExists(projectName) {
		t.Error("Expected project to not exist after removal")
	}
}

// TestBootstrapProject_EnvironmentFileContent tests environment file content generation
func TestBootstrapProject_EnvironmentFileContent(t *testing.T) {
	bm, err := NewBootstrapManager(false)
	if err != nil {
		t.Fatalf("Failed to create bootstrap manager: %v", err)
	}

	tempDir := t.TempDir()
	projectName := "env-content-test"

	// Create project structure
	if err := bm.paths.CreateProjectStructure(projectName); err != nil {
		t.Fatalf("Failed to create project structure: %v", err)
	}

	// Test with optional fields
	config := &BootstrapConfig{
		ProjectName:   projectName,
		RepoURL:       "git@github.com:user/repo.git",
		LocalPath:     "/tmp/deploy",
		DeployCommand: "npm run deploy",
		PollInterval:  "30s",
		Port:          "3000",
		SSHKeyPath:    "/home/user/.ssh/id_rsa",
		GitToken:      "secret-token",
		SecureEnvPath: "/secure/path",
		User:          "testuser",
		WorkingDir:    filepath.Join(tempDir, "working"),
	}

	err = bm.createEnvironmentFile(config)
	if err != nil {
		t.Fatalf("Failed to create environment file: %v", err)
	}

	// Read and verify content
	envPath := bm.paths.GetProjectEnvFile(projectName)
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read environment file: %v", err)
	}

	contentStr := string(content)

	// Check for required variables
	requiredVars := []string{
		"BEACON_REPO_URL",
		"BEACON_LOCAL_PATH",
		"BEACON_POLL_INTERVAL",
		"BEACON_PORT",
		"BEACON_PROJECT_NAME",
		"BEACON_WORKING_DIR",
	}

	for _, varName := range requiredVars {
		if !contains(contentStr, varName) {
			t.Errorf("Expected environment file to contain %s", varName)
		}
	}

	// Check for optional variables that were provided
	optionalVars := map[string]string{
		"BEACON_SSH_KEY_PATH":    "/home/user/.ssh/id_rsa",
		"BEACON_GIT_TOKEN":       "secret-token",
		"BEACON_DEPLOY_COMMAND":  "npm run deploy",
		"BEACON_SECURE_ENV_PATH": "/secure/path",
	}

	for varName, expectedValue := range optionalVars {
		if !contains(contentStr, varName) {
			t.Errorf("Expected environment file to contain %s", varName)
		}
		if !contains(contentStr, expectedValue) {
			t.Errorf("Expected environment file to contain value %s for %s", expectedValue, varName)
		}
	}
}
