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
	configPath string
	envPath    string
	deviceName string
	deviceType string
}

// NewWizard creates a new configuration wizard
func NewWizard(configPath, envPath string) *Wizard {
	return &Wizard{
		configPath: configPath,
		envPath:    envPath,
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
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("1. Review the generated configuration files")
	fmt.Println("2. Set up your environment variables")
	fmt.Println("3. Run: beacon monitor")
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
		// Configure Discord
		if w.configureDiscord() {
			pluginConfigs = append(pluginConfigs, plugins.PluginConfig{
				Name:    "discord",
				Enabled: true,
				Config: map[string]interface{}{
					"webhook_url": "${DISCORD_WEBHOOK_URL}",
				},
			})
		}

		// Configure Telegram
		if w.configureTelegram() {
			pluginConfigs = append(pluginConfigs, plugins.PluginConfig{
				Name:    "telegram",
				Enabled: true,
				Config: map[string]interface{}{
					"bot_token": "${TELEGRAM_BOT_TOKEN}",
					"chat_id":   "${TELEGRAM_CHAT_ID}",
				},
			})
		}

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

// configureDiscord asks user if they want to configure Discord
func (w *Wizard) configureDiscord() bool {
	fmt.Print("Configure Discord webhook? (y/n) [n]: ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

// configureTelegram asks user if they want to configure Telegram
func (w *Wizard) configureTelegram() bool {
	fmt.Print("Configure Telegram bot? (y/n) [n]: ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
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
	content.WriteString("# Copy this file to .env and fill in your values\n\n")

	// Add plugin environment variables
	for _, pluginConfig := range pluginConfigs {
		switch pluginConfig.Name {
		case "discord":
			content.WriteString("# Discord Webhook\n")
			content.WriteString("DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/YOUR/WEBHOOK\n\n")
		case "telegram":
			content.WriteString("# Telegram Bot\n")
			content.WriteString("TELEGRAM_BOT_TOKEN=your-bot-token\n")
			content.WriteString("TELEGRAM_CHAT_ID=your-chat-id\n\n")
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

// generateYAMLConfig creates a YAML configuration string
func (w *Wizard) generateYAMLConfig(config monitor.Config) string {
	var content strings.Builder

	content.WriteString("# Beacon Monitor Configuration\n")
	content.WriteString("# Generated by Beacon Configuration Wizard\n\n")

	content.WriteString("device:\n")
	content.WriteString(fmt.Sprintf("  name: %s\n", config.Device.Name))
	content.WriteString(fmt.Sprintf("  location: %s\n", config.Device.Location))
	content.WriteString(fmt.Sprintf("  environment: %s\n", config.Device.Environment))
	content.WriteString("  tags:\n")
	for _, tag := range config.Device.Tags {
		content.WriteString(fmt.Sprintf("    - %s\n", tag))
	}
	content.WriteString("\n")

	content.WriteString("checks:\n")
	for _, check := range config.Checks {
		content.WriteString(fmt.Sprintf("  - name: %s\n", check.Name))
		content.WriteString(fmt.Sprintf("    type: %s\n", check.Type))
		content.WriteString(fmt.Sprintf("    interval: %s\n", check.Interval.String()))

		switch check.Type {
		case "http":
			content.WriteString(fmt.Sprintf("    url: %s\n", check.URL))
			if check.ExpectStatus != 0 {
				content.WriteString(fmt.Sprintf("    expect_status: %d\n", check.ExpectStatus))
			}
		case "port":
			content.WriteString(fmt.Sprintf("    host: %s\n", check.Host))
			content.WriteString(fmt.Sprintf("    port: %d\n", check.Port))
		case "command":
			content.WriteString(fmt.Sprintf("    cmd: %s\n", check.Cmd))
		}
		content.WriteString("\n")
	}

	if len(config.Plugins) > 0 {
		content.WriteString("plugins:\n")
		for _, plugin := range config.Plugins {
			content.WriteString(fmt.Sprintf("  - name: %s\n", plugin.Name))
			content.WriteString(fmt.Sprintf("    enabled: %t\n", plugin.Enabled))
			for key, value := range plugin.Config {
				switch v := value.(type) {
				case string:
					content.WriteString(fmt.Sprintf("    %s: %s\n", key, v))
				case []string:
					content.WriteString(fmt.Sprintf("    %s:\n", key))
					for _, item := range v {
						content.WriteString(fmt.Sprintf("      - %s\n", item))
					}
				}
			}
			content.WriteString("\n")
		}
	}

	if len(config.AlertRules) > 0 {
		content.WriteString("alert_rules:\n")
		for _, rule := range config.AlertRules {
			content.WriteString(fmt.Sprintf("  - check: %s\n", rule.Check))
			content.WriteString(fmt.Sprintf("    severity: %s\n", rule.Severity))
			content.WriteString("    plugins:\n")
			for _, plugin := range rule.Plugins {
				content.WriteString(fmt.Sprintf("      - %s\n", plugin))
			}
			if rule.Cooldown != "" {
				content.WriteString(fmt.Sprintf("    cooldown: %s\n", rule.Cooldown))
			}
			content.WriteString("\n")
		}
	}

	if config.Report.SendTo != "" {
		content.WriteString("report:\n")
		content.WriteString(fmt.Sprintf("  send_to: %s\n", config.Report.SendTo))
		content.WriteString(fmt.Sprintf("  token: %s\n", config.Report.Token))
		content.WriteString("\n")
	}

	content.WriteString("system_metrics:\n")
	content.WriteString(fmt.Sprintf("  enabled: %t\n", config.SystemMetrics.Enabled))
	content.WriteString(fmt.Sprintf("  interval: %s\n", config.SystemMetrics.Interval.String()))
	content.WriteString(fmt.Sprintf("  cpu: %t\n", config.SystemMetrics.CPU))
	content.WriteString(fmt.Sprintf("  memory: %t\n", config.SystemMetrics.Memory))
	content.WriteString(fmt.Sprintf("  disk: %t\n", config.SystemMetrics.Disk))
	content.WriteString(fmt.Sprintf("  load_average: %t\n", config.SystemMetrics.LoadAverage))

	return content.String()
}
