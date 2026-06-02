package remoteaccess

import "testing"

// SetOOB requires a primary factor first (it is auxiliary hardening), then
// round-trips, coexists with the primary factors, and clears independently.
func TestOOBStoreLifecycle(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())

	// With nothing configured, enrolling OOB is rejected — there is nothing for it
	// to gate.
	if err := SetOOB("JBSWY3DPEHPK3PXP"); err == nil {
		t.Fatal("expected SetOOB to fail with no primary factor")
	}

	if err := SetPassphrase("correct horse battery"); err != nil {
		t.Fatalf("SetPassphrase: %v", err)
	}

	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	if err := SetOOB(secret); err != nil {
		t.Fatalf("SetOOB: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.HasOOB() || cfg.OOB.Secret != secret {
		t.Fatalf("expected OOB secret to round-trip, got %+v", cfg.OOB)
	}
	if !cfg.HasPassphrase() {
		t.Fatal("SetOOB must preserve the passphrase factor")
	}

	// Adding a passkey keeps the OOB secret intact.
	if err := AddCredential(PasskeyCredential{ID: "cred-1", PublicKey: "cHVibGljLWtleQ=="}); err != nil {
		t.Fatalf("AddCredential: %v", err)
	}
	cfg, _ = Load()
	if !cfg.HasOOB() || !cfg.HasCredentials() {
		t.Fatalf("expected OOB + passkey to coexist, got %+v", cfg)
	}

	// Clearing OOB leaves the primary factors untouched.
	if err := ClearOOB(); err != nil {
		t.Fatalf("ClearOOB: %v", err)
	}
	cfg, _ = Load()
	if cfg.HasOOB() {
		t.Fatal("expected OOB cleared")
	}
	if !cfg.HasPassphrase() || !cfg.HasCredentials() {
		t.Fatalf("ClearOOB must keep primary factors, got %+v", cfg)
	}

	// ClearOOB is a no-op when nothing is enrolled.
	if err := ClearOOB(); err != nil {
		t.Fatalf("ClearOOB (already empty): %v", err)
	}
}

// SetOOB rejects an empty secret.
func TestSetOOBEmptySecret(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())
	if err := SetPassphrase("correct horse battery"); err != nil {
		t.Fatalf("SetPassphrase: %v", err)
	}
	if err := SetOOB("   "); err == nil {
		t.Fatal("expected SetOOB to reject an empty secret")
	}
}
