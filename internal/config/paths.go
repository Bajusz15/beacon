package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BeaconPaths manages all Beacon-related paths in a unified way
type BeaconPaths struct {
	BaseDir       string // ~/.beacon
	ConfigDir     string // ~/.beacon/config
	ProjectsDir   string // ~/.beacon/config/projects
	TemplatesDir  string // ~/.beacon/templates (shared templates)
	LogsDir       string // ~/.beacon/logs (project-specific logs)
	SystemdDir    string // ~/.config/systemd/user (for user services)
	SystemdDirSys string // /etc/systemd/system (for system services)
	WorkingDir    string // ~/beacon (default working directory)
}

// NewBeaconPaths creates a new BeaconPaths instance with all paths resolved
func NewBeaconPaths() (*BeaconPaths, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %v", err)
	}

	paths := &BeaconPaths{
		BaseDir:       filepath.Join(homeDir, ".beacon"),
		ConfigDir:     filepath.Join(homeDir, ".beacon", "config"),
		ProjectsDir:   filepath.Join(homeDir, ".beacon", "config", "projects"),
		TemplatesDir:  filepath.Join(homeDir, ".beacon", "templates"),
		LogsDir:       filepath.Join(homeDir, ".beacon", "logs"),
		SystemdDir:    filepath.Join(homeDir, ".config", "systemd", "user"),
		SystemdDirSys: "/etc/systemd/system",
		WorkingDir:    filepath.Join(homeDir, "beacon"),
	}

	return paths, nil
}

// EnsureDirectories creates all necessary directories
func (bp *BeaconPaths) EnsureDirectories() error {
	dirs := []string{
		bp.BaseDir,
		bp.ConfigDir,
		bp.ProjectsDir,
		bp.TemplatesDir,
		bp.LogsDir,
		bp.SystemdDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}

	return nil
}

// GetProjectConfigDir returns the configuration directory for a specific project
func (bp *BeaconPaths) GetProjectConfigDir(projectName string) string {
	return filepath.Join(bp.ProjectsDir, projectName)
}

// GetProjectEnvFile returns the environment file path for a specific project
func (bp *BeaconPaths) GetProjectEnvFile(projectName string) string {
	return filepath.Join(bp.GetProjectConfigDir(projectName), "env")
}

// GetProjectMonitorFile returns the monitor configuration file path for a specific project
func (bp *BeaconPaths) GetProjectMonitorFile(projectName string) string {
	return filepath.Join(bp.GetProjectConfigDir(projectName), "monitor.yml")
}

// GetProjectAlertsFile returns the alerts configuration file path for a specific project
func (bp *BeaconPaths) GetProjectAlertsFile(projectName string) string {
	return filepath.Join(bp.GetProjectConfigDir(projectName), "alerts.yml")
}

// GetProjectKeysDir returns the keys directory for a specific project
func (bp *BeaconPaths) GetProjectKeysDir(projectName string) string {
	return filepath.Join(bp.GetProjectConfigDir(projectName), "keys")
}

// GetProjectLogsDir returns the logs directory for a specific project
func (bp *BeaconPaths) GetProjectLogsDir(projectName string) string {
	return filepath.Join(bp.LogsDir, projectName)
}

// GetProjectWorkingDir returns the working directory for a specific project
func (bp *BeaconPaths) GetProjectWorkingDir(projectName string) string {
	return filepath.Join(bp.WorkingDir, projectName)
}

// GetSystemdServiceFile returns the systemd service file path for a specific project
func (bp *BeaconPaths) GetSystemdServiceFile(projectName string, useSystemService bool) string {
	if useSystemService {
		return filepath.Join(bp.SystemdDirSys, fmt.Sprintf("beacon@%s.service", projectName))
	}
	return filepath.Join(bp.SystemdDir, fmt.Sprintf("beacon@%s.service", projectName))
}

// GetMasterKeyFile returns the master key file path for encryption
func (bp *BeaconPaths) GetMasterKeyFile() string {
	return filepath.Join(bp.BaseDir, ".master_key")
}

// ValidateProjectName validates a project name for filesystem safety
func (bp *BeaconPaths) ValidateProjectName(name string) error {
	if name == "" {
		return fmt.Errorf("project name cannot be empty")
	}

	// Check for invalid characters
	for _, char := range name {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_') {
			return fmt.Errorf("project name contains invalid character '%c'. Only letters, numbers, hyphens, and underscores are allowed", char)
		}
	}

	// Check for reserved names
	reserved := []string{"config", "templates", "keys", "logs", "systemd"}
	for _, reserved := range reserved {
		if strings.EqualFold(name, reserved) {
			return fmt.Errorf("project name '%s' is reserved", name)
		}
	}

	return nil
}

// CreateProjectStructure creates the directory structure for a new project
func (bp *BeaconPaths) CreateProjectStructure(projectName string) error {
	if err := bp.ValidateProjectName(projectName); err != nil {
		return err
	}

	// Create project config directory
	projectConfigDir := bp.GetProjectConfigDir(projectName)
	if err := os.MkdirAll(projectConfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create project config directory: %v", err)
	}

	// Create project keys directory
	projectKeysDir := bp.GetProjectKeysDir(projectName)
	if err := os.MkdirAll(projectKeysDir, 0755); err != nil {
		return fmt.Errorf("failed to create project keys directory: %v", err)
	}

	// Create project logs directory
	projectLogsDir := bp.GetProjectLogsDir(projectName)
	if err := os.MkdirAll(projectLogsDir, 0755); err != nil {
		return fmt.Errorf("failed to create project logs directory: %v", err)
	}

	// Create project working directory
	projectWorkingDir := bp.GetProjectWorkingDir(projectName)
	if err := os.MkdirAll(projectWorkingDir, 0755); err != nil {
		return fmt.Errorf("failed to create project working directory: %v", err)
	}

	return nil
}

// ListProjects returns a list of all configured projects
func (bp *BeaconPaths) ListProjects() ([]string, error) {
	entries, err := os.ReadDir(bp.ProjectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read projects directory: %v", err)
	}

	var projects []string
	for _, entry := range entries {
		if entry.IsDir() {
			projects = append(projects, entry.Name())
		}
	}

	return projects, nil
}

// ProjectExists checks if a project exists
func (bp *BeaconPaths) ProjectExists(projectName string) bool {
	projectConfigDir := bp.GetProjectConfigDir(projectName)
	_, err := os.Stat(projectConfigDir)
	return err == nil
}

// RemoveProject removes a project and all its associated files
func (bp *BeaconPaths) RemoveProject(projectName string) error {
	if err := bp.ValidateProjectName(projectName); err != nil {
		return err
	}

	// Remove project config directory (includes keys)
	projectConfigDir := bp.GetProjectConfigDir(projectName)
	if err := os.RemoveAll(projectConfigDir); err != nil {
		return fmt.Errorf("failed to remove project config directory: %v", err)
	}

	// Remove project logs directory
	projectLogsDir := bp.GetProjectLogsDir(projectName)
	if err := os.RemoveAll(projectLogsDir); err != nil {
		return fmt.Errorf("failed to remove project logs directory: %v", err)
	}

	// Remove project working directory
	projectWorkingDir := bp.GetProjectWorkingDir(projectName)
	if err := os.RemoveAll(projectWorkingDir); err != nil {
		return fmt.Errorf("failed to remove project working directory: %v", err)
	}

	// Remove systemd service files (both user and system)
	userServiceFile := bp.GetSystemdServiceFile(projectName, false)
	if err := os.Remove(userServiceFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove user systemd service: %v", err)
	}

	systemServiceFile := bp.GetSystemdServiceFile(projectName, true)
	if err := os.Remove(systemServiceFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove system systemd service: %v", err)
	}

	return nil
}

// GetRelativePath returns a path relative to the base directory for display purposes
func (bp *BeaconPaths) GetRelativePath(fullPath string) string {
	if strings.HasPrefix(fullPath, bp.BaseDir) {
		return strings.TrimPrefix(fullPath, bp.BaseDir+"/")
	}
	return fullPath
}

// String returns a string representation of all paths for debugging
func (bp *BeaconPaths) String() string {
	return fmt.Sprintf(`BeaconPaths:
  BaseDir:       %s
  ConfigDir:     %s
  ProjectsDir:   %s
  TemplatesDir:  %s
  LogsDir:       %s
  SystemdDir:    %s
  SystemdDirSys: %s
  WorkingDir:    %s`,
		bp.BaseDir,
		bp.ConfigDir,
		bp.ProjectsDir,
		bp.TemplatesDir,
		bp.LogsDir,
		bp.SystemdDir,
		bp.SystemdDirSys,
		bp.WorkingDir,
	)
}
