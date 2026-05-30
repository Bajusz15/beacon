package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"beacon/internal/remoteaccess"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func createRemoteAccessCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "remote-access",
		Short: "Device-verified passphrase that gates remote terminal/tunnel sessions",
		Long: `Require a locally-set passphrase before BeaconInfra can open a remote
terminal or tunnel on this device. The passphrase is verified ON THIS DEVICE —
BeaconInfra is only a relay and never sees the passphrase or a reusable proof.
A fully compromised cloud cannot open a session without this secret.

Setting a passphrase turns the gate on. With none set, behavior is unchanged.`,
	}

	setCmd := &cobra.Command{
		Use:   "set-passphrase",
		Short: "Set or change the remote-access passphrase (prompted twice, no echo)",
		Run:   runSetPassphrase,
	}
	setCmd.Flags().String("passphrase", "", "Passphrase (non-interactive; else prompted). Use with care.")

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show whether a passphrase is configured",
		Run:   runRemoteAccessStatus,
	}

	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Remove the passphrase, disabling the gate (local recovery)",
		Run:   runRemoteAccessClear,
	}

	root.AddCommand(setCmd, statusCmd, clearCmd)
	return root
}

func runSetPassphrase(cmd *cobra.Command, args []string) {
	pp, _ := cmd.Flags().GetString("passphrase")
	if pp == "" {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			logger.Fatalf("beacon remote-access set-passphrase: non-interactive terminal; use --passphrase")
		}
		entered, err := promptPassphraseTwice()
		if err != nil {
			logger.Fatalf("%v", err)
		}
		pp = entered
	}
	pp = strings.TrimSpace(pp)

	if err := remoteaccess.SetPassphrase(pp); err != nil {
		logger.Fatalf("set passphrase: %v", err)
	}

	fmt.Println()
	fmt.Println("  ✓ Remote-access passphrase set.")
	fmt.Println()
	fmt.Println("  Remote terminal and tunnel sessions now require this passphrase,")
	fmt.Println("  verified locally on this device. Restart beacon for it to take effect")
	fmt.Println("  on a running agent (the gate is read at session time).")
	fmt.Println()
}

// promptPassphraseTwice reads a passphrase (no echo) twice and verifies they
// match. The caller must ensure stdin is an interactive terminal.
func promptPassphraseTwice() (string, error) {
	fmt.Fprint(os.Stderr, "New remote-access passphrase: ")
	b1, err := term.ReadPassword(syscall.Stdin)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read passphrase: %w", err)
	}
	fmt.Fprint(os.Stderr, "Confirm passphrase: ")
	b2, err := term.ReadPassword(syscall.Stdin)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read passphrase: %w", err)
	}
	if string(b1) != string(b2) {
		return "", fmt.Errorf("passphrases do not match")
	}
	return string(b1), nil
}

func runRemoteAccessStatus(cmd *cobra.Command, args []string) {
	if !remoteaccess.IsConfigured() {
		fmt.Println()
		fmt.Println("  Remote-access passphrase: NOT configured")
		fmt.Println("  Remote terminal/tunnel sessions are NOT gated.")
		fmt.Println()
		fmt.Println("  Set one with:  beacon remote-access set-passphrase")
		fmt.Println()
		return
	}
	cfg, err := remoteaccess.Load()
	if err != nil {
		logger.Fatalf("read remote-access config: %v", err)
	}
	fmt.Println()
	fmt.Println("  Remote-access passphrase: CONFIGURED")
	if cfg.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, cfg.UpdatedAt); err == nil {
			fmt.Printf("  Last updated: %s\n", t.Local().Format("2006-01-02 15:04:05"))
		}
	}
	fmt.Println("  Remote terminal/tunnel sessions require a per-session unlock.")
	fmt.Println()
	fmt.Println("  Note: active in-memory unlocks live in the running agent and are")
	fmt.Println("  cleared on restart (fail-closed).")
	fmt.Println()
}

func runRemoteAccessClear(cmd *cobra.Command, args []string) {
	if !remoteaccess.IsConfigured() {
		fmt.Println("  No remote-access passphrase configured; nothing to clear.")
		return
	}
	if err := remoteaccess.Clear(); err != nil && !errors.Is(err, os.ErrNotExist) {
		logger.Fatalf("clear passphrase: %v", err)
	}
	fmt.Println()
	fmt.Println("  ✓ Remote-access passphrase cleared. Sessions are no longer gated.")
	fmt.Println()
}
