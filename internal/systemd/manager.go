package systemd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

// ServiceType represents the type of systemd service (user or system)
type ServiceType int

const (
	UserService ServiceType = iota
	SystemService
)

// ServiceConfig holds configuration for creating a systemd service
type ServiceConfig struct {
	ProjectName     string
	ServiceType     ServiceType
	EnvironmentFile string
	WorkingDir      string
	ExecStart       string
	User            string
	Description     string
	RestartSec      int
}

// ServiceManager manages systemd services for Beacon projects
type ServiceManager struct {
	serviceType ServiceType
}

// NewServiceManager creates a new ServiceManager
func NewServiceManager(serviceType ServiceType) *ServiceManager {
	return &ServiceManager{
		serviceType: serviceType,
	}
}

// CreateService creates a systemd service file for a Beacon project
func (sm *ServiceManager) CreateService(config *ServiceConfig) error {
	servicePath := sm.getServicePath(config.ProjectName)
	
	// Ensure the directory exists
	serviceDir := filepath.Dir(servicePath)
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return fmt.Errorf("failed to create service directory: %v", err)
	}

	// Create the service file
	file, err := os.Create(servicePath)
	if err != nil {
		return fmt.Errorf("failed to create service file: %v", err)
	}
	defer file.Close()

	// Generate service content
	content, err := sm.generateServiceContent(config)
	if err != nil {
		return fmt.Errorf("failed to generate service content: %v", err)
	}

	if _, err := file.WriteString(content); err != nil {
		return fmt.Errorf("failed to write service file: %v", err)
	}

	// Set appropriate permissions
	if err := os.Chmod(servicePath, 0644); err != nil {
		return fmt.Errorf("failed to set service file permissions: %v", err)
	}

	return nil
}

// EnableService enables a systemd service
func (sm *ServiceManager) EnableService(projectName string) error {
	serviceName := fmt.Sprintf("beacon@%s.service", projectName)
	
	var cmd *exec.Cmd
	if sm.serviceType == UserService {
		cmd = exec.Command("systemctl", "--user", "enable", serviceName)
	} else {
		cmd = exec.Command("systemctl", "enable", serviceName)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable service: %v", err)
	}

	return nil
}

// StartService starts a systemd service
func (sm *ServiceManager) StartService(projectName string) error {
	serviceName := fmt.Sprintf("beacon@%s.service", projectName)
	
	var cmd *exec.Cmd
	if sm.serviceType == UserService {
		cmd = exec.Command("systemctl", "--user", "start", serviceName)
	} else {
		cmd = exec.Command("systemctl", "start", serviceName)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start service: %v", err)
	}

	return nil
}

// StopService stops a systemd service
func (sm *ServiceManager) StopService(projectName string) error {
	serviceName := fmt.Sprintf("beacon@%s.service", projectName)
	
	var cmd *exec.Cmd
	if sm.serviceType == UserService {
		cmd = exec.Command("systemctl", "--user", "stop", serviceName)
	} else {
		cmd = exec.Command("systemctl", "stop", serviceName)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop service: %v", err)
	}

	return nil
}

// RestartService restarts a systemd service
func (sm *ServiceManager) RestartService(projectName string) error {
	serviceName := fmt.Sprintf("beacon@%s.service", projectName)
	
	var cmd *exec.Cmd
	if sm.serviceType == UserService {
		cmd = exec.Command("systemctl", "--user", "restart", serviceName)
	} else {
		cmd = exec.Command("systemctl", "restart", serviceName)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to restart service: %v", err)
	}

	return nil
}

// DisableService disables a systemd service
func (sm *ServiceManager) DisableService(projectName string) error {
	serviceName := fmt.Sprintf("beacon@%s.service", projectName)
	
	var cmd *exec.Cmd
	if sm.serviceType == UserService {
		cmd = exec.Command("systemctl", "--user", "disable", serviceName)
	} else {
		cmd = exec.Command("systemctl", "disable", serviceName)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to disable service: %v", err)
	}

	return nil
}

// GetServiceStatus returns the status of a systemd service
func (sm *ServiceManager) GetServiceStatus(projectName string) (string, error) {
	serviceName := fmt.Sprintf("beacon@%s.service", projectName)
	
	var cmd *exec.Cmd
	if sm.serviceType == UserService {
		cmd = exec.Command("systemctl", "--user", "is-active", serviceName)
	} else {
		cmd = exec.Command("systemctl", "is-active", serviceName)
	}

	output, err := cmd.Output()
	if err != nil {
		return "inactive", nil // Service doesn't exist or is inactive
	}

	return strings.TrimSpace(string(output)), nil
}

// RemoveService removes a systemd service file
func (sm *ServiceManager) RemoveService(projectName string) error {
	servicePath := sm.getServicePath(projectName)
	
	if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service file: %v", err)
	}

	return nil
}

// ReloadDaemon reloads the systemd daemon
func (sm *ServiceManager) ReloadDaemon() error {
	var cmd *exec.Cmd
	if sm.serviceType == UserService {
		cmd = exec.Command("systemctl", "--user", "daemon-reload")
	} else {
		cmd = exec.Command("systemctl", "daemon-reload")
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reload daemon: %v", err)
	}

	return nil
}

// IsAvailable checks if systemd is available
func (sm *ServiceManager) IsAvailable() bool {
	var cmd *exec.Cmd
	if sm.serviceType == UserService {
		cmd = exec.Command("systemctl", "--user", "status")
	} else {
		cmd = exec.Command("systemctl", "status")
	}

	return cmd.Run() == nil
}

// getServicePath returns the path where the service file should be created
func (sm *ServiceManager) getServicePath(projectName string) string {
	serviceName := fmt.Sprintf("beacon@%s.service", projectName)
	
	if sm.serviceType == UserService {
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, ".config", "systemd", "user", serviceName)
	} else {
		return filepath.Join("/etc/systemd/system", serviceName)
	}
}

// generateServiceContent generates the systemd service file content
func (sm *ServiceManager) generateServiceContent(config *ServiceConfig) (string, error) {
	tmpl := `[Unit]
Description={{.Description}}
After=network.target

[Service]
EnvironmentFile={{.EnvironmentFile}}
Type=simple
ExecStart={{.ExecStart}}
WorkingDirectory={{.WorkingDir}}
Restart=always
RestartSec={{.RestartSec}}
{{if .User}}User={{.User}}{{end}}

# Logging
StandardOutput=journal
StandardError=journal

[Install]
{{if eq .ServiceType 0}}WantedBy=default.target{{else}}WantedBy=multi-user.target{{end}}
`

	t, err := template.New("systemd").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	err = t.Execute(&buf, struct {
		*ServiceConfig
		ServiceType ServiceType
	}{
		ServiceConfig: config,
		ServiceType:   sm.serviceType,
	})

	return buf.String(), err
}

// GetDefaultServiceConfig returns a default service configuration
func GetDefaultServiceConfig(projectName, environmentFile, workingDir string) *ServiceConfig {
	return &ServiceConfig{
		ProjectName:     projectName,
		ServiceType:     UserService, // Default to user service
		EnvironmentFile: environmentFile,
		WorkingDir:      workingDir,
		ExecStart:       "/usr/local/bin/beacon deploy",
		Description:     fmt.Sprintf("Beacon Agent for %s - Lightweight deployment and monitoring", projectName),
		RestartSec:      5,
	}
}
