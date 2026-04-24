package alerting

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSimpleAlertingCLI_Comprehensive(t *testing.T) {
	t.Run("initialization", func(t *testing.T) {
		cli := NewSimpleAlertingCLI()

		assert.NotNil(t, cli)
		assert.Equal(t, "", cli.configPath)
	})

	t.Run("command_structure", func(t *testing.T) {
		cmd := CreateSimpleAlertingCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "alerts", cmd.Use)
		assert.Equal(t, "Manage simple alert routing for projects", cmd.Short)
		assert.Contains(t, cmd.Long, "Manage simple alert routing for Beacon projects")
		assert.Contains(t, cmd.Example, "beacon alerts init --project myapp")

		// Check subcommands
		subcommands := cmd.Commands()
		assert.Equal(t, 5, len(subcommands))

		subcommandNames := make([]string, len(subcommands))
		for i, subcmd := range subcommands {
			subcommandNames[i] = subcmd.Use
		}

		// Check that all expected commands exist (use partial matching for commands with args)
		hasInit := false
		hasStatus := false
		hasAcknowledge := false
		hasResolve := false
		hasTest := false

		for _, name := range subcommandNames {
			switch name {
			case "init":
				hasInit = true
			case "status":
				hasStatus = true
			case "acknowledge [alert-id]":
				hasAcknowledge = true
			case "resolve [alert-id]":
				hasResolve = true
			case "test":
				hasTest = true
			}
		}

		assert.True(t, hasInit, "Should have init command")
		assert.True(t, hasStatus, "Should have status command")
		assert.True(t, hasAcknowledge, "Should have acknowledge command")
		assert.True(t, hasResolve, "Should have resolve command")
		assert.True(t, hasTest, "Should have test command")
	})

	t.Run("individual_commands", func(t *testing.T) {
		cli := NewSimpleAlertingCLI()
		var pn string

		root := CreateSimpleAlertingCommand()
		pf := root.PersistentFlags().Lookup("project")
		require.NotNil(t, pf)
		assert.Equal(t, "p", pf.Shorthand)

		initCmd := createSimpleInitCommand(cli, &pn)
		assert.NotNil(t, initCmd)
		assert.Equal(t, "init", initCmd.Use)
		assert.Equal(t, "Initialize simple alert routing configuration for a project", initCmd.Short)

		// Test status command
		statusCmd := createSimpleStatusCommand(cli)
		assert.NotNil(t, statusCmd)
		assert.Equal(t, "status", statusCmd.Use)
		assert.Equal(t, "Show current alert status", statusCmd.Short)

		// Test acknowledge command
		ackCmd := createSimpleAcknowledgeCommand(cli)
		assert.NotNil(t, ackCmd)
		assert.Equal(t, "acknowledge [alert-id]", ackCmd.Use)
		assert.Equal(t, "Acknowledge an alert", ackCmd.Short)

		// Test resolve command
		resolveCmd := createSimpleResolveCommand(cli)
		assert.NotNil(t, resolveCmd)
		assert.Equal(t, "resolve [alert-id]", resolveCmd.Use)
		assert.Equal(t, "Resolve an alert", resolveCmd.Short)

		// Test test command
		testCmd := createSimpleTestCommand(cli)
		assert.NotNil(t, testCmd)
		assert.Equal(t, "test", testCmd.Use)
		assert.Equal(t, "Test simple alert routing", testCmd.Short)
	})

	t.Run("config_operations", func(t *testing.T) {
		// Create temporary directory for test
		tempDir := t.TempDir()

		projectName := "test-project"
		configDir := filepath.Join(tempDir, ".beacon", "config", "projects", projectName)
		err := os.MkdirAll(configDir, 0755)
		require.NoError(t, err)

		cli := NewSimpleAlertingCLI()
		cli.configPath = filepath.Join(configDir, "alerts.yml")

		// Create a simple config manually instead of using InitSimpleConfig
		// which uses the global config system
		testConfig := SimpleAlertConfig{
			Routing: []AlertRouting{
				{
					Severity:         SeverityCritical,
					Channels:         []string{"email", "webhook"},
					Recipients:       []string{"admin@example.com", "#alerts"},
					BackupDelay:      10 * time.Minute,
					BackupRecipients: []string{"backup@example.com"},
					Enabled:          true,
				},
				{
					Severity:         SeverityWarning,
					Channels:         []string{"webhook"},
					Recipients:       []string{"#alerts"},
					BackupDelay:      30 * time.Minute,
					BackupRecipients: []string{"backup@example.com"},
					Enabled:          true,
				},
				{
					Severity:    SeverityInfo,
					Channels:    []string{"webhook"},
					Recipients:  []string{"#logs"},
					BackupDelay: 0,
					Enabled:     true,
				},
			},
			Channels: map[string]interface{}{
				"email": map[string]interface{}{
					"smtp_host": "smtp.gmail.com",
					"enabled":   false,
				},
				"webhook": map[string]interface{}{
					"url":     "${WEBHOOK_URL}",
					"enabled": false,
				},
			},
		}

		// Write the config manually
		data, err := yaml.Marshal(testConfig)
		require.NoError(t, err)

		err = os.WriteFile(cli.configPath, data, 0644)
		require.NoError(t, err)
		assert.FileExists(t, cli.configPath)

		// Verify config content
		readData, err := os.ReadFile(cli.configPath)
		require.NoError(t, err)

		var config SimpleAlertConfig
		err = yaml.Unmarshal(readData, &config)
		require.NoError(t, err)

		// Verify routing configuration
		assert.Equal(t, 3, len(config.Routing))

		// Find routing by severity
		var criticalRouting, warningRouting, infoRouting *AlertRouting
		for i := range config.Routing {
			switch config.Routing[i].Severity {
			case SeverityCritical:
				criticalRouting = &config.Routing[i]
			case SeverityWarning:
				warningRouting = &config.Routing[i]
			case SeverityInfo:
				infoRouting = &config.Routing[i]
			}
		}

		require.NotNil(t, criticalRouting)
		assert.Contains(t, criticalRouting.Channels, "email")
		assert.Contains(t, criticalRouting.Channels, "webhook")
		assert.True(t, criticalRouting.Enabled)
		assert.Equal(t, 10*time.Minute, criticalRouting.BackupDelay) // Actual value from implementation

		require.NotNil(t, warningRouting)
		assert.Contains(t, warningRouting.Channels, "webhook")
		assert.True(t, warningRouting.Enabled)

		require.NotNil(t, infoRouting)
		assert.Contains(t, infoRouting.Channels, "webhook")
		assert.True(t, infoRouting.Enabled)

		assert.Contains(t, config.Channels, "email")
		assert.Contains(t, config.Channels, "webhook")

		// Test loading the config back
		loadedConfig, err := cli.loadSimpleConfig()
		assert.NoError(t, err)
		assert.NotNil(t, loadedConfig)
		assert.Equal(t, 3, len(loadedConfig.Routing))
	})

	t.Run("config_error_handling", func(t *testing.T) {
		cli := NewSimpleAlertingCLI()

		// Test file not found
		cli.configPath = "/nonexistent/path/alerts.yml"
		config, err := cli.loadSimpleConfig()
		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "failed to read config file")

		// Test invalid YAML
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "alerts.yml")

		invalidYAML := `
alert_routing:
  - severity: critical
    channels: [email
    # Missing closing bracket - invalid YAML
`

		err = os.WriteFile(configPath, []byte(invalidYAML), 0644)
		require.NoError(t, err)

		cli.configPath = configPath
		config, err = cli.loadSimpleConfig()
		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "failed to unmarshal config")
	})

	t.Run("alert_manager_integration", func(t *testing.T) {
		// Setup test environment
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "alerts.yml")

		testConfig := SimpleAlertConfig{
			Routing: []AlertRouting{
				{
					Severity:   SeverityCritical,
					Channels:   []string{"email"},
					Recipients: []string{"admin@example.com"},
					Enabled:    true,
				},
				{
					Severity:   SeverityWarning,
					Channels:   []string{"slack"},
					Recipients: []string{"#warnings"},
					Enabled:    true,
				},
			},
		}

		// Write test config
		data, err := yaml.Marshal(testConfig)
		require.NoError(t, err)

		err = os.WriteFile(configPath, data, 0644)
		require.NoError(t, err)

		cli := NewSimpleAlertingCLI()
		cli.configPath = configPath

		// Test loading alert manager
		sam, err := cli.loadSimpleAlertManager()
		assert.NoError(t, err)
		assert.NotNil(t, sam)

		// Verify routing was loaded
		assert.Equal(t, 2, len(sam.routing))
		assert.Contains(t, sam.routing, SeverityCritical)
		assert.Contains(t, sam.routing, SeverityWarning)

		// Test config not found error
		cli.configPath = "/nonexistent/path/alerts.yml"
		sam, err = cli.loadSimpleAlertManager()
		assert.Error(t, err)
		assert.Nil(t, sam)
		assert.Contains(t, err.Error(), "alert config not found")
		assert.Contains(t, err.Error(), "Run 'beacon alerts init' to create it")
	})

	t.Run("alert_operations", func(t *testing.T) {
		// Setup test environment with working config
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "alerts.yml")

		testConfig := SimpleAlertConfig{
			Routing: []AlertRouting{
				{
					Severity:   SeverityCritical,
					Channels:   []string{"email"},
					Recipients: []string{"admin@example.com"},
					Enabled:    true,
				},
			},
		}

		data, err := yaml.Marshal(testConfig)
		require.NoError(t, err)

		err = os.WriteFile(configPath, data, 0644)
		require.NoError(t, err)

		cli := NewSimpleAlertingCLI()
		cli.configPath = configPath

		// Create an alert first by loading the manager and processing an alert
		sam, err := cli.loadSimpleAlertManager()
		require.NoError(t, err)

		context := AlertContext{
			AlertID:   "test-alert",
			Service:   "database",
			Severity:  SeverityCritical,
			Message:   "Database is down",
			Timestamp: time.Now(),
			Source:    "test",
		}

		err = sam.ProcessAlert(context)
		require.NoError(t, err)

		// Test acknowledgment - CLI methods create new manager instances, so we need to use the same manager
		err = sam.AcknowledgeAlert("test-alert", "admin")
		assert.NoError(t, err)

		// Verify acknowledgment
		status, err := sam.GetAlertStatus("test-alert")
		require.NoError(t, err)
		assert.True(t, status.Acknowledged)
		assert.Equal(t, "admin", status.AcknowledgedBy)

		// Test resolution
		err = sam.ResolveAlert("test-alert")
		assert.NoError(t, err)

		// Verify resolution
		status, err = sam.GetAlertStatus("test-alert")
		require.NoError(t, err)
		assert.True(t, status.Resolved)
	})

	t.Run("test_routing", func(t *testing.T) {
		// Setup test environment
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "alerts.yml")

		testConfig := SimpleAlertConfig{
			Routing: []AlertRouting{
				{
					Severity:   SeverityCritical,
					Channels:   []string{"email"},
					Recipients: []string{"admin@example.com"},
					Enabled:    true,
				},
				{
					Severity:   SeverityWarning,
					Channels:   []string{"slack"},
					Recipients: []string{"#warnings"},
					Enabled:    true,
				},
				{
					Severity:   SeverityInfo,
					Channels:   []string{"webhook"},
					Recipients: []string{"#info"},
					Enabled:    true,
				},
			},
		}

		data, err := yaml.Marshal(testConfig)
		require.NoError(t, err)

		err = os.WriteFile(configPath, data, 0644)
		require.NoError(t, err)

		cli := NewSimpleAlertingCLI()
		cli.configPath = configPath
		cli.projectID = "test-project"

		// Test routing (this will process the predefined test alerts)
		err = cli.TestSimpleRouting()
		assert.NoError(t, err)
	})

	t.Run("error_conditions", func(t *testing.T) {
		cli := NewSimpleAlertingCLI()
		cli.configPath = "/nonexistent/path/alerts.yml"

		// Test various error conditions
		err := cli.AcknowledgeSimpleAlert("non-existent", "user")
		assert.Error(t, err)

		err = cli.ResolveSimpleAlert("non-existent")
		assert.Error(t, err)

		err = cli.TestSimpleRouting()
		assert.Error(t, err)
	})

	t.Run("data_structures", func(t *testing.T) {
		config := SimpleAlertConfig{
			Routing: []AlertRouting{
				{
					Severity:   SeverityCritical,
					Channels:   []string{"email"},
					Recipients: []string{"admin@example.com"},
					Enabled:    true,
				},
			},
			Channels: map[string]interface{}{
				"email": map[string]interface{}{
					"smtp_host": "smtp.example.com",
					"enabled":   true,
				},
			},
			Rules: []map[string]interface{}{
				{
					"name":     "database-check",
					"severity": "critical",
				},
			},
		}

		assert.Equal(t, 1, len(config.Routing))
		assert.Equal(t, SeverityCritical, config.Routing[0].Severity)
		assert.Contains(t, config.Channels, "email")
		assert.Equal(t, 1, len(config.Rules))
	})

	t.Run("full_workflow", func(t *testing.T) {
		// Integration test: init -> process alerts -> acknowledge -> resolve

		tempDir := t.TempDir()
		projectName := "integration-test"
		configDir := filepath.Join(tempDir, ".beacon", "config", "projects", projectName)
		err := os.MkdirAll(configDir, 0755)
		require.NoError(t, err)

		cli := NewSimpleAlertingCLI()
		cli.configPath = filepath.Join(configDir, "alerts.yml")

		// Step 1: Create config manually
		testConfig := SimpleAlertConfig{
			Routing: []AlertRouting{
				{
					Severity:   SeverityCritical,
					Channels:   []string{"email"},
					Recipients: []string{"admin@example.com"},
					Enabled:    true,
				},
			},
		}

		data, err := yaml.Marshal(testConfig)
		require.NoError(t, err)

		err = os.WriteFile(cli.configPath, data, 0644)
		require.NoError(t, err)
		assert.FileExists(t, cli.configPath)

		// Step 2: Load alert manager
		sam, err := cli.loadSimpleAlertManager()
		require.NoError(t, err)
		assert.NotNil(t, sam)

		// Step 3: Process a test alert
		context := AlertContext{
			AlertID:   "integration-test-alert",
			Service:   "test-service",
			Severity:  SeverityCritical,
			Message:   "Integration test alert",
			Timestamp: time.Now(),
			Source:    "test",
		}

		err = sam.ProcessAlert(context)
		require.NoError(t, err)

		// Step 4: Verify alert exists
		status, err := sam.GetAlertStatus("integration-test-alert")
		require.NoError(t, err)
		assert.False(t, status.Acknowledged)
		assert.False(t, status.Resolved)

		// Step 5: Acknowledge alert - use the same manager instance
		err = sam.AcknowledgeAlert("integration-test-alert", "test-user")
		require.NoError(t, err)

		status, err = sam.GetAlertStatus("integration-test-alert")
		require.NoError(t, err)
		assert.True(t, status.Acknowledged)
		assert.Equal(t, "test-user", status.AcknowledgedBy)
		assert.False(t, status.Resolved)

		// Step 6: Resolve alert - use the same manager instance
		err = sam.ResolveAlert("integration-test-alert")
		require.NoError(t, err)

		status, err = sam.GetAlertStatus("integration-test-alert")
		require.NoError(t, err)
		assert.True(t, status.Acknowledged)
		assert.True(t, status.Resolved)
	})
}
