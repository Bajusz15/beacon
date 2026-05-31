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
	for _, name := range []string{"set-passphrase", "status", "clear", "add-passkey", "list-passkeys", "remove-passkey"} {
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
