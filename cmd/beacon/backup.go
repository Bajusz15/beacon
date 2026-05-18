package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func createBackupCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "backup",
		Short: "Trigger a manual backup or view backup status",
		Long: `Beacon Backup — scheduled restic backups of configured directories.

Configure backup in ~/.beacon/config.yaml:

  backup:
    enabled: true
    schedule: "0 3 * * *"
    paths:
      - ~/
      - /etc/
    exclude:
      - "*.tmp"
      - node_modules
      - .cache
    destination: "/mnt/backup/beacon"
    password_file: ~/.beacon/restic-password
    retention:
      keep_daily: 7
      keep_weekly: 4
      keep_monthly: 6

Requires restic to be installed (https://restic.net).`,
		RunE: runBackupNow,
	}
	root.Flags().Int("port", 9100, "Master metrics port")

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show last backup result and schedule",
		RunE:  runBackupStatus,
	}
	statusCmd.Flags().Int("port", 9100, "Master metrics port")

	root.AddCommand(statusCmd)
	return root
}

func runBackupNow(cmd *cobra.Command, _ []string) error {
	port, _ := cmd.Flags().GetInt("port")
	url := fmt.Sprintf("http://127.0.0.1:%d/api/backup/run", port)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(url, "application/json", nil)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return fmt.Errorf("beacon start is not running (port %d)", port)
		}
		return fmt.Errorf("connecting to master: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		fmt.Println("Backup started. Run 'beacon backup status' to monitor progress.")
	case http.StatusConflict:
		fmt.Println("Backup already in progress.")
	case http.StatusBadRequest:
		return fmt.Errorf("backup not configured — add a 'backup:' section to ~/.beacon/config.yaml")
	default:
		return fmt.Errorf("unexpected response: HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func runBackupStatus(cmd *cobra.Command, _ []string) error {
	port, _ := cmd.Flags().GetInt("port")
	snap, err := fetchStatus(port)
	if err != nil {
		return err
	}

	if snap.Backup == nil {
		fmt.Println("Backup is not configured.")
		fmt.Println("\nAdd a 'backup:' section to ~/.beacon/config.yaml to enable.")
		return nil
	}

	b := snap.Backup
	if err := printBackupLine("%-16s %s\n", "Enabled:", formatBool(b.Enabled)); err != nil {
		return err
	}
	if err := printBackupLine("%-16s %s\n", "Schedule:", valueOrDash(b.Schedule)); err != nil {
		return err
	}
	if err := printBackupLine("%-16s %s\n", "Running:", formatBool(b.Running)); err != nil {
		return err
	}

	if !b.LastRunAt.IsZero() {
		if err := printBackupLine("%-16s %s\n", "Last run:", b.LastRunAt.Local().Format("2006-01-02 15:04:05")); err != nil {
			return err
		}
		if err := printBackupLine("%-16s %.1fs\n", "Duration:", b.LastDurationS); err != nil {
			return err
		}
		if b.LastSuccess {
			if err := printBackupLine("%-16s OK\n", "Result:"); err != nil {
				return err
			}
		} else {
			if err := printBackupLine("%-16s FAILED: %s\n", "Result:", b.LastError); err != nil {
				return err
			}
		}
	}

	if !b.NextRunAt.IsZero() {
		if err := printBackupLine("%-16s %s\n", "Next run:", b.NextRunAt.Local().Format("2006-01-02 15:04:05")); err != nil {
			return err
		}
	}

	if b.SnapshotCount > 0 {
		if err := printBackupLine("%-16s %d\n", "Snapshots:", b.SnapshotCount); err != nil {
			return err
		}
	}

	return nil
}

func printBackupLine(format string, args ...any) error {
	_, err := fmt.Fprintf(os.Stdout, format, args...)
	return err
}

func formatBool(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func valueOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
