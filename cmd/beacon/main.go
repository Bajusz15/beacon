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
	"beacon/internal/child"
	"beacon/internal/identity"
	"beacon/internal/k8sobserver"
	"beacon/internal/master"
	"beacon/internal/mcp"
	"beacon/internal/monitor"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "beacon",
	Short: "Beacon - IoT deployment and monitoring agent",
	Long: `Beacon is a lightweight agent for self-hosted devices — deploy, monitor, and report health.

  beacon init      write ~/.beacon/config.yaml (no network)
  beacon master    start the master agent — spawns child agents, sends heartbeats
  beacon bootstrap set up a new project (interactive or from a config file)
  beacon monitor   run a single project's health checks (dev/debug)
  beacon deploy    poll a Git repo for new tags and deploy
  beacon version   show version`,
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
Available services: deploy, monitor, master (cloud agent: systemctl --user restart beacon-master.service)`,
	Example: `  beacon restart
  beacon restart deploy
  beacon restart monitor
  beacon restart master`,
	Run: func(cmd *cobra.Command, args []string) {
		service := "deploy"
		if len(args) > 0 {
			service = args[0]
		}

		switch service {
		case "deploy":
			log.Println("[Beacon] Restarting deploy service...")
			log.Println("[Beacon] Deploy service restart requested")
		case "monitor":
			log.Println("[Beacon] Restarting monitor service...")
			log.Println("[Beacon] Monitor service restart requested")
		case "master":
			log.Println("[Beacon] Restart master: systemctl --user restart beacon-master.service")
		default:
			log.Printf("[Beacon] Unknown service: %s. Available services: deploy, monitor, master\n", service)
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

var initAgentCmd = &cobra.Command{
	Use:   "init",
	Short: "Write local cloud identity to ~/.beacon/config.yaml (no network)",
	Long: `Writes ~/.beacon/config.yaml with api_key, device_name, cloud_url, and heartbeat_interval.
No HTTP requests are made — the device is registered on the first heartbeat when you run beacon master.

Cloud URL defaults to https://beaconinfra.dev/api if not specified.
Device name defaults to the system hostname if not specified.

Get your API key at https://beaconinfra.dev → Settings → API Keys.

Environment: BEACON_API_KEY, BEACON_CLOUD_URL, BEACON_DEVICE_NAME.`,
	Example: `  # Minimal — just your API key, everything else uses defaults
  beacon init --api-key usr_abc123def456

  # Explicit device name
  beacon init --api-key usr_abc123def456 --name my-pi

  # Self-hosted backend
  beacon init --api-key usr_abc123def456 --cloud-url https://my-backend.example.com/api`,
	Run: func(cmd *cobra.Command, args []string) {
		cloudURL, _ := cmd.Flags().GetString("cloud-url")
		if cloudURL == "" {
			cloudURL, _ = cmd.Flags().GetString("server")
		}
		apiKey, _ := cmd.Flags().GetString("api-key")
		if apiKey == "" {
			apiKey = os.Getenv("BEACON_API_KEY")
		}
		if apiKey == "" {
			apiKey = os.Getenv("BEACON_USER_API_KEY")
		}
		name, _ := cmd.Flags().GetString("name")
		if name == "" {
			name, _ = cmd.Flags().GetString("device-name")
		}
		if name == "" {
			name = os.Getenv("BEACON_DEVICE_NAME")
		}

		if err := identity.WriteUserInit(apiKey, name, cloudURL); err != nil {
			log.Fatalf("beacon init: %v", err)
		}
		p, err := identity.UserConfigPath()
		if err != nil {
			log.Printf("[Beacon] Wrote ~/.beacon/config.yaml")
			return
		}
		log.Printf("[Beacon] Wrote %s", p)
	},
}

var masterCmd = &cobra.Command{
	Use:   "master",
	Short: "Run the machine-wide cloud reporter (foreground)",
	Long: `Runs independently of any project. Reads ~/.beacon/config.yaml.

When cloud_reporting_enabled is true and api_key, cloud_url, and device_name are set (via beacon init),
sends POST /agent/heartbeat every heartbeat_interval (default 60s if unset).

Typical install: beacon bootstrap creates beacon-master.service; it runs this command in the background.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		master.Run(ctx)
	},
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

	initAgentCmd.Flags().String("cloud-url", "", "API base URL including /api (BEACON_CLOUD_URL or BEACON_SERVER_URL)")
	initAgentCmd.Flags().String("server", "", "Alias for --cloud-url (BEACON_SERVER_URL)")
	initAgentCmd.Flags().String("api-key", "", "User API key (BEACON_API_KEY or BEACON_USER_API_KEY)")
	initAgentCmd.Flags().String("name", "", "Device name (BEACON_DEVICE_NAME; default: hostname)")
	initAgentCmd.Flags().String("device-name", "", "Alias for --name")

	// Child agent flags (hidden command - spawned by master only)
	childAgentCmd.Flags().String("project-id", "", "Project identifier")
	childAgentCmd.Flags().String("config", "", "Path to project YAML config")
	childAgentCmd.Flags().String("ipc-dir", "", "IPC directory for this child")
	_ = childAgentCmd.MarkFlagRequired("project-id")
	_ = childAgentCmd.MarkFlagRequired("config")
	_ = childAgentCmd.MarkFlagRequired("ipc-dir")
}

// childAgentCmd is the hidden "beacon agent" subcommand spawned by the master.
// Users should never run this directly - it's for internal master/child IPC.
var childAgentCmd = &cobra.Command{
	Use:    "agent",
	Short:  "Run as child agent (internal - spawned by master)",
	Hidden: true, // Don't show in help - internal use only
	Run: func(cmd *cobra.Command, args []string) {
		projectID, _ := cmd.Flags().GetString("project-id")
		configPath, _ := cmd.Flags().GetString("config")
		ipcDir, _ := cmd.Flags().GetString("ipc-dir")

		cfg := &child.Config{
			ProjectID:  projectID,
			ConfigPath: configPath,
			IPCDir:     ipcDir,
		}

		c, err := child.New(cfg)
		if err != nil {
			log.Fatalf("[Beacon agent] Failed to initialize: %v", err)
		}

		if err := c.Run(); err != nil {
			log.Fatalf("[Beacon agent] Failed: %v", err)
		}
	},
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
	rootCmd.AddCommand(initAgentCmd)
	rootCmd.AddCommand(masterCmd)
	rootCmd.AddCommand(monitorCmd)
	rootCmd.AddCommand(childAgentCmd) // Hidden - spawned by master only
	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(restartCmd)
	rootCmd.AddCommand(wizardCmd)
	rootCmd.AddCommand(templateCmd)
	rootCmd.AddCommand(keys.KeysCmd)
	rootCmd.AddCommand(alerting.CreateSimpleAlertingCommand())
	rootCmd.AddCommand(projects.CreateProjectCommand())
	rootCmd.AddCommand(createSourceCommand())
	rootCmd.AddCommand(createMCPCommand())

	// If no subcommand is provided, run in deploy mode
	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		runDeploy()
	}

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func createSourceCommand() *cobra.Command {
	sourceCmd := &cobra.Command{
		Use:   "source",
		Short: "Manage observation sources (e.g. Kubernetes)",
		Long:  `Add, list, remove, or show status of observation sources such as Kubernetes (read-only workload observer).`,
		Example: `  beacon source add kubernetes my-k8s --kubeconfig ~/.kube/config --project myapp
  beacon source list --project myapp
  beacon source status my-k8s --project myapp`,
	}
	sourceCmd.AddCommand(k8sobserver.ListSourcesCommand())
	sourceCmd.AddCommand(k8sobserver.RemoveSourceCommand())
	sourceCmd.AddCommand(k8sobserver.StatusSourceCommand())
	addCmd := &cobra.Command{Use: "add", Short: "Add an observation source"}
	addCmd.AddCommand(k8sobserver.AddSourceCommand())
	sourceCmd.AddCommand(addCmd)
	return sourceCmd
}

func createMCPCommand() *cobra.Command {
	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP server for Cursor/Claude Desktop",
		Long: `Expose Beacon tools via Model Context Protocol.

Tools: inventory, status, logs, diff (read); deploy, restart (write, gated).
Set BEACON_MCP_DEPLOY_ENABLED=1 and BEACON_MCP_RESTART_ENABLED=1 to enable write tools.`,
	}

	var transport, listen, tokenEnv string
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Run MCP server",
		Long: `Run the Beacon MCP server. Use stdio for local Cursor/Claude Desktop,
or http for network access (requires --token-env for auth).`,
		Example: `  beacon mcp serve --transport stdio
  beacon mcp serve --transport http --listen 127.0.0.1:7766 --token-env BEACON_MCP_TOKEN`,
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()
			opts := mcp.ServeOptions{
				Transport: transport,
				Listen:    listen,
				TokenEnv:  tokenEnv,
			}
			if err := mcp.RunServe(ctx, opts); err != nil {
				log.Fatalf("MCP server: %v", err)
			}
		},
	}
	serveCmd.Flags().StringVar(&transport, "transport", "stdio", "Transport: stdio (local) or http")
	serveCmd.Flags().StringVar(&listen, "listen", "127.0.0.1:7766", "Listen address for http transport")
	serveCmd.Flags().StringVar(&tokenEnv, "token-env", "", "Env var name for bearer token (recommended for http)")

	mcpCmd.AddCommand(serveCmd)
	return mcpCmd
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
