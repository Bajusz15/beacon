package wizard

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"beacon/internal/monitor"
	"beacon/internal/plugins"
)

// Wizard handles interactive configuration setup
type Wizard struct {
	configPath    string
	envPath       string
	bootstrapPath string
	deviceName    string
	deviceType    string
}

// NewWizard creates a new configuration wizard
func NewWizard(configPath, envPath string) *Wizard {
	// Generate bootstrap config path from monitor config path
	// Default to current directory
	bootstrapPath := "beacon.bootstrap.yml"
	if configPath != "" {
		// Use same directory as monitor config
		dir := filepath.Dir(configPath)
		if dir != "." && dir != "" && dir != configPath {
			// If configPath has a directory component, use it
			bootstrapPath = filepath.Join(dir, "beacon.bootstrap.yml")
		}
		// If dir is "." or same as configPath, bootstrapPath stays as "beacon.bootstrap.yml"
	}

	return &Wizard{
		configPath:    configPath,
		envPath:       envPath,
		bootstrapPath: bootstrapPath,
	}
}

// Run starts the interactive configuration wizard
func (w *Wizard) Run() error {
	fmt.Println("🚀 Welcome to Beacon Configuration Wizard!")
	fmt.Println("This wizard will help you set up monitoring for your device.")
	fmt.Println()

	// Get device information
	if err := w.getDeviceInfo(); err != nil {
		return err
	}

	// Select template
	template, err := w.selectTemplate()
	if err != nil {
		return err
	}

	// Configure monitoring
	checks, err := w.configureMonitoring(template)
	if err != nil {
		return err
	}

	// Configure plugins
	pluginConfigs, alertRules, err := w.configurePlugins()
	if err != nil {
		return err
	}

	// Configure reporting
	reportConfig, err := w.configureReporting()
	if err != nil {
		return err
	}

	// Generate configuration files
	if err := w.generateConfig(checks, pluginConfigs, alertRules, reportConfig); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("✅ Configuration complete!")
	fmt.Printf("📁 Monitor config: %s\n", w.configPath)
	fmt.Printf("🔧 Environment file: %s\n", w.envPath)
	fmt.Printf("🚀 Bootstrap config: %s\n", w.bootstrapPath)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("1. Review the generated configuration files")
	fmt.Println("2. Set up your environment variables in the .env file")
	fmt.Println("3. Customize bootstrap config for your project (repo URL, deploy command, etc.)")
	fmt.Println("4. Run: beacon bootstrap myproject -f " + w.bootstrapPath)
	fmt.Println("5. Run: beacon monitor -f " + w.configPath)
	fmt.Println()

	return nil
}

// getDeviceInfo collects basic device information
func (w *Wizard) getDeviceInfo() error {
	fmt.Println("📱 Device Information")
	fmt.Println("====================")

	// Device name
	fmt.Print("Device name (e.g., 'raspberry-pi', 'web-server'): ")
	reader := bufio.NewReader(os.Stdin)
	name, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	w.deviceName = strings.TrimSpace(name)
	if w.deviceName == "" {
		w.deviceName = "beacon-device"
	}

	// Device type
	fmt.Println()
	fmt.Println("Device type:")
	fmt.Println("1. Raspberry Pi / IoT Device")
	fmt.Println("2. Web Server / Application")
	fmt.Println("3. Docker Container Host")
	fmt.Println("4. Database Server")
	fmt.Println("5. Custom")
	fmt.Print("Select (1-5): ")

	choice, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	choice = strings.TrimSpace(choice)

	switch choice {
	case "1":
		w.deviceType = "raspberry-pi"
	case "2":
		w.deviceType = "web-server"
	case "3":
		w.deviceType = "docker-host"
	case "4":
		w.deviceType = "database-server"
	case "5":
		w.deviceType = "custom"
	default:
		w.deviceType = "custom"
	}

	fmt.Println()
	return nil
}

// selectTemplate helps user select a configuration template
func (w *Wizard) selectTemplate() (*Template, error) {
	fmt.Println("📋 Configuration Template")
	fmt.Println("========================")

	templates := getTemplates()
	for i, template := range templates {
		fmt.Printf("%d. %s\n", i+1, template.Name)
		fmt.Printf("   %s\n", template.Description)
		fmt.Println()
	}

	fmt.Print("Select template (1-", len(templates), "): ")
	reader := bufio.NewReader(os.Stdin)
	choice, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	choiceNum, err := strconv.Atoi(strings.TrimSpace(choice))
	if err != nil || choiceNum < 1 || choiceNum > len(templates) {
		choiceNum = 1 // Default to first template
	}

	return templates[choiceNum-1], nil
}

// configureMonitoring sets up monitoring checks based on template
func (w *Wizard) configureMonitoring(template *Template) ([]monitor.CheckConfig, error) {
	fmt.Println("🔍 Monitoring Configuration")
	fmt.Println("===========================")

	var checks []monitor.CheckConfig

	// Add template-based checks
	for _, templateCheck := range template.Checks {
		fmt.Printf("Configure %s check? (y/n) [y]: ", templateCheck.Name)
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response == "" || response == "y" || response == "yes" {
			check, err := w.configureCheck(templateCheck)
			if err != nil {
				return nil, err
			}
			checks = append(checks, check)
		}
	}

	// Ask for additional checks
	fmt.Println()
	fmt.Print("Add additional custom checks? (y/n) [n]: ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response == "y" || response == "yes" {
		for {
			check, err := w.addCustomCheck()
			if err != nil {
				return nil, err
			}
			if check == nil {
				break // User chose to stop
			}
			checks = append(checks, *check)
		}
	}

	return checks, nil
}

// configureCheck configures a specific check
func (w *Wizard) configureCheck(templateCheck TemplateCheck) (monitor.CheckConfig, error) {
	check := monitor.CheckConfig{
		Name:     templateCheck.Name,
		Type:     templateCheck.Type,
		Interval: templateCheck.Interval,
	}

	fmt.Printf("Configuring %s check:\n", check.Name)

	reader := bufio.NewReader(os.Stdin)

	switch check.Type {
	case "http":
		fmt.Printf("URL [%s]: ", templateCheck.URL)
		url, err := reader.ReadString('\n')
		if err != nil {
			return check, err
		}
		url = strings.TrimSpace(url)
		if url == "" {
			url = templateCheck.URL
		}
		check.URL = url

		fmt.Printf("Expected status code [%d]: ", templateCheck.ExpectedStatus)
		statusStr, err := reader.ReadString('\n')
		if err != nil {
			return check, err
		}
		statusStr = strings.TrimSpace(statusStr)
		if statusStr != "" {
			if status, err := strconv.Atoi(statusStr); err == nil {
				check.ExpectStatus = status
			} else {
				check.ExpectStatus = templateCheck.ExpectedStatus
			}
		} else {
			check.ExpectStatus = templateCheck.ExpectedStatus
		}

	case "port":
		fmt.Printf("Host [%s]: ", templateCheck.Host)
		host, err := reader.ReadString('\n')
		if err != nil {
			return check, err
		}
		host = strings.TrimSpace(host)
		if host == "" {
			host = templateCheck.Host
		}
		check.Host = host

		fmt.Printf("Port [%d]: ", templateCheck.Port)
		portStr, err := reader.ReadString('\n')
		if err != nil {
			return check, err
		}
		portStr = strings.TrimSpace(portStr)
		if portStr != "" {
			if port, err := strconv.Atoi(portStr); err == nil {
				check.Port = port
			} else {
				check.Port = templateCheck.Port
			}
		} else {
			check.Port = templateCheck.Port
		}

	case "command":
		fmt.Printf("Command [%s]: ", templateCheck.Command)
		cmd, err := reader.ReadString('\n')
		if err != nil {
			return check, err
		}
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			cmd = templateCheck.Command
		}
		check.Cmd = cmd
	}

	fmt.Printf("Check interval [%s]: ", check.Interval.String())
	intervalStr, err := reader.ReadString('\n')
	if err != nil {
		return check, err
	}
	intervalStr = strings.TrimSpace(intervalStr)
	if intervalStr != "" {
		if interval, err := time.ParseDuration(intervalStr); err == nil {
			check.Interval = interval
		}
	}

	fmt.Println()
	return check, nil
}

// addCustomCheck allows user to add a custom check
func (w *Wizard) addCustomCheck() (*monitor.CheckConfig, error) {
	fmt.Print("Add another check? (y/n) [n]: ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		return nil, nil
	}

	fmt.Println()
	fmt.Print("Check name: ")
	name, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)

	fmt.Println("Check type:")
	fmt.Println("1. HTTP")
	fmt.Println("2. Port")
	fmt.Println("3. Command")
	fmt.Print("Select (1-3): ")

	typeChoice, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	typeChoice = strings.TrimSpace(typeChoice)

	check := monitor.CheckConfig{
		Name:     name,
		Interval: 30 * time.Second,
	}

	switch typeChoice {
	case "1":
		check.Type = "http"
		fmt.Print("URL: ")
		url, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		check.URL = strings.TrimSpace(url)

		fmt.Print("Expected status code [200]: ")
		statusStr, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		statusStr = strings.TrimSpace(statusStr)
		if statusStr != "" {
			if status, err := strconv.Atoi(statusStr); err == nil {
				check.ExpectStatus = status
			} else {
				check.ExpectStatus = 200
			}
		} else {
			check.ExpectStatus = 200
		}

	case "2":
		check.Type = "port"
		fmt.Print("Host [localhost]: ")
		host, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		host = strings.TrimSpace(host)
		if host == "" {
			host = "localhost"
		}
		check.Host = host

		fmt.Print("Port: ")
		portStr, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		portStr = strings.TrimSpace(portStr)
		if port, err := strconv.Atoi(portStr); err == nil {
			check.Port = port
		}

	case "3":
		check.Type = "command"
		fmt.Print("Command: ")
		cmd, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		check.Cmd = strings.TrimSpace(cmd)

	default:
		return nil, fmt.Errorf("invalid check type")
	}

	fmt.Println()
	return &check, nil
}

// configurePlugins sets up alert plugins
func (w *Wizard) configurePlugins() ([]plugins.PluginConfig, []plugins.AlertRule, error) {
	fmt.Println("🔔 Alert Configuration")
	fmt.Println("======================")

	var pluginConfigs []plugins.PluginConfig
	var alertRules []plugins.AlertRule

	fmt.Print("Enable alert notifications? (y/n) [y]: ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return nil, nil, err
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response == "" || response == "y" || response == "yes" {
		// Configure Email
		if w.configureEmail() {
			pluginConfigs = append(pluginConfigs, plugins.PluginConfig{
				Name:    "email",
				Enabled: true,
				Config: map[string]interface{}{
					"smtp_host": "${SMTP_HOST}",
					"smtp_port": "${SMTP_PORT}",
					"smtp_user": "${SMTP_USER}",
					"smtp_pass": "${SMTP_PASSWORD}",
					"from":      "${SMTP_FROM}",
					"to":        []string{"${SMTP_TO}"},
				},
			})
		}

		// Create default alert rules
		if len(pluginConfigs) > 0 {
			alertRules = []plugins.AlertRule{
				{
					Check:    "*", // All checks
					Severity: "critical",
					Plugins:  w.getPluginNames(pluginConfigs),
					Cooldown: "5m",
				},
			}
		}
	}

	fmt.Println()
	return pluginConfigs, alertRules, nil
}

// configureEmail asks user if they want to configure Email
func (w *Wizard) configureEmail() bool {
	fmt.Print("Configure Email notifications? (y/n) [n]: ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

// getPluginNames extracts plugin names from configs
func (w *Wizard) getPluginNames(configs []plugins.PluginConfig) []string {
	var names []string
	for _, config := range configs {
		names = append(names, config.Name)
	}
	return names
}

// configureReporting sets up BeaconInfra reporting
func (w *Wizard) configureReporting() (monitor.ReportConfig, error) {
	fmt.Println("📊 Reporting Configuration")
	fmt.Println("==========================")

	reportConfig := monitor.ReportConfig{
		SendTo: "https://beaconinfra.com/api/v1/report",
		Token:  "${BEACONINFRA_TOKEN}",
	}

	fmt.Print("Enable BeaconInfra reporting? (y/n) [y]: ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return reportConfig, err
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response == "" || response == "y" || response == "yes" {
		fmt.Println("✅ BeaconInfra reporting enabled")
		fmt.Println("   You'll need to set BEACONINFRA_TOKEN environment variable")
	} else {
		reportConfig.SendTo = ""
		reportConfig.Token = ""
		fmt.Println("❌ BeaconInfra reporting disabled")
	}

	fmt.Println()
	return reportConfig, nil
}

// generateConfig creates the configuration files
func (w *Wizard) generateConfig(checks []monitor.CheckConfig, pluginConfigs []plugins.PluginConfig, alertRules []plugins.AlertRule, reportConfig monitor.ReportConfig) error {
	// Generate monitor config
	monitorConfig := monitor.Config{
		Device: monitor.DeviceConfig{
			Name:        w.deviceName,
			Location:    "Unknown",
			Tags:        []string{w.deviceType},
			Environment: "production",
		},
		Checks:     checks,
		Plugins:    pluginConfigs,
		AlertRules: alertRules,
		Report:     reportConfig,
		SystemMetrics: monitor.SystemMetricsConfig{
			Enabled:     true,
			Interval:    60 * time.Second,
			CPU:         true,
			Memory:      true,
			Disk:        true,
			LoadAverage: true,
		},
	}

	// Write monitor config
	if err := w.writeMonitorConfig(monitorConfig); err != nil {
		return err
	}

	// Write environment file
	if err := w.writeEnvFile(pluginConfigs, reportConfig); err != nil {
		return err
	}

	// Write bootstrap config file
	if err := w.writeBootstrapConfig(); err != nil {
		return err
	}

	return nil
}

// writeMonitorConfig writes the monitor configuration to file
func (w *Wizard) writeMonitorConfig(config monitor.Config) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(w.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// For now, we'll create a simple YAML-like output
	// In a real implementation, you'd use a YAML library
	content := w.generateYAMLConfig(config)

	return os.WriteFile(w.configPath, []byte(content), 0644)
}

// writeEnvFile writes environment variables template
func (w *Wizard) writeEnvFile(pluginConfigs []plugins.PluginConfig, reportConfig monitor.ReportConfig) error {
	var content strings.Builder
	content.WriteString("# Beacon Environment Configuration\n")
	content.WriteString("# Fill in your actual values below. Never commit this file with real credentials!\n\n")

	// Add bootstrap/deployment environment variables
	content.WriteString("# Bootstrap and Deployment Configuration\n")
	content.WriteString("# These are used by 'beacon bootstrap' and 'beacon deploy'\n\n")
	content.WriteString("# Repository URL (supports both HTTPS and SSH)\n")
	content.WriteString("BEACON_REPO_URL=https://github.com/username/my-repo.git\n\n")
	content.WriteString("# Authentication (choose one or both)\n")
	content.WriteString("# Path to SSH private key (optional, for SSH URLs)\n")
	content.WriteString("# BEACON_SSH_KEY_PATH=$HOME/.ssh/id_rsa\n\n")
	content.WriteString("# Personal access token (optional, for HTTPS URLs)\n")
	content.WriteString("# BEACON_GIT_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxx\n\n")
	content.WriteString("# Local deployment path\n")
	content.WriteString("BEACON_LOCAL_PATH=$HOME/beacon/my-project\n\n")
	content.WriteString("# Deploy command (optional)\n")
	content.WriteString("# BEACON_DEPLOY_COMMAND=./deploy.sh\n\n")
	content.WriteString("# Polling interval\n")
	content.WriteString("BEACON_POLL_INTERVAL=60s\n\n")
	content.WriteString("# HTTP server port\n")
	content.WriteString("BEACON_PORT=8080\n\n")

	// Add plugin environment variables
	content.WriteString("# Plugin Configuration\n")
	content.WriteString("# ====================\n\n")
	for _, pluginConfig := range pluginConfigs {
		switch pluginConfig.Name {
		case "email":
			content.WriteString("# Email SMTP\n")
			content.WriteString("SMTP_HOST=smtp.gmail.com\n")
			content.WriteString("SMTP_PORT=587\n")
			content.WriteString("SMTP_USER=your-email@gmail.com\n")
			content.WriteString("SMTP_PASSWORD=your-app-password\n")
			content.WriteString("SMTP_FROM=beacon@example.com\n")
			content.WriteString("SMTP_TO=admin@example.com\n\n")
		}
	}

	// Add BeaconInfra token if enabled
	if reportConfig.SendTo != "" {
		content.WriteString("# BeaconInfra\n")
		content.WriteString("BEACONINFRA_TOKEN=your-beaconinfra-token\n\n")
	}

	return os.WriteFile(w.envPath, []byte(content.String()), 0644)
}

// writeBootstrapConfig writes a generic bootstrap configuration file
func (w *Wizard) writeBootstrapConfig() error {
	var content strings.Builder
	content.WriteString("# Beacon Bootstrap Configuration\n")
	content.WriteString("# Generated by Beacon Configuration Wizard\n")
	content.WriteString("# \n")
	content.WriteString("# Customize this file with your project details:\n")
	content.WriteString("#   - repo_url: Your Git repository URL\n")
	content.WriteString("#   - deploy_command: Command to run on deployment\n")
	content.WriteString("#   - Authentication: SSH key or Git token\n")
	content.WriteString("# \n")
	content.WriteString("# Usage: beacon bootstrap myproject -f beacon.bootstrap.yml\n\n")

	content.WriteString("# Project configuration\n")
	content.WriteString("project_name: \"my-project\"  # Will be overridden by bootstrap command\n")
	content.WriteString("repo_url: \"https://github.com/username/my-repo.git\"\n")
	content.WriteString("local_path: \"$HOME/beacon/my-project\"\n")
	content.WriteString("deploy_command: \"./deploy.sh\"  # Or: docker compose up --build -d\n")
	content.WriteString("poll_interval: \"60s\"\n")
	content.WriteString("port: \"8080\"\n\n")

	content.WriteString("# Authentication (choose one or both)\n")
	content.WriteString("# For SSH URLs:\n")
	content.WriteString("ssh_key_path: \"$HOME/.ssh/id_rsa\"  # Path to your SSH private key\n")
	content.WriteString("# For HTTPS URLs:\n")
	content.WriteString("#git_token: \"ghp_xxxxxxxxxxxxxxxxxxxx\"  # GitHub Personal Access Token\n\n")

	content.WriteString("# Security and environment\n")
	content.WriteString("secure_env_path: \"$HOME/.beacon/config/projects/my-project/env\"\n")
	content.WriteString("user: \"$USER\"  # User to run the service as\n")
	content.WriteString("working_dir: \"$HOME/beacon/my-project\"\n")
	content.WriteString("use_system_service: false  # Set to true for system-wide service\n")

	// Create directory if it doesn't exist
	dir := filepath.Dir(w.bootstrapPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(w.bootstrapPath, []byte(content.String()), 0644)
}

// generateYAMLConfig creates a YAML configuration string
func (w *Wizard) generateYAMLConfig(config monitor.Config) string {
	var content strings.Builder

	content.WriteString("# Beacon project monitor — checks, alerts, and log forwarding only.\n")
	content.WriteString("# Device name, API key, cloud reporting, heartbeats, and system_metrics live in ~/.beacon/config.yaml.\n")
	content.WriteString("# Generated by Beacon Configuration Wizard\n\n")

	content.WriteString("checks:\n")
	for _, check := range config.Checks {
		fmt.Fprintf(&content, "  - name: %s\n", check.Name)
		fmt.Fprintf(&content, "    type: %s\n", check.Type)
		fmt.Fprintf(&content, "    interval: %s\n", check.Interval.String())

		switch check.Type {
		case "http":
			fmt.Fprintf(&content, "    url: %s\n", check.URL)
			if check.ExpectStatus != 0 {
				fmt.Fprintf(&content, "    expect_status: %d\n", check.ExpectStatus)
			}
		case "port":
			fmt.Fprintf(&content, "    host: %s\n", check.Host)
			fmt.Fprintf(&content, "    port: %d\n", check.Port)
		case "command":
			fmt.Fprintf(&content, "    cmd: %s\n", check.Cmd)
		}
		content.WriteString("\n")
	}

	if len(config.Plugins) > 0 {
		content.WriteString("plugins:\n")
		for _, plugin := range config.Plugins {
			fmt.Fprintf(&content, "  - name: %s\n", plugin.Name)
			fmt.Fprintf(&content, "    enabled: %t\n", plugin.Enabled)
			for key, value := range plugin.Config {
				switch v := value.(type) {
				case string:
					fmt.Fprintf(&content, "    %s: %s\n", key, v)
				case []string:
					fmt.Fprintf(&content, "    %s:\n", key)
					for _, item := range v {
						fmt.Fprintf(&content, "      - %s\n", item)
					}
				}
			}
			content.WriteString("\n")
		}
	}

	if len(config.AlertRules) > 0 {
		content.WriteString("alert_rules:\n")
		for _, rule := range config.AlertRules {
			fmt.Fprintf(&content, "  - check: %s\n", rule.Check)
			fmt.Fprintf(&content, "    severity: %s\n", rule.Severity)
			content.WriteString("    plugins:\n")
			for _, plugin := range rule.Plugins {
				fmt.Fprintf(&content, "      - %s\n", plugin)
			}
			if rule.Cooldown != "" {
				fmt.Fprintf(&content, "    cooldown: %s\n", rule.Cooldown)
			}
			content.WriteString("\n")
		}
	}

	return content.String()
}
