package remoteaccess

import (
	"encoding/base64"
	"testing"
	"time"
)

// setupConfigured points BEACON_HOME at a temp dir and sets a passphrase.
func setupConfigured(t *testing.T, passphrase string) {
	t.Helper()
	t.Setenv("BEACON_HOME", t.TempDir())
	if err := SetPassphrase(passphrase); err != nil {
		t.Fatalf("SetPassphrase: %v", err)
	}
}

// browserProof mimics what the browser computes from the challenge material.
func browserProof(t *testing.T, passphrase string, ch *ChallengeResult, action, sessionID string) string {
	t.Helper()
	salt, err := base64.StdEncoding.DecodeString(ch.Salt)
	if err != nil {
		t.Fatalf("decode salt: %v", err)
	}
	key := deriveKey(passphrase, salt, ch.Params)
	return base64.StdEncoding.EncodeToString(computeProof(key, ch.Nonce, action, sessionID))
}

func TestHappyPathUnlockConsumeOnce(t *testing.T) {
	const pp = "correct horse battery"
	setupConfigured(t, pp)
	g := NewGrants()

	ch, err := g.Challenge("terminal_open", "sess-1")
	if err != nil {
		t.Fatalf("Challenge: %v", err)
	}
	proof := browserProof(t, pp, ch, "terminal_open", "sess-1")
	if err := g.Verify("terminal_open", "sess-1", ch.Nonce, proof, ""); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !g.Consume("terminal_open", "sess-1") {
		t.Fatal("expected first Consume to succeed")
	}
	if g.Consume("terminal_open", "sess-1") {
		t.Fatal("expected second Consume to fail (single-use)")
	}
}

func TestWrongProofRejected(t *testing.T) {
	const pp = "right passphrase"
	setupConfigured(t, pp)
	g := NewGrants()

	ch, err := g.Challenge("terminal_open", "sess-1")
	if err != nil {
		t.Fatalf("Challenge: %v", err)
	}
	bad := browserProof(t, "wrong passphrase", ch, "terminal_open", "sess-1")
	if err := g.Verify("terminal_open", "sess-1", ch.Nonce, bad, ""); err != ErrBadProof {
		t.Fatalf("expected ErrBadProof, got %v", err)
	}
	if g.Consume("terminal_open", "sess-1") {
		t.Fatal("expected no grant after bad proof")
	}
}

func TestProofIsSessionBound(t *testing.T) {
	const pp = "session binding test"
	setupConfigured(t, pp)
	g := NewGrants()

	ch, _ := g.Challenge("terminal_open", "sess-A")
	proof := browserProof(t, pp, ch, "terminal_open", "sess-A")
	// A compromised cloud cannot reuse the captured proof for a different session.
	if err := g.Verify("terminal_open", "sess-B", ch.Nonce, proof, ""); err == nil {
		t.Fatal("expected verification to fail for a different session id")
	}
}

func TestProofIsActionBound(t *testing.T) {
	const pp = "action binding test"
	setupConfigured(t, pp)
	g := NewGrants()

	// A challenge issued for tunnel_connect must not be satisfiable by a proof
	// computed for terminal_open on the same session.
	ch, _ := g.Challenge("tunnel_connect", "sess-1")
	proof := browserProof(t, pp, ch, "terminal_open", "sess-1")
	if err := g.Verify("tunnel_connect", "sess-1", ch.Nonce, proof, ""); err != ErrBadProof {
		t.Fatalf("expected ErrBadProof for action-mismatched proof, got %v", err)
	}

	// The correctly-bound proof unlocks.
	ch2, _ := g.Challenge("tunnel_connect", "sess-2")
	good := browserProof(t, pp, ch2, "tunnel_connect", "sess-2")
	if err := g.Verify("tunnel_connect", "sess-2", ch2.Nonce, good, ""); err != nil {
		t.Fatalf("Verify (tunnel_connect): %v", err)
	}
	if !g.Consume("tunnel_connect", "sess-2") {
		t.Fatal("expected tunnel_connect grant to be consumable")
	}
}

func TestBackoffAfterRepeatedFailures(t *testing.T) {
	const pp = "backoff test"
	setupConfigured(t, pp)
	g := NewGrants()

	for i := 0; i < maxFreeFailures; i++ {
		ch, _ := g.Challenge("terminal_open", "sess-1")
		_ = g.Verify("terminal_open", "sess-1", ch.Nonce, "AAAA", "")
	}
	ch, _ := g.Challenge("terminal_open", "sess-1")
	if err := g.Verify("terminal_open", "sess-1", ch.Nonce, "AAAA", ""); err != ErrLockedOut {
		t.Fatalf("expected ErrLockedOut after %d failures, got %v", maxFreeFailures, err)
	}
}

func TestExpiredNonceRejected(t *testing.T) {
	const pp = "expiry test"
	setupConfigured(t, pp)
	g := NewGrants()

	ch, _ := g.Challenge("terminal_open", "sess-1")
	// Force the pending challenge to be expired.
	g.mu.Lock()
	p := g.pending["sess-1"]
	p.exp = time.Now().Add(-time.Second)
	g.pending["sess-1"] = p
	g.mu.Unlock()

	proof := browserProof(t, pp, ch, "terminal_open", "sess-1")
	if err := g.Verify("terminal_open", "sess-1", ch.Nonce, proof, ""); err != ErrNoChallenge {
		t.Fatalf("expected ErrNoChallenge for expired nonce, got %v", err)
	}
}

func TestNotConfigured(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())
	g := NewGrants()
	if g.IsConfigured() {
		t.Fatal("expected not configured")
	}
	if _, err := g.Challenge("terminal_open", "sess-1"); err != ErrNotConfigured {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestGrantTTLClampActiveGrantAndExpiry(t *testing.T) {
	g := NewGrants()
	g.SetTTL(time.Second)
	if g.grantTTL != time.Minute {
		t.Fatalf("short TTL should clamp to one minute, got %s", g.grantTTL)
	}
	g.SetTTL(2 * time.Hour)
	if g.grantTTL != time.Hour {
		t.Fatalf("long TTL should clamp to one hour, got %s", g.grantTTL)
	}

	g.mu.Lock()
	g.grants["soon"] = grant{action: "terminal_open", exp: time.Now().Add(2 * time.Minute)}
	g.grants["later"] = grant{action: "terminal_open", exp: time.Now().Add(5 * time.Minute)}
	g.grants["expired"] = grant{action: "terminal_open", exp: time.Now().Add(-time.Minute)}
	g.mu.Unlock()

	exp, ok := g.ActiveGrant()
	if !ok {
		t.Fatal("expected active grant")
	}
	if time.Until(exp) > 3*time.Minute {
		t.Fatalf("expected soonest active grant, got %s", exp)
	}
	if g.Consume("other_action", "soon") {
		t.Fatal("wrong action should not consume successfully")
	}
	if g.Consume("terminal_open", "expired") {
		t.Fatal("expired grant should not consume")
	}
}

func TestChallengePasskeyAllowListAndOOBGate(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())
	if err := AddCredential(PasskeyCredential{ID: "cred-1", PublicKey: "cHVibGljLWtleQ==", RPID: "beaconinfra.dev", Origin: "https://beaconinfra.dev"}); err != nil {
		t.Fatalf("AddCredential: %v", err)
	}
	enrollTestOOB(t)
	g := NewGrants()

	ch, err := g.ChallengePasskey("terminal_open", "sess-1")
	if err != nil {
		t.Fatalf("ChallengePasskey: %v", err)
	}
	if ch.RPID != "beaconinfra.dev" || len(ch.AllowCredentials) != 1 || ch.AllowCredentials[0].ID != "cred-1" {
		t.Fatalf("unexpected challenge: %+v", ch)
	}
	// A wrong OOB code is rejected before the assertion is even parsed.
	if err := g.VerifyPasskey("terminal_open", "sess-1", "not-json", "000000"); err != ErrBadOOBCode {
		t.Fatalf("expected OOB failure before assertion verification, got %v", err)
	}
}

func TestBeginEnrollValidationAndFinishErrors(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())
	g := NewGrants()

	if _, err := g.BeginEnroll("", "beaconinfra.dev", "https://beaconinfra.dev"); err == nil {
		t.Fatal("expected missing session to fail")
	}
	if err := g.FinishEnroll("missing", "{}", "label"); err != ErrNoChallenge {
		t.Fatalf("expected missing enrollment challenge, got %v", err)
	}

	ch, err := g.BeginEnroll("sess-1", "beaconinfra.dev", "https://beaconinfra.dev")
	if err != nil {
		t.Fatalf("BeginEnroll: %v", err)
	}
	if ch == "" {
		t.Fatal("expected challenge")
	}
	if err := g.FinishEnroll("sess-1", "not-json", "label"); err == nil {
		t.Fatal("expected invalid registration response to fail")
	}
	if err := g.FinishEnroll("sess-1", "not-json", "label"); err != ErrNoChallenge {
		t.Fatalf("enrollment should be single-use after finish attempt, got %v", err)
	}
}

func TestPruneRemovesExpiredPendingEnrollsAndGrants(t *testing.T) {
	g := NewGrants()
	g.mu.Lock()
	g.pending["old"] = pending{exp: time.Now().Add(-time.Second)}
	g.pending["fresh"] = pending{exp: time.Now().Add(time.Minute)}
	g.grants["old"] = grant{exp: time.Now().Add(-time.Second)}
	g.grants["fresh"] = grant{exp: time.Now().Add(time.Minute)}
	g.enrolls["old"] = pendingEnroll{exp: time.Now().Add(-time.Second)}
	g.enrolls["fresh"] = pendingEnroll{exp: time.Now().Add(time.Minute)}
	g.pruneLocked()
	g.pruneEnrollLocked()
	_, oldPending := g.pending["old"]
	_, freshPending := g.pending["fresh"]
	_, oldGrant := g.grants["old"]
	_, freshGrant := g.grants["fresh"]
	_, oldEnroll := g.enrolls["old"]
	_, freshEnroll := g.enrolls["fresh"]
	g.mu.Unlock()

	if oldPending || oldGrant || oldEnroll {
		t.Fatal("expected expired state to be pruned")
	}
	if !freshPending || !freshGrant || !freshEnroll {
		t.Fatal("expected fresh state to be retained")
	}
}
