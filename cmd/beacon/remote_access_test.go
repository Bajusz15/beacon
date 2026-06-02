package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"beacon/internal/remoteaccess"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return buf.String()
}

func executeRemoteAccessCommand(t *testing.T, args ...string) string {
	t.Helper()
	cmd := createRemoteAccessCommand()
	cmd.SetArgs(args)
	return captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute remote-access %v: %v", args, err)
		}
	})
}

func TestCreateRemoteAccessCommandShape(t *testing.T) {
	cmd := createRemoteAccessCommand()
	if cmd.Use != "remote-access" {
		t.Fatalf("unexpected use: %q", cmd.Use)
	}
	for _, name := range []string{
		"set-passphrase", "clear-passphrase", "status", "clear",
		"add-passkey", "list-passkeys", "remove-passkey", "setup-oob", "remove-oob",
	} {
		if _, _, err := cmd.Find([]string{name}); err != nil {
			t.Fatalf("missing subcommand %q: %v", name, err)
		}
	}
	set, _, err := cmd.Find([]string{"set-passphrase"})
	if err != nil {
		t.Fatalf("find set-passphrase: %v", err)
	}
	if set.Flags().Lookup("passphrase") == nil {
		t.Fatal("set-passphrase should expose --passphrase")
	}
	oob, _, err := cmd.Find([]string{"setup-oob"})
	if err != nil {
		t.Fatalf("find setup-oob: %v", err)
	}
	if oob.Flags().Lookup("if-absent") == nil {
		t.Fatal("setup-oob should expose --if-absent")
	}
}

func TestRemoteAccessCommandPassphraseLifecycle(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())

	out := executeRemoteAccessCommand(t, "status")
	if !strings.Contains(out, "NOT configured") {
		t.Fatalf("expected unconfigured status, got:\n%s", out)
	}

	out = executeRemoteAccessCommand(t, "set-passphrase", "--passphrase", "correct horse battery")
	if !strings.Contains(out, "Remote-access passphrase set") {
		t.Fatalf("expected set confirmation, got:\n%s", out)
	}

	out = executeRemoteAccessCommand(t, "status")
	for _, want := range []string{"CONFIGURED", "Passkeys: none enrolled", "Passphrase: configured"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected status to contain %q, got:\n%s", want, out)
		}
	}

	out = executeRemoteAccessCommand(t, "clear")
	if !strings.Contains(out, "Remote-access cleared") {
		t.Fatalf("expected clear confirmation, got:\n%s", out)
	}

	out = executeRemoteAccessCommand(t, "clear")
	if !strings.Contains(out, "nothing to clear") {
		t.Fatalf("expected no-op clear message, got:\n%s", out)
	}
}

func TestRemoteAccessPasskeyCommands(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())

	out := executeRemoteAccessCommand(t, "add-passkey")
	if !strings.Contains(out, "Passkey enrollment code:") || !strings.Contains(out, "expires in") {
		t.Fatalf("expected enrollment instructions, got:\n%s", out)
	}

	out = executeRemoteAccessCommand(t, "list-passkeys")
	if !strings.Contains(out, "No passkeys enrolled") {
		t.Fatalf("expected empty passkey list, got:\n%s", out)
	}

	err := remoteaccess.AddCredential(remoteaccess.PasskeyCredential{
		ID:        "cred-1",
		PublicKey: "cHVibGljLWtleQ==",
		RPID:      "beaconinfra.dev",
		Origin:    "https://beaconinfra.dev",
		Label:     "Laptop",
	})
	if err != nil {
		t.Fatalf("AddCredential: %v", err)
	}

	out = executeRemoteAccessCommand(t, "list-passkeys")
	for _, want := range []string{"Laptop", "cred-1", "beaconinfra.dev"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected list to contain %q, got:\n%s", want, out)
		}
	}

	out = executeRemoteAccessCommand(t, "remove-passkey", "Laptop")
	if !strings.Contains(out, "Passkey removed") {
		t.Fatalf("expected remove confirmation, got:\n%s", out)
	}
	if remoteaccess.IsConfigured() {
		t.Fatal("expected removing last passkey to disable the gate")
	}
}

func TestRemoteAccessOOBCommands(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())

	// A primary factor is required before the out-of-band factor.
	executeRemoteAccessCommand(t, "set-passphrase", "--passphrase", "correct horse battery")

	out := executeRemoteAccessCommand(t, "setup-oob")
	for _, want := range []string{"Scan this QR code", "secret", "Out-of-band verification enabled"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected setup-oob output to contain %q, got:\n%s", want, out)
		}
	}
	cfg, err := remoteaccess.Load()
	if err != nil || !cfg.HasOOB() {
		t.Fatalf("expected OOB enrolled after setup-oob, cfg=%+v err=%v", cfg, err)
	}
	secret := cfg.OOB.Secret

	out = executeRemoteAccessCommand(t, "status")
	if !strings.Contains(out, "Out-of-band: authenticator app enrolled") {
		t.Fatalf("expected status to show OOB enrolled, got:\n%s", out)
	}

	// --if-absent must leave the existing secret unchanged.
	out = executeRemoteAccessCommand(t, "setup-oob", "--if-absent")
	if !strings.Contains(out, "already enabled") {
		t.Fatalf("expected --if-absent to report no change, got:\n%s", out)
	}
	cfg, _ = remoteaccess.Load()
	if cfg.OOB.Secret != secret {
		t.Fatal("--if-absent must not rotate the existing secret")
	}

	out = executeRemoteAccessCommand(t, "remove-oob")
	if !strings.Contains(out, "Out-of-band authenticator factor removed") {
		t.Fatalf("expected remove-oob confirmation, got:\n%s", out)
	}
	cfg, _ = remoteaccess.Load()
	if cfg.HasOOB() {
		t.Fatal("expected OOB removed")
	}
}

func TestRemoteAccessClearPassphraseKeepsPasskey(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())

	executeRemoteAccessCommand(t, "set-passphrase", "--passphrase", "correct horse battery")
	if err := remoteaccess.AddCredential(remoteaccess.PasskeyCredential{
		ID: "cred-1", PublicKey: "cHVibGljLWtleQ==", RPID: "beaconinfra.dev", Origin: "https://beaconinfra.dev",
	}); err != nil {
		t.Fatalf("AddCredential: %v", err)
	}

	// clear-passphrase removes only the passphrase; the passkey (and thus the
	// gate) must survive — this is what the HA add-on relies on.
	executeRemoteAccessCommand(t, "clear-passphrase")

	cfg, err := remoteaccess.Load()
	if err != nil {
		t.Fatalf("Load after clear-passphrase: %v", err)
	}
	if cfg.HasPassphrase() {
		t.Fatal("clear-passphrase must remove the passphrase")
	}
	if !cfg.HasCredentials() {
		t.Fatal("clear-passphrase must keep enrolled passkeys")
	}
	if !remoteaccess.IsConfigured() {
		t.Fatal("gate must stay configured while a passkey remains")
	}

	// Safe to run when no passphrase is set (the add-on calls it on every start).
	executeRemoteAccessCommand(t, "clear-passphrase")
}
