package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// ConfigHierarchy manages the hierarchy of global vs project-specific configurations
type ConfigHierarchy struct {
	paths *BeaconPaths
}

// NewConfigHierarchy creates a new configuration hierarchy manager
func NewConfigHierarchy() (*ConfigHierarchy, error) {
	paths, err := NewBeaconPaths()
	if err != nil {
		return nil, err
	}

	return &ConfigHierarchy{
		paths: paths,
	}, nil
}

// ConfigType represents the type of configuration
type ConfigType int

const (
	AlertsConfig ConfigType = iota
	MonitorConfig
	KeysConfig
)

// GetConfigPath returns the appropriate config path based on hierarchy rules
func (ch *ConfigHierarchy) GetConfigPath(configType ConfigType, projectName string) string {
	switch configType {
	case AlertsConfig:
		// Alerts are always project-specific
		if projectName == "" {
			return "" // No global alerts
		}
		return ch.paths.GetProjectAlertsFile(projectName)

	case MonitorConfig:
		// Monitor configs are always project-specific
		if projectName == "" {
			return "" // No global monitor config
		}
		return ch.paths.GetProjectMonitorFile(projectName)

	case KeysConfig:
		// Keys are always project-specific
		if projectName == "" {
			return "" // No global keys
		}
		return filepath.Join(ch.paths.GetProjectKeysDir(projectName), "keys.yml")

	default:
		return ""
	}
}

// LoadConfig loads configuration with hierarchy support
func (ch *ConfigHierarchy) LoadConfig(configType ConfigType, projectName string, target interface{}) error {
	configPath := ch.GetConfigPath(configType, projectName)

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("configuration file not found: %s", configPath)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}

	if err := yaml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to unmarshal config: %v", err)
	}

	return nil
}

// SaveConfig saves configuration to the appropriate location
func (ch *ConfigHierarchy) SaveConfig(configType ConfigType, projectName string, config interface{}, forceGlobal bool) error {
	if projectName == "" {
		return fmt.Errorf("project name is required for all configuration types")
	}

	var configPath string

	switch configType {
	case AlertsConfig:
		configPath = ch.paths.GetProjectAlertsFile(projectName)
	case MonitorConfig:
		configPath = ch.paths.GetProjectMonitorFile(projectName)
	case KeysConfig:
		configPath = filepath.Join(ch.paths.GetProjectKeysDir(projectName), "keys.yml")
	default:
		return fmt.Errorf("unsupported config type")
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	return nil
}

// ListConfigFiles returns all configuration files for a given type
func (ch *ConfigHierarchy) ListConfigFiles(configType ConfigType) ([]string, error) {
	var files []string

	// Get all projects
	projects, err := ch.paths.ListProjects()
	if err != nil {
		return files, err
	}

	// Add project-specific configs
	for _, project := range projects {
		var projectPath string
		switch configType {
		case AlertsConfig:
			projectPath = ch.paths.GetProjectAlertsFile(project)
		case MonitorConfig:
			projectPath = ch.paths.GetProjectMonitorFile(project)
		case KeysConfig:
			projectPath = filepath.Join(ch.paths.GetProjectKeysDir(project), "keys.yml")
		}

		if _, err := os.Stat(projectPath); err == nil {
			files = append(files, projectPath)
		}
	}

	return files, nil
}

// GetConfigInfo returns information about a configuration file
func (ch *ConfigHierarchy) GetConfigInfo(configPath string) (*ConfigInfo, error) {
	stat, err := os.Stat(configPath)
	if err != nil {
		return nil, err
	}

	info := &ConfigInfo{
		Path:     configPath,
		Size:     stat.Size(),
		Modified: stat.ModTime(),
		IsGlobal: false, // All configs are now project-specific
		Project:  "",
	}

	// Determine project name from path
	if filepath.Dir(filepath.Dir(configPath)) == ch.paths.ProjectsDir {
		info.Project = filepath.Base(filepath.Dir(configPath))
	}

	return info, nil
}

// ConfigInfo holds information about a configuration file
type ConfigInfo struct {
	Path     string
	Size     int64
	Modified time.Time
	IsGlobal bool
	Project  string
}

// ExplainHierarchy returns a human-readable explanation of the configuration hierarchy
func (ch *ConfigHierarchy) ExplainHierarchy() string {
	return `Beacon Configuration Hierarchy:

🔧 SHARED (Global) - Used by all projects:
   • ~/.beacon/templates/      - Alert templates (Email, Webhook, etc.)

📁 PER-PROJECT - Each project has its own:
   • ~/.beacon/config/projects/{project}/env        - Environment variables
   • ~/.beacon/config/projects/{project}/monitor.yml - Monitoring config
   • ~/.beacon/config/projects/{project}/alerts.yml   - Alert routing
   • ~/.beacon/config/projects/{project}/keys/        - API keys (BeaconInfra, etc.)
   • ~/.beacon/logs/{project}/                       - Project-specific logs
   • ~/beacon/{project}/                             - Working directory

🎯 DESIGN PRINCIPLES:
   1. Each project is completely isolated
   2. Projects can have multiple devices/keys
   3. Different projects can use different BeaconInfra accounts
   4. Alert templates are shared for consistency
   5. No global configuration - everything is project-specific

💡 EXAMPLES:
   • Project "home-server" → Own keys, alerts, logs
   • Project "iot-sensors" → Different keys, different alerts
   • Shared templates → Same Email/Webhook formatting across projects
   • Multiple devices per project → Multiple keys in project/keys/`
}
