package main

import (
	"beacon/internal/alerting"
	"beacon/internal/config"
	"beacon/internal/deploy"
	"beacon/internal/keys"
	"beacon/internal/projects"
	"beacon/internal/server"
	"beacon/internal/state"
	"beacon/internal/templates"
	"beacon/internal/version"
	"beacon/internal/wizard"
	"fmt"
	"os"
	"path/filepath"

	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"beacon/internal/bootstrap"
	"beacon/internal/monitor"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "beacon",
	Short: "Beacon - IoT deployment and monitoring agent",
	Long: `Beacon is a lightweight deployment and monitoring agent for IoT devices.
Usage:
1. beacon deploy - runs the deployment agent that polls Git repositories for new tags and deploys them
2. beacon bootstrap - sets up your project configuration and optionally creates systemd services
3. beacon monitor - runs health checks and monitoring
4. beacon version - displays version information`,
	Version: version.GetVersion(),
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display version information",
	Long:  `Display detailed version information including build details.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Beacon %s\n", version.GetVersion())
		fmt.Printf("Commit: %s\n", version.GetCommit())
		fmt.Printf("Build Date: %s\n", version.GetBuildDate())
		fmt.Printf("Built by: %s\n", version.GetBuildUser())
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart [service]",
	Short: "Restart beacon services",
	Long: `Restart beacon services. If no service is specified, restarts the deploy service.
Available services: deploy, monitor`,
	Example: `  beacon restart
  beacon restart deploy
  beacon restart monitor`,
	Run: func(cmd *cobra.Command, args []string) {
		service := "deploy"
		if len(args) > 0 {
			service = args[0]
		}

		switch service {
		case "deploy":
			log.Println("[Beacon] Restarting deploy service...")
			// For now, just log the restart - in a real implementation,
			// this would signal the systemd service to restart
			log.Println("[Beacon] Deploy service restart requested")
		case "monitor":
			log.Println("[Beacon] Restarting monitor service...")
			log.Println("[Beacon] Monitor service restart requested")
		default:
			log.Printf("[Beacon] Unknown service: %s. Available services: deploy, monitor\n", service)
			os.Exit(1)
		}
	},
}

var wizardCmd = &cobra.Command{
	Use:   "setup-wizard",
	Short: "Interactive configuration wizard",
	Long: `Setup wizard helps you configure Beacon monitoring with an interactive interface.
This wizard will guide you through setting up device monitoring, alert plugins,
and reporting configuration.`,
	Example: `  beacon setup-wizard
  beacon setup-wizard --config ./beacon.monitor.yml --env .env`,
	Run: func(cmd *cobra.Command, args []string) {
		configPath, _ := cmd.Flags().GetString("config")
		envPath, _ := cmd.Flags().GetString("env")

		w := wizard.NewWizard(configPath, envPath)
		if err := w.Run(); err != nil {
			log.Fatalf("Wizard failed: %v", err)
		}
	},
}

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Manage alert templates",
	Long: `Manage alert templates for customizing notification formats.
Templates use Go template syntax and can be JSON, HTML, or plain text.`,
	Example: `  beacon template add my-alerts ./templates/discord.json
  beacon template list
  beacon template remove my-alerts
  beacon template show my-alerts
  beacon template check`,
}

var templateAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a template",
	Long:  `Add a new template from a file. The template will be stored and monitored for changes.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli, err := templates.NewCLI()
		if err != nil {
			log.Fatalf("Failed to initialize template CLI: %v", err)
		}
		if err := cli.AddTemplate(); err != nil {
			log.Fatalf("Failed to add template: %v", err)
		}
	},
}

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all templates",
	Long:  `List all registered templates with their paths and modification times.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli, err := templates.NewCLI()
		if err != nil {
			log.Fatalf("Failed to initialize template CLI: %v", err)
		}
		if err := cli.ListTemplates(); err != nil {
			log.Fatalf("Failed to list templates: %v", err)
		}
	},
}

var templateRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove a template",
	Long:  `Remove a registered template.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli, err := templates.NewCLI()
		if err != nil {
			log.Fatalf("Failed to initialize template CLI: %v", err)
		}
		if err := cli.RemoveTemplate(); err != nil {
			log.Fatalf("Failed to remove template: %v", err)
		}
	},
}

var templateShowCmd = &cobra.Command{
	Use:   "show [template-name]",
	Short: "Show template content",
	Long:  `Show the content of a registered template.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cli, err := templates.NewCLI()
		if err != nil {
			log.Fatalf("Failed to initialize template CLI: %v", err)
		}
		if err := cli.ShowTemplate(args[0]); err != nil {
			log.Fatalf("Failed to show template: %v", err)
		}
	},
}

var templateCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for template changes",
	Long:  `Check if any registered templates have been modified since last check.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli, err := templates.NewCLI()
		if err != nil {
			log.Fatalf("Failed to initialize template CLI: %v", err)
		}
		if err := cli.CheckChanges(); err != nil {
			log.Fatalf("Failed to check template changes: %v", err)
		}
	},
}

var templateInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize default templates",
	Long:  `Create default template files in ~/.beacon/templates/ directory.`,
	Run: func(cmd *cobra.Command, args []string) {
		templateDir := templates.GetDefaultTemplatePath()
		if err := templates.CreateDefaultTemplates(templateDir); err != nil {
			log.Fatalf("Failed to create default templates: %v", err)
		}
		fmt.Printf("✅ Default templates created in: %s\n", templateDir)
		fmt.Println()
		fmt.Println("Available templates:")
		fmt.Println("  - discord.json   (Discord webhook format)")
		fmt.Println("  - telegram.txt  (Telegram message format)")
		fmt.Println("  - email.html    (HTML email format)")
		fmt.Println("  - webhook.json  (Generic webhook format)")
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("1. Edit templates as needed")
		fmt.Println("2. Add templates: beacon template add discord " + templateDir + "/discord.json")
		fmt.Println("3. Start monitoring: beacon monitor")
	},
}

func init() {
	wizardCmd.Flags().StringP("config", "c", "beacon.monitor.yml", "Path to monitor configuration file")
	wizardCmd.Flags().StringP("env", "e", ".env", "Path to environment file")

	// Add template subcommands
	templateCmd.AddCommand(templateAddCmd)
	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateRemoveCmd)
	templateCmd.AddCommand(templateShowCmd)
	templateCmd.AddCommand(templateCheckCmd)
	templateCmd.AddCommand(templateInitCmd)
}

var monitorCmd = &cobra.Command{
	Use:   "monitor [config-file]",
	Short: "Run health checks and report results",
	Long: `Monitor runs health checks based on configuration and reports results.

You can specify a configuration file as an argument or using --config flag:
  beacon monitor                    # Uses beacon.monitor.yml in the current directory
  beacon monitor my-config.yml      # Uses my-config.yml
  beacon monitor -f my-config.yml  # Uses my-config.yml

The configuration file should contain device info, checks, and alert rules.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		monitor.Run(cmd, args)
	},
}

func init() {
	monitorCmd.Flags().StringP("config", "f", "", "Path to configuration file")
}

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Run beacon in deployment mode (default behavior)",
	Long: `Run beacon in deployment mode that polls Git repositories for new tags
and automatically deploys them. This is the default behavior when
no subcommand is specified.`,
	Run: func(cmd *cobra.Command, args []string) {
		runDeploy()
	},
}

func main() {
	// Add subcommands
	rootCmd.AddCommand(bootstrap.BootstrapCommand())
	rootCmd.AddCommand(monitorCmd)
	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(restartCmd)
	rootCmd.AddCommand(wizardCmd)
	rootCmd.AddCommand(templateCmd)
	rootCmd.AddCommand(keys.KeysCmd)
	rootCmd.AddCommand(alerting.CreateSimpleAlertingCommand())
	rootCmd.AddCommand(projects.CreateProjectCommand())

	// If no subcommand is provided, run in deploy mode
	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		runDeploy()
	}

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runDeploy() {
	log.Println("[Beacon] Deploy agent starting...")

	// Set up graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()

	// Create data directory for persistence
	statusStorage := filepath.Join(os.Getenv("HOME"), ".beacon", cfg.ProjectDir)
	status := state.NewStatus(statusStorage)

	// Start HTTP status/metrics endpoint
	go server.StartHTTPServer(cfg, status)

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	// Main polling loop
	for {
		select {
		case <-ctx.Done():
			log.Println("[Beacon] Shutdown signal received, stopping...")
			return
		case <-ticker.C:
			deploy.CheckForNewTag(cfg, status)
		}
	}
}
