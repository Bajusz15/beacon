package main

import (
	"beacon/internal/config"
	"beacon/internal/deploy"
	"beacon/internal/server"
	"beacon/internal/state"
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
3. beacon monitor - runs health checks and monitoring (not yet implemented)`,
}

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap [project-name]",
	Short: "Bootstrap a project with systemd integration",
	Long: `Bootstrap creates the necessary directory structure, configuration files,
and systemd services for a beacon project. If no project name is provided,
you will be prompted for one.`,
	Example: `  beacon bootstrap myapp
  beacon bootstrap
  beacon bootstrap myapp --force --skip-systemd`,
	Run: func(cmd *cobra.Command, args []string) {
		bootstrap.Run(cmd, args)
	},
}

func init() {
	bootstrapCmd.Flags().BoolP("force", "f", false, "Force overwrite of existing components")
	bootstrapCmd.Flags().BoolP("skip-systemd", "s", false, "Skip systemd service setup")
}

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Run health checks and report results",
	Long: `Monitor runs health checks based on configuration and reports results.
This command is not yet fully implemented.`,
	Run: func(cmd *cobra.Command, args []string) {
		monitor.Run(cmd, args)
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
	rootCmd.AddCommand(bootstrapCmd)
	rootCmd.AddCommand(monitorCmd)
	rootCmd.AddCommand(deployCmd)

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
