package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"beacon/internal/alerting"
	"beacon/internal/config"
	"beacon/internal/deploy"
	"beacon/internal/keys"
	"beacon/internal/projects"
	"beacon/internal/server"
	"beacon/internal/state"
	"beacon/internal/version"
	"beacon/internal/wizard"

	"beacon/internal/bootstrap"
	"beacon/internal/child"
	"beacon/internal/identity"
	"beacon/internal/master"
	"beacon/internal/mcp"
	"beacon/internal/monitor"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "beacon",
	Short: "Beacon - IoT deployment and monitoring agent",
	Long: `Beacon is a lightweight agent for self-hosted devices — deploy, monitor, and report health.

  With no subcommand, this help is shown. Use beacon start for the master agent (dashboard, tunnels, heartbeats), or beacon deploy to poll Git/Docker for releases.

  beacon init      write local ~/.beacon/config.yaml (no network)
  beacon cloud login  save BeaconInfra API key (after local setup)
  beacon start     start the master agent — manages projects, tunnels, local dashboard
  beacon bootstrap set up a new project (interactive or from a config file)
  beacon monitor   run a single project's health checks (dev/debug)
  beacon deploy    poll a Git repo for new tags and deploy
  beacon update    self-update to the latest release
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
Available services: deploy, monitor, start (cloud agent: systemctl --user restart beacon.service)`,
	Example: `  beacon restart
  beacon restart deploy
  beacon restart monitor
  beacon restart start`,
	Run: func(cmd *cobra.Command, args []string) {
		service := "deploy"
		if len(args) > 0 {
			service = args[0]
		}

		switch service {
		case "deploy":
			logger.Infof("Restarting deploy service...")
			logger.Infof("Deploy service restart requested")
		case "monitor":
			logger.Infof("Restarting monitor service...")
			logger.Infof("Monitor service restart requested")
		case "start", "master":
			logger.Infof("Restart master: systemctl --user restart beacon.service")
		default:
			logger.Infof("Unknown service: %s. Available services: deploy, monitor, start\n", service)
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
			logger.Fatalf("Wizard failed: %v", err)
		}
	},
}

func init() {
	wizardCmd.Flags().StringP("config", "c", "beacon.monitor.yml", "Path to monitor configuration file")
	wizardCmd.Flags().StringP("env", "e", ".env", "Path to environment file")
}

var initAgentCmd = &cobra.Command{
	Use:   "init",
	Short: "Write local machine config to ~/.beacon/config.yaml (no network)",
	Long: `Creates or updates ~/.beacon/config.yaml with local settings only. No HTTP requests are made.

Sets device_name (default: system hostname) and optional metrics port. New configs get cloud_reporting_enabled: false; existing configs keep their current value.
Does not store an API key — use "beacon cloud login" after you have a BeaconInfra account.

Environment: BEACON_DEVICE_NAME for default device name when --name is omitted.`,
	Example: `  beacon init
  beacon init --name my-pi
  beacon init --metrics-port 9100`,
	Run: func(cmd *cobra.Command, args []string) {
		metricsPort, _ := cmd.Flags().GetInt("metrics-port")
		name, _ := cmd.Flags().GetString("name")
		if name == "" {
			name, _ = cmd.Flags().GetString("device-name")
		}
		if name == "" {
			name = os.Getenv("BEACON_DEVICE_NAME")
		}

		if err := identity.WriteUserLocalInit(name, metricsPort); err != nil {
			logger.Fatalf("beacon init: %v", err)
		}

		cfg, _ := identity.LoadUserConfig()
		deviceName := ""
		if cfg != nil {
			deviceName = cfg.DeviceName
		}

		fmt.Println()
		fmt.Println("  ✓ Config written to ~/.beacon/config.yaml")
		if deviceName != "" {
			fmt.Printf("  ✓ Device name: %s\n", deviceName)
		}
		fmt.Println()
		fmt.Println("  Next step — authenticate with BeaconInfra:")
		fmt.Println()
		fmt.Println("    beacon cloud login --api-key YOUR_KEY")
		fmt.Println()
		fmt.Println("  Get an API key at https://beaconinfra.dev → API Keys")
		fmt.Println()
	},
}

var masterCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"master"},
	Short:   "Start the master agent (detaches to background by default)",
	Long: `Reads ~/.beacon/config.yaml, manages project agents and tunnel connections,
serves a local dashboard, and sends heartbeats to BeaconInfra cloud.

By default the process detaches from the terminal. Use --foreground to keep it
in the foreground (useful for systemd, Docker, or debugging).`,
	Run: func(cmd *cobra.Command, args []string) {
		foreground, _ := cmd.Flags().GetBool("foreground")

		if !foreground {
			// Re-exec ourselves with --foreground in a detached process
			execPath, err := os.Executable()
			if err != nil {
				logger.Fatalf("Cannot find executable: %v", err)
			}

			// Build args: beacon start --foreground (pass through any other flags)
			childArgs := []string{"start", "--foreground"}

			logPath := filepath.Join(os.Getenv("HOME"), ".beacon", "master.log")
			if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
				logger.Fatalf("Cannot create log dir: %v", err)
			}
			logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				logger.Fatalf("Cannot open log file %s: %v", logPath, err)
			}

			proc := &os.ProcAttr{
				Dir:   "/",
				Env:   os.Environ(),
				Files: []*os.File{os.Stdin, logFile, logFile},
				Sys:   daemonSysProcAttr(),
			}
			p, err := os.StartProcess(execPath, append([]string{execPath}, childArgs...), proc)
			if err != nil {
				logger.Fatalf("Failed to start background process: %v", err)
			}
			_ = logFile.Close()
			_ = p.Release()

			fmt.Println()
			fmt.Printf("  ✓ Beacon started (pid %d)\n", p.Pid)
			fmt.Printf("  ✓ Logs: %s\n", logPath)
			fmt.Printf("  ✓ Dashboard: http://127.0.0.1:9100\n")
			fmt.Println()
			fmt.Println("  Your device will appear in BeaconInfra after the first")
			fmt.Println("  heartbeat (~30 seconds). View it at https://beaconinfra.dev")
			fmt.Println()
			return
		}

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

	masterCmd.Flags().Bool("foreground", false, "Run in the foreground (don't detach)")

	initAgentCmd.Flags().Int("metrics-port", 0, "Metrics/dashboard port (0 = leave unchanged)")
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
			logger.Fatalf("agent:Failed to initialize: %v", err)
		}

		if err := c.Run(); err != nil {
			logger.Fatalf("agent:Failed: %v", err)
		}
	},
}

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Run beacon in deployment mode (poll Git for new tags and deploy)",
	Long: `Run beacon in deployment mode: polls Git repositories for new tags
and automatically deploys them. Must be run explicitly: beacon deploy`,
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
	rootCmd.AddCommand(keys.KeysCmd)
	rootCmd.AddCommand(alerting.CreateSimpleAlertingCommand())
	rootCmd.AddCommand(projects.CreateProjectCommand())
	rootCmd.AddCommand(createMCPCommand())
	rootCmd.AddCommand(createConfigCommand())
	rootCmd.AddCommand(createCloudCommand())
	rootCmd.AddCommand(createTunnelCommand())
	rootCmd.AddCommand(createVPNCommand())
	rootCmd.AddCommand(createUpdateCommand())

	// If no subcommand is provided, show help (matches common CLI expectations).
	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	}

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
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
				logger.Fatalf("MCP server: %v", err)
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
	logger.Infof("Deploy agent starting...")

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
			logger.Infof("Shutdown signal received, stopping...")
			return
		case <-ticker.C:
			deploy.CheckForNewTag(cfg, status)
		}
	}
}
