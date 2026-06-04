package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"beacon/internal/remoteaccess"

	"github.com/mdp/qrterminal/v3"
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
		Short: "Remove ALL remote-access factors (passphrase and passkeys)",
		Run:   runRemoteAccessClear,
	}

	clearPassphraseCmd := &cobra.Command{
		Use:   "clear-passphrase",
		Short: "Remove only the passphrase factor, leaving passkeys and OOB intact",
		Run:   runRemoteAccessClearPassphrase,
	}

	addPasskeyCmd := &cobra.Command{
		Use:   "add-passkey",
		Short: "Authorize enrolling a passkey: prints a one-time enrollment code",
		Long: `Generate a one-time, device-local enrollment code. Enter it in the
BeaconInfra dashboard within a few minutes to register a passkey (Touch ID,
Windows Hello, a security key, or your phone). The passkey's private key never
leaves your authenticator; the device stores only its public key.

Passkeys are the preferred remote-access factor; a passphrase, if set, is the
fallback. Rooting enrollment in this device-local code means a compromised cloud
cannot enroll a passkey on its own.`,
		Run: runRemoteAccessAddPasskey,
	}

	listPasskeysCmd := &cobra.Command{
		Use:   "list-passkeys",
		Short: "List enrolled passkeys",
		Run:   runRemoteAccessListPasskeys,
	}

	removePasskeyCmd := &cobra.Command{
		Use:   "remove-passkey <id-or-label>",
		Short: "Remove an enrolled passkey by credential id or label",
		Args:  cobra.ExactArgs(1),
		Run:   runRemoteAccessRemovePasskey,
	}

	setupOOBCmd := &cobra.Command{
		Use:   "setup-oob",
		Short: "Enroll an authenticator app as a second, out-of-band factor (shows a QR)",
		Long: `Generate a TOTP secret and print a QR code to scan into any authenticator
app (Google Authenticator, 1Password, Authy, …). Once enrolled, every remote
unlock also requires the current 6-digit code from that app — a second factor
generated on your phone and verified on this device, which the cloud never sees.

A passphrase or passkey must already be configured first.`,
		Run: runRemoteAccessSetupOOB,
	}

	setupOOBCmd.Flags().Bool("if-absent", false, "Do nothing if an authenticator factor is already enrolled (idempotent setup)")

	removeOOBCmd := &cobra.Command{
		Use:   "remove-oob",
		Short: "Remove the out-of-band authenticator-app factor",
		Run:   runRemoteAccessRemoveOOB,
	}

	root.AddCommand(setCmd, statusCmd, clearCmd, clearPassphraseCmd, addPasskeyCmd, listPasskeysCmd, removePasskeyCmd, setupOOBCmd, removeOOBCmd)
	return root
}

func runRemoteAccessClearPassphrase(cmd *cobra.Command, args []string) {
	if err := remoteaccess.ClearPassphrase(); err != nil && !errors.Is(err, os.ErrNotExist) {
		logger.Fatalf("clear passphrase: %v", err)
	}
}

func runRemoteAccessSetupOOB(cmd *cobra.Command, args []string) {
	// With --if-absent, leave an already-enrolled authenticator factor untouched
	// (so an add-on that re-runs this on every start does not rotate the secret
	// and lock the user out).
	if ifAbsent, _ := cmd.Flags().GetBool("if-absent"); ifAbsent && remoteaccess.IsConfigured() {
		if cfg, err := remoteaccess.Load(); err == nil && cfg.HasOOB() {
			fmt.Println("  Out-of-band verification already enabled; leaving it unchanged.")
			return
		}
	}
	secret, err := remoteaccess.GenerateTOTPSecret()
	if err != nil {
		logger.Fatalf("generate TOTP secret: %v", err)
	}
	if err := remoteaccess.SetOOB(secret); err != nil {
		logger.Fatalf("enable out-of-band verification: %v", err)
	}
	account, err := os.Hostname()
	if err != nil || strings.TrimSpace(account) == "" {
		account = "beacon-device"
	}
	otpURL := remoteaccess.OTPAuthURL(secret, account, "Beacon")

	fmt.Println()
	fmt.Println("  Scan this QR code with your authenticator app:")
	fmt.Println()
	qrterminal.GenerateWithConfig(otpURL, qrterminal.Config{
		Level:     qrterminal.M,
		Writer:    os.Stdout,
		BlackChar: qrterminal.BLACK,
		WhiteChar: qrterminal.WHITE,
		QuietZone: 1,
	})
	fmt.Println()
	fmt.Printf("  Or enter this secret manually:  %s\n", secret)
	fmt.Println()
	fmt.Println("  ✓ Out-of-band verification enabled.")
	fmt.Println("  Remote unlocks now also require the 6-digit code from your app.")
	fmt.Println("  Restart beacon for it to take effect on a running agent.")
	fmt.Println()
}

func runRemoteAccessRemoveOOB(cmd *cobra.Command, args []string) {
	if err := remoteaccess.ClearOOB(); err != nil {
		logger.Fatalf("remove out-of-band verification: %v", err)
	}
	fmt.Println()
	fmt.Println("  ✓ Out-of-band authenticator factor removed.")
	fmt.Println()
}

func runRemoteAccessAddPasskey(cmd *cobra.Command, args []string) {
	code, err := remoteaccess.SetEnrollToken(remoteaccess.DefaultEnrollTTL)
	if err != nil {
		logger.Fatalf("generate enrollment code: %v", err)
	}
	fmt.Println()
	fmt.Printf("  Passkey enrollment code:  %s\n", code)
	fmt.Println()
	fmt.Println("  Open the device in the BeaconInfra dashboard, choose \"Add passkey\",")
	fmt.Println("  and enter this code, then follow your browser's passkey prompt.")
	fmt.Printf("  The code expires in %s and can be used once.\n", remoteaccess.DefaultEnrollTTL)
	fmt.Println()
}

func runRemoteAccessListPasskeys(cmd *cobra.Command, args []string) {
	creds, err := remoteaccess.ListCredentials()
	if err != nil {
		logger.Fatalf("list passkeys: %v", err)
	}
	fmt.Println()
	if len(creds) == 0 {
		fmt.Println("  No passkeys enrolled.")
		fmt.Println("  Add one with:  beacon remote-access add-passkey")
		fmt.Println()
		return
	}
	for _, c := range creds {
		label := c.Label
		if label == "" {
			label = "(unlabeled)"
		}
		fmt.Printf("  • %s\n", label)
		fmt.Printf("      id:    %s\n", c.ID)
		fmt.Printf("      rp:    %s\n", c.RPID)
		if c.AddedAt != "" {
			fmt.Printf("      added: %s\n", c.AddedAt)
		}
	}
	fmt.Println()
}

func runRemoteAccessRemovePasskey(cmd *cobra.Command, args []string) {
	if err := remoteaccess.RemoveCredential(strings.TrimSpace(args[0])); err != nil {
		logger.Fatalf("remove passkey: %v", err)
	}
	fmt.Println()
	fmt.Println("  ✓ Passkey removed.")
	fmt.Println()
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
		fmt.Println("  Remote-access gate: NOT configured")
		fmt.Println("  Remote terminal/tunnel sessions are NOT gated.")
		fmt.Println()
		fmt.Println("  Add a passkey:   beacon remote-access add-passkey")
		fmt.Println("  Or a passphrase: beacon remote-access set-passphrase")
		fmt.Println()
		return
	}
	cfg, err := remoteaccess.Load()
	if err != nil {
		logger.Fatalf("read remote-access config: %v", err)
	}
	fmt.Println()
	fmt.Println("  Remote-access gate: CONFIGURED")
	if cfg.HasCredentials() {
		fmt.Printf("  Passkeys: %d enrolled (preferred)\n", len(cfg.Credentials))
	} else {
		fmt.Println("  Passkeys: none enrolled")
	}
	if cfg.HasPassphrase() {
		fmt.Println("  Passphrase: configured (fallback)")
	} else {
		fmt.Println("  Passphrase: not set")
	}
	if cfg.HasOOB() {
		fmt.Println("  Out-of-band: authenticator app enrolled (required at unlock)")
	} else {
		fmt.Println("  Out-of-band: not set")
	}
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
		fmt.Println("  No remote-access factors configured; nothing to clear.")
		return
	}
	if err := remoteaccess.Clear(); err != nil && !errors.Is(err, os.ErrNotExist) {
		logger.Fatalf("clear remote-access: %v", err)
	}
	fmt.Println()
	fmt.Println("  ✓ Remote-access cleared (passphrase and passkeys). Sessions are no longer gated.")
	fmt.Println()
}
