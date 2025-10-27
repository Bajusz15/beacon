package alerting

import (
	"fmt"
	"log"
	"os"
	"time"

	"beacon/internal/config"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// SimpleAlertingCLI provides CLI commands for simple alert routing
type SimpleAlertingCLI struct {
	configPath string
}

// NewSimpleAlertingCLI creates a new simple alerting CLI
func NewSimpleAlertingCLI() *SimpleAlertingCLI {
	return &SimpleAlertingCLI{
		configPath: "", // Will be set per project
	}
}

// CreateSimpleAlertingCommand creates the simplified alerting command
func CreateSimpleAlertingCommand() *cobra.Command {
	cli := NewSimpleAlertingCLI()

	var alertingCmd = &cobra.Command{
		Use:   "alerts",
		Short: "Manage simple alert routing for projects",
		Long: `Manage simple alert routing for Beacon projects.
Perfect for self-hosted IoT monitoring and homelab setups.

Features:
- Severity-based routing (critical, warning, info)
- Multiple channels (email, Discord, Telegram, Slack)
- Simple backup notification after delay
- Quiet hours to suppress non-critical alerts
- Clean, simple configuration`,
		Example: `  beacon alerts init --project myapp
  beacon alerts status --project myapp
  beacon alerts acknowledge alert-123 --project myapp
  beacon alerts resolve alert-456 --project myapp
  beacon alerts test --project myapp`,
	}

	alertingCmd.AddCommand(createSimpleInitCommand(cli))
	alertingCmd.AddCommand(createSimpleStatusCommand(cli))
	alertingCmd.AddCommand(createSimpleAcknowledgeCommand(cli))
	alertingCmd.AddCommand(createSimpleResolveCommand(cli))
	alertingCmd.AddCommand(createSimpleTestCommand(cli))

	return alertingCmd
}

func createSimpleInitCommand(cli *SimpleAlertingCLI) *cobra.Command {
	var projectName string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize simple alert routing configuration for a project",
		Long:  `Create a simple alert routing configuration file with sensible defaults for a specific project.`,
		Run: func(cmd *cobra.Command, args []string) {
			if projectName == "" {
				log.Fatalf("Project name is required. Use --project flag")
			}

			if err := cli.InitSimpleConfig(projectName); err != nil {
				log.Fatalf("Failed to initialize alert config: %v", err)
			}
			fmt.Println("✅ Simple alert routing configuration initialized!")
			fmt.Printf("📁 Configuration file: %s\n", cli.configPath)
			fmt.Println()
			fmt.Println("Next steps:")
			fmt.Println("1. Edit the configuration file to match your needs")
			fmt.Println("2. Set up your alert channels (email, Discord, etc.)")
			fmt.Println("3. Configure quiet hours if desired")
			fmt.Println("4. Test your configuration with: beacon alerts test --project " + projectName)
			fmt.Println()
			fmt.Println("💡 Perfect for:")
			fmt.Println("   - Self-hosted IoT monitoring")
			fmt.Println("   - Homelab infrastructure")
			fmt.Println("   - Small team setups")
			fmt.Println("   - Privacy-first monitoring")
		},
	}

	cmd.Flags().StringVarP(&projectName, "project", "p", "", "Project name (required)")
	err := cmd.MarkFlagRequired("project")
	if err != nil {
		log.Fatalf("Failed to mark flag required: %v", err)
	}
	return cmd
}

func createSimpleStatusCommand(cli *SimpleAlertingCLI) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current alert status",
		Long:  `Display all active alerts and their current status.`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := cli.ShowSimpleStatus(); err != nil {
				log.Fatalf("Failed to show status: %v", err)
			}
		},
	}
}

func createSimpleAcknowledgeCommand(cli *SimpleAlertingCLI) *cobra.Command {
	return &cobra.Command{
		Use:   "acknowledge [alert-id]",
		Short: "Acknowledge an alert",
		Long:  `Acknowledge an alert to mark it as seen.`,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			alertID := args[0]
			acknowledgedBy, _ := cmd.Flags().GetString("by")
			if acknowledgedBy == "" {
				acknowledgedBy = "cli-user"
			}

			if err := cli.AcknowledgeSimpleAlert(alertID, acknowledgedBy); err != nil {
				log.Fatalf("Failed to acknowledge alert: %v", err)
			}
			fmt.Printf("✅ Alert %s acknowledged by %s\n", alertID, acknowledgedBy)
		},
	}
}

func createSimpleResolveCommand(cli *SimpleAlertingCLI) *cobra.Command {
	return &cobra.Command{
		Use:   "resolve [alert-id]",
		Short: "Resolve an alert",
		Long:  `Mark an alert as resolved.`,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			alertID := args[0]
			if err := cli.ResolveSimpleAlert(alertID); err != nil {
				log.Fatalf("Failed to resolve alert: %v", err)
			}
			fmt.Printf("✅ Alert %s resolved\n", alertID)
		},
	}
}

func createSimpleTestCommand(cli *SimpleAlertingCLI) *cobra.Command {
	return &cobra.Command{
		Use:   "test",
		Short: "Test simple alert routing",
		Long:  `Test the simple alert routing system with sample alerts.`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := cli.TestSimpleRouting(); err != nil {
				log.Fatalf("Failed to test routing: %v", err)
			}
		},
	}
}

// InitSimpleConfig creates a simple alert routing configuration
func (cli *SimpleAlertingCLI) InitSimpleConfig(projectName string) error {
	hierarchy, err := config.NewConfigHierarchy()
	if err != nil {
		return fmt.Errorf("failed to initialize config hierarchy: %v", err)
	}

	// Ensure directories exist
	paths, err := config.NewBeaconPaths()
	if err != nil {
		return fmt.Errorf("failed to initialize paths: %v", err)
	}

	if err := paths.EnsureDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %v", err)
	}

	// Set the config path for this project
	cli.configPath = hierarchy.GetConfigPath(config.AlertsConfig, projectName)

	// Check if config already exists
	if _, err := os.Stat(cli.configPath); err == nil {
		return fmt.Errorf("configuration file already exists: %s", cli.configPath)
	}

	// Create simple configuration
	alertConfig := SimpleAlertConfig{
		Routing: []AlertRouting{
			{
				Severity:         SeverityCritical,
				Channels:         []string{"email", "discord"},
				Recipients:       []string{"admin@example.com", "#alerts"},
				BackupDelay:      10 * time.Minute,
				BackupRecipients: []string{"backup@example.com"},
				Enabled:          true,
			},
			{
				Severity:         SeverityWarning,
				Channels:         []string{"discord"},
				Recipients:       []string{"#alerts"},
				BackupDelay:      30 * time.Minute,
				BackupRecipients: []string{"backup@example.com"},
				Enabled:          true,
			},
			{
				Severity:    SeverityInfo,
				Channels:    []string{"discord"},
				Recipients:  []string{"#logs"},
				BackupDelay: 0,
				Enabled:     true,
			},
		},
		Channels: map[string]interface{}{
			"email": map[string]interface{}{
				"smtp_host":     "smtp.gmail.com",
				"smtp_port":     587,
				"smtp_user":     "${SMTP_USER}",
				"smtp_password": "${SMTP_PASSWORD}",
				"from":          "Beacon Alerts <alerts@example.com>",
				"enabled":       true,
			},
			"discord": map[string]interface{}{
				"webhook_url": "${DISCORD_WEBHOOK_URL}",
				"username":    "Beacon Bot",
				"enabled":     true,
			},
		},
		Templates: map[string]interface{}{
			"critical": map[string]interface{}{
				"subject": "🚨 CRITICAL: {{.Service}} is {{.Status}}",
				"body":    "🚨 **CRITICAL ALERT**\n\n**Service:** {{.Service}}\n**Status:** {{.Status}}\n**Time:** {{.Timestamp.Format \"2006-01-02 15:04:05\"}}\n\nPlease check immediately!",
			},
			"warning": map[string]interface{}{
				"subject": "⚠️ WARNING: {{.Service}} {{.Message}}",
				"body":    "⚠️ **WARNING**\n\n**Service:** {{.Service}}\n**Message:** {{.Message}}\n**Time:** {{.Timestamp.Format \"2006-01-02 15:04:05\"}}",
			},
			"info": map[string]interface{}{
				"subject": "ℹ️ INFO: {{.Service}} {{.Message}}",
				"body":    "ℹ️ **INFO**\n\n**Service:** {{.Service}}\n**Message:** {{.Message}}\n**Time:** {{.Timestamp.Format \"2006-01-02 15:04:05\"}}",
			},
		},
	}

	// Write configuration using hierarchy
	if err := hierarchy.SaveConfig(config.AlertsConfig, projectName, alertConfig, false); err != nil {
		return fmt.Errorf("failed to save config: %v", err)
	}

	return nil
}

// ShowSimpleStatus displays current alert status
func (cli *SimpleAlertingCLI) ShowSimpleStatus() error {
	sam, err := cli.loadSimpleAlertManager()
	if err != nil {
		return err
	}

	activeAlerts := sam.GetActiveAlerts()

	if len(activeAlerts) == 0 {
		fmt.Println("✅ No active alerts")
		return nil
	}

	fmt.Printf("📊 Active Alerts (%d)\n", len(activeAlerts))
	fmt.Println()

	for alertID, alert := range activeAlerts {
		fmt.Printf("🚨 Alert ID: %s\n", alertID)
		fmt.Printf("   Service: %s\n", alert.Context.Service)
		fmt.Printf("   Severity: %s\n", alert.Context.Severity)
		fmt.Printf("   Message: %s\n", alert.Context.Message)
		fmt.Printf("   Timestamp: %s\n", alert.Context.Timestamp.Format(time.RFC3339))
		fmt.Printf("   Acknowledged: %t\n", alert.Acknowledged)
		if alert.Acknowledged {
			fmt.Printf("   Acknowledged By: %s\n", alert.AcknowledgedBy)
			fmt.Printf("   Acknowledged At: %s\n", alert.AcknowledgedAt.Format(time.RFC3339))
		}
		fmt.Printf("   Resolved: %t\n", alert.Resolved)
		if alert.Resolved {
			fmt.Printf("   Resolved At: %s\n", alert.ResolvedAt.Format(time.RFC3339))
		}
		fmt.Println()
	}

	return nil
}

// AcknowledgeSimpleAlert acknowledges an alert
func (cli *SimpleAlertingCLI) AcknowledgeSimpleAlert(alertID, acknowledgedBy string) error {
	sam, err := cli.loadSimpleAlertManager()
	if err != nil {
		return err
	}

	return sam.AcknowledgeAlert(alertID, acknowledgedBy)
}

// ResolveSimpleAlert resolves an alert
func (cli *SimpleAlertingCLI) ResolveSimpleAlert(alertID string) error {
	sam, err := cli.loadSimpleAlertManager()
	if err != nil {
		return err
	}

	return sam.ResolveAlert(alertID)
}

// TestSimpleRouting tests the simple alert routing system
func (cli *SimpleAlertingCLI) TestSimpleRouting() error {
	sam, err := cli.loadSimpleAlertManager()
	if err != nil {
		return err
	}

	fmt.Println("🧪 Testing Simple Alert Routing")
	fmt.Println()

	// Test alert contexts
	testAlerts := []AlertContext{
		{
			AlertID:     "test-db-down",
			Service:     "postgresql",
			Severity:    SeverityCritical,
			Message:     "Database connection failed",
			Timestamp:   time.Now(),
			Source:      "beacon-agent",
			Environment: "production",
			Tags: map[string]string{
				"environment": "production",
				"tier":        "database",
			},
		},
		{
			AlertID:     "test-api-slow",
			Service:     "api",
			Severity:    SeverityWarning,
			Message:     "Response time exceeded threshold",
			Timestamp:   time.Now(),
			Source:      "beacon-agent",
			Environment: "production",
			Tags: map[string]string{
				"environment": "production",
				"tier":        "api",
			},
		},
		{
			AlertID:     "test-deployment",
			Service:     "app",
			Severity:    SeverityInfo,
			Message:     "Deployment completed successfully",
			Timestamp:   time.Now(),
			Source:      "beacon-agent",
			Environment: "production",
			Tags: map[string]string{
				"environment": "production",
				"type":        "deployment",
			},
		},
	}

	for _, alert := range testAlerts {
		fmt.Printf("📤 Processing alert: %s (%s)\n", alert.AlertID, alert.Service)

		err := sam.ProcessAlert(alert)
		if err != nil {
			fmt.Printf("❌ Failed to process alert: %v\n", err)
			continue
		}

		fmt.Printf("✅ Alert processed successfully\n")

		// Show alert status
		status, err := sam.GetAlertStatus(alert.AlertID)
		if err != nil {
			fmt.Printf("❌ Failed to get alert status: %v\n", err)
			continue
		}

		fmt.Printf("   Severity: %s\n", status.Context.Severity)
		fmt.Printf("   Channels: %v\n", status.Routing.Channels)
		fmt.Printf("   Recipients: %v\n", status.Routing.Recipients)
		fmt.Println()
	}

	fmt.Println("🎉 Simple alert routing test completed!")
	return nil
}

// loadSimpleAlertManager loads the simple alert manager from config
func (cli *SimpleAlertingCLI) loadSimpleAlertManager() (*SimpleAlertManager, error) {
	sam := NewSimpleAlertManager()

	// Check if config file exists
	if _, err := os.Stat(cli.configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("alert config not found: %s\nRun 'beacon alerts init' to create it", cli.configPath)
	}

	// Load configuration
	config, err := cli.loadSimpleConfig()
	if err != nil {
		return nil, err
	}

	// Load routing
	if err := sam.LoadRouting(config.Routing); err != nil {
		return nil, fmt.Errorf("failed to load routing: %v", err)
	}

	return sam, nil
}

// loadSimpleConfig loads the simple alert configuration from file
func (cli *SimpleAlertingCLI) loadSimpleConfig() (*SimpleAlertConfig, error) {
	data, err := os.ReadFile(cli.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var config SimpleAlertConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %v", err)
	}

	return &config, nil
}

// SimpleAlertConfig represents the simple alert configuration
type SimpleAlertConfig struct {
	Routing   []AlertRouting           `yaml:"alert_routing"`
	Channels  map[string]interface{}   `yaml:"alert_channels"`
	Templates map[string]interface{}   `yaml:"alert_templates"`
	Rules     []map[string]interface{} `yaml:"alert_rules"`
}
