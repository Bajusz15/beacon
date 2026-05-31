package remoteaccess

import (
	"testing"
	"time"
)

// recordingNotifier captures the last OOB code it was asked to deliver.
type recordingNotifier struct {
	code string
	fail bool
}

func (r *recordingNotifier) SendOOBCode(action, code string) error {
	if r.fail {
		return errInjected
	}
	r.code = code
	return nil
}

var errInjected = &injErr{}

type injErr struct{}

func (*injErr) Error() string { return "inject" }

// Legacy passphrase-only configs must still load, and adding a passkey must not
// destroy the passphrase (and vice versa).
func TestStoreBackCompatAndMerge(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())

	if err := SetPassphrase("correct horse battery"); err != nil {
		t.Fatalf("SetPassphrase: %v", err)
	}
	cfg, err := Load()
	if err != nil || !cfg.HasPassphrase() {
		t.Fatalf("expected passphrase loaded, got cfg=%+v err=%v", cfg, err)
	}

	cred := PasskeyCredential{ID: "cred-1", PublicKey: "cG9wa2V5", RPID: "beaconinfra.dev", Origin: "https://beaconinfra.dev"}
	if err := AddCredential(cred); err != nil {
		t.Fatalf("AddCredential: %v", err)
	}
	cfg, _ = Load()
	if !cfg.HasPassphrase() || !cfg.HasCredentials() {
		t.Fatalf("expected both factors after AddCredential, got %+v", cfg)
	}

	// Clearing the passphrase keeps the passkey and the gate stays configured.
	if err := ClearPassphrase(); err != nil {
		t.Fatalf("ClearPassphrase: %v", err)
	}
	if !IsConfigured() {
		t.Fatal("expected gate still configured (passkey remains)")
	}
	cfg, _ = Load()
	if cfg.HasPassphrase() || !cfg.HasCredentials() {
		t.Fatalf("expected passkey-only after ClearPassphrase, got %+v", cfg)
	}

	// Removing the last passkey disables the gate entirely.
	if err := RemoveCredential("cred-1"); err != nil {
		t.Fatalf("RemoveCredential: %v", err)
	}
	if IsConfigured() {
		t.Fatal("expected gate disabled after removing last factor")
	}
}

// When an OOB notifier is configured, the delivered code is required at unlock.
func TestOOBGatingPassphrase(t *testing.T) {
	const pp = "correct horse battery"
	setupConfigured(t, pp)

	g := NewGrants()
	note := &recordingNotifier{}
	g.SetNotifier(note)

	ch, err := g.Challenge("terminal_open", "sess-1")
	if err != nil {
		t.Fatalf("Challenge: %v", err)
	}
	if note.code == "" {
		t.Fatal("expected an OOB code to be delivered")
	}
	proof := browserProof(t, pp, ch, "terminal_open", "sess-1")

	// Wrong OOB code is rejected.
	if err := g.Verify("terminal_open", "sess-1", ch.Nonce, proof, "000000"); err != ErrBadOOBCode {
		t.Fatalf("expected ErrBadOOBCode, got %v", err)
	}

	// Correct OOB code unlocks (fresh challenge, since the failure consumed a try).
	ch2, _ := g.Challenge("terminal_open", "sess-2")
	proof2 := browserProof(t, pp, ch2, "terminal_open", "sess-2")
	if err := g.Verify("terminal_open", "sess-2", ch2.Nonce, proof2, note.code); err != nil {
		t.Fatalf("expected unlock with correct OOB code, got %v", err)
	}
	if !g.Consume("terminal_open", "sess-2") {
		t.Fatal("expected a consumable grant after OOB unlock")
	}
}

// A failed OOB delivery keeps the gate closed (challenge errors).
func TestOOBDeliveryFailureClosesGate(t *testing.T) {
	setupConfigured(t, "correct horse battery")
	g := NewGrants()
	g.SetNotifier(&recordingNotifier{fail: true})
	if _, err := g.Challenge("terminal_open", "sess-1"); err != ErrOOBDelivery {
		t.Fatalf("expected ErrOOBDelivery, got %v", err)
	}
}

func TestEnrollTokenStore(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())

	code, err := SetEnrollToken(DefaultEnrollTTL)
	if err != nil {
		t.Fatalf("SetEnrollToken: %v", err)
	}
	if ConsumeEnrollToken("000000") {
		t.Fatal("wrong code should not be accepted")
	}
	if !ConsumeEnrollToken(code) {
		t.Fatal("correct code should be accepted")
	}
	if ConsumeEnrollToken(code) {
		t.Fatal("code must be single-use")
	}
}

func TestEnrollTokenExpiry(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())
	code, err := SetEnrollToken(-1 * time.Second) // already expired
	if err != nil {
		t.Fatalf("SetEnrollToken: %v", err)
	}
	if ConsumeEnrollToken(code) {
		t.Fatal("expired code must be rejected")
	}
}
