package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"beacon/internal/config"
	"beacon/internal/systemd"

	"github.com/spf13/cobra"
)

func createLogsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show the Beacon agent logs",
		Long: `Show the Beacon master agent's logs.

When Beacon runs as a systemd service, this reads that unit's journal. Otherwise it
reads ~/.beacon/master.log (written by a detached "beacon start"). No need to know the
unit name or journalctl flags.`,
		Example: `  beacon logs          # last 100 lines
  beacon logs -f       # follow (like tail -f)
  beacon logs -n 500   # last 500 lines`,
		RunE: runLogs,
	}
	cmd.Flags().BoolP("follow", "f", false, "Follow the log output (Ctrl-C to stop)")
	cmd.Flags().IntP("lines", "n", 100, "Number of recent lines to show")
	return cmd
}

func runLogs(cmd *cobra.Command, _ []string) error {
	follow, _ := cmd.Flags().GetBool("follow")
	lines, _ := cmd.Flags().GetInt("lines")

	// Prefer the systemd journal when the master runs as a service.
	if scope, unit, found := systemd.DetectMasterUnit(); found {
		if _, err := exec.LookPath("journalctl"); err == nil {
			args := []string{}
			if scope == systemd.UserService {
				args = append(args, "--user")
			}
			args = append(args, "-u", unit, "-n", strconv.Itoa(lines), "--no-pager")
			if follow {
				args = append(args, "-f")
			}
			return streamCommand("journalctl", args...)
		}
	}

	// Otherwise fall back to the detached-start log file.
	home, err := config.BeaconHomeDir()
	if err != nil {
		return err
	}
	logPath := filepath.Join(home, "master.log")
	if _, err := os.Stat(logPath); err != nil {
		return fmt.Errorf("no logs found: no Beacon systemd service, and %s does not exist.\nIs Beacon running? Check `beacon status`, or start it with `beacon start`", logPath)
	}
	args := []string{"-n", strconv.Itoa(lines)}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, logPath)
	return streamCommand("tail", args...)
}

// streamCommand runs a command with the terminal's stdio attached (for tail/journalctl).
func streamCommand(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}
