package main

import (
	"fmt"
	"os"

	"beacon/internal/identity"
	"beacon/internal/systemd"

	"github.com/spf13/cobra"
)

func createServiceCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "service",
		Short: "Install and manage Beacon as a background service",
		Long: `Install Beacon as a systemd service so the agent runs in the background, starts on
boot, and is supervised (auto-restart). A system service is used when run as root,
otherwise a per-user service.

Only Linux with systemd is supported. On macOS or a system without systemd, run
"beacon start" instead (or set up a launchd job).`,
	}
	root.AddCommand(
		&cobra.Command{
			Use:   "install",
			Short: "Install, enable, and start the Beacon service",
			RunE:  runServiceInstall,
		},
		&cobra.Command{
			Use:   "uninstall",
			Short: "Stop, disable, and remove the Beacon service",
			RunE:  runServiceUninstall,
		},
		&cobra.Command{
			Use:   "status",
			Short: "Show the Beacon service status",
			RunE:  runServiceStatus,
		},
	)
	return root
}

// serviceScope picks a system service when running as root, else a per-user service.
func serviceScope() systemd.ServiceType {
	if os.Geteuid() == 0 {
		return systemd.SystemService
	}
	return systemd.UserService
}

// ensureSystemd returns a manager if systemd is usable in this scope, or a helpful error.
func ensureSystemd() (*systemd.ServiceManager, error) {
	sm := systemd.NewServiceManager(serviceScope())
	if !sm.IsAvailable() {
		return nil, fmt.Errorf("systemd is not available here.\nRun `beacon start` to run the agent directly (macOS uses launchd; `beacon service` is Linux/systemd only)")
	}
	return sm, nil
}

func runServiceInstall(_ *cobra.Command, _ []string) error {
	sm, err := ensureSystemd()
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot resolve the beacon binary path: %w", err)
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/root"
	}

	execLine := fmt.Sprintf("%s start --foreground", exe)
	if err := sm.CreateMasterService(execLine, home); err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	if err := sm.ReloadDaemon(); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}
	if err := sm.EnableMasterService(); err != nil {
		return fmt.Errorf("enable service: %w", err)
	}
	if err := sm.StartMasterService(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}

	scope := "system"
	if serviceScope() == systemd.UserService {
		scope = "user"
	}
	fmt.Println()
	fmt.Printf("  ✓ Beacon installed as a %s service (starts on boot, auto-restarts)\n", scope)
	fmt.Println()
	fmt.Println("    beacon logs -f     # watch it")
	fmt.Println("    beacon status      # health")
	fmt.Println("    beacon restart     # restart")
	if !cloudConfigured() {
		fmt.Println()
		fmt.Println("  It's running locally. Connect it to the cloud any time:")
		fmt.Println("    beacon cloud login --api-key bci_live_...")
	}
	fmt.Println()
	return nil
}

func runServiceUninstall(_ *cobra.Command, _ []string) error {
	if _, err := ensureSystemd(); err != nil {
		return err
	}
	scope, unit, found := systemd.DetectMasterUnit()
	if !found {
		fmt.Println("  No Beacon service is installed.")
		return nil
	}
	sm := systemd.NewServiceManager(scope)
	if err := sm.StopUnit(unit); err != nil {
		fmt.Printf("  (stop %s: %v)\n", unit, err)
	}
	if err := sm.DisableUnit(unit); err != nil {
		fmt.Printf("  (disable %s: %v)\n", unit, err)
	}
	path := sm.UnitFilePath(unit)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	if err := sm.ReloadDaemon(); err != nil {
		fmt.Printf("  (daemon-reload: %v)\n", err)
	}
	fmt.Printf("  ✓ Removed %s\n", unit)
	return nil
}

func runServiceStatus(_ *cobra.Command, _ []string) error {
	if _, err := ensureSystemd(); err != nil {
		return err
	}
	scope, unit, found := systemd.DetectMasterUnit()
	if !found {
		fmt.Println("  No Beacon service is installed. Install it with: beacon service install")
		return nil
	}
	args := []string{}
	if scope == systemd.UserService {
		args = append(args, "--user")
	}
	args = append(args, "status", unit, "--no-pager")
	return streamCommand("systemctl", args...)
}

// cloudConfigured reports whether an API key + cloud reporting are set.
func cloudConfigured() bool {
	uc, err := identity.LoadUserConfig()
	return err == nil && uc != nil && uc.CloudReportingEnabled && uc.APIKey != ""
}
