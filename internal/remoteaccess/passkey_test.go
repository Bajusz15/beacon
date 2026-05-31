package remoteaccess

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestStoreCredentialUpdateAndRemovalEdges(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())

	if err := AddCredential(PasskeyCredential{}); err == nil {
		t.Fatal("expected empty credential to be rejected")
	}

	cred := PasskeyCredential{ID: "cred-1", PublicKey: "cHVibGljLWtleQ==", RPID: "beaconinfra.dev", Origin: "https://beaconinfra.dev", Label: "first"}
	if err := AddCredential(cred); err != nil {
		t.Fatalf("AddCredential: %v", err)
	}
	cred.Label = "replacement"
	if err := AddCredential(cred); err != nil {
		t.Fatalf("replace credential: %v", err)
	}
	creds, err := ListCredentials()
	if err != nil {
		t.Fatalf("ListCredentials: %v", err)
	}
	if len(creds) != 1 || creds[0].Label != "replacement" {
		t.Fatalf("expected replacement credential, got %+v", creds)
	}

	if err := UpdateSignCount("cred-1", 42); err != nil {
		t.Fatalf("UpdateSignCount: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Credentials[0].SignCount != 42 {
		t.Fatalf("expected sign count update, got %+v", cfg.Credentials[0])
	}

	if err := UpdateSignCount("missing", 99); err != nil {
		t.Fatalf("missing credential update should be ignored: %v", err)
	}
	if err := RemoveCredential("missing"); err == nil {
		t.Fatal("expected missing credential removal to fail")
	}
	if err := RemoveCredential("replacement"); err != nil {
		t.Fatalf("remove by label: %v", err)
	}
	if IsConfigured() {
		t.Fatal("expected last credential removal to delete store")
	}
}

func TestLoadOrNewTreatsEmptyConfigAsEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BEACON_HOME", home)
	path := filepath.Join(home, storeFileName)
	if err := os.WriteFile(path, []byte(`{"updated_at":"2026-01-01T00:00:00Z"}`), 0600); err != nil {
		t.Fatalf("write empty config: %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("expected empty config to fail Load")
	}
	if err := AddCredential(PasskeyCredential{ID: "cred-1", PublicKey: "cHVibGljLWtleQ=="}); err != nil {
		t.Fatalf("AddCredential should repopulate empty config: %v", err)
	}
}

func TestEnrollTokenInvalidAndClear(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BEACON_HOME", home)

	if ConsumeEnrollToken("123456") {
		t.Fatal("missing token should be rejected")
	}

	path, err := enrollPath()
	if err != nil {
		t.Fatalf("enrollPath: %v", err)
	}
	if err := os.WriteFile(path, []byte(`not-json`), 0600); err != nil {
		t.Fatalf("write invalid token: %v", err)
	}
	if ConsumeEnrollToken("123456") {
		t.Fatal("invalid token should be rejected")
	}

	tok := enrollToken{Hash: hashCode("123456"), ExpiresAt: "not-a-time"}
	data, err := json.Marshal(tok)
	if err != nil {
		t.Fatalf("marshal token: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write bad expiry token: %v", err)
	}
	if ConsumeEnrollToken("123456") {
		t.Fatal("bad expiry token should be rejected")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expired malformed token should be removed, stat err=%v", err)
	}

	code, err := SetEnrollToken(DefaultEnrollTTL)
	if err != nil {
		t.Fatalf("SetEnrollToken: %v", err)
	}
	if code == "" {
		t.Fatal("expected non-empty code")
	}
	if err := ClearEnrollToken(); err != nil {
		t.Fatalf("ClearEnrollToken: %v", err)
	}
	if ConsumeEnrollToken(code) {
		t.Fatal("cleared token should not be accepted")
	}
	if err := ClearEnrollToken(); err != nil {
		t.Fatalf("ClearEnrollToken should ignore missing token: %v", err)
	}
}

func TestClearRemovesStore(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())
	if err := SetPassphrase("correct horse battery"); err != nil {
		t.Fatalf("SetPassphrase: %v", err)
	}
	if !IsConfigured() {
		t.Fatal("expected configured store")
	}
	if err := Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if IsConfigured() {
		t.Fatal("expected Clear to remove store")
	}
	if err := Clear(); err != nil {
		t.Fatalf("Clear should ignore missing store: %v", err)
	}
}
