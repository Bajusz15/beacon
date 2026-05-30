package remoteaccess

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"sync"
	"time"
)

const (
	nonceSize = 32
	// nonceTTL is how long a challenge nonce is valid for the browser to return
	// a proof. Short, because the round trip is interactive.
	nonceTTL = 120 * time.Second
	// defaultGrantTTL is the default lifetime of a successful unlock.
	defaultGrantTTL = 10 * time.Minute

	maxFreeFailures = 5
	maxBackoff      = 5 * time.Minute
)

var (
	// ErrNotConfigured is returned when no passphrase is set on the device.
	ErrNotConfigured = errors.New("remote-access passphrase not configured")
	// ErrLockedOut is returned while the device is backing off after repeated
	// verification failures.
	ErrLockedOut = errors.New("too many failed attempts; try again later")
	// ErrNoChallenge is returned when an unlock arrives with no matching
	// outstanding challenge (unknown/expired nonce).
	ErrNoChallenge = errors.New("no matching challenge (expired or unknown)")
	// ErrBadProof is returned when the supplied proof does not match.
	ErrBadProof = errors.New("invalid passphrase proof")
)

type pending struct {
	nonce  string
	action string
	exp    time.Time
}

type grant struct {
	action string
	exp    time.Time
}

// ChallengeResult is the material the agent hands back to the browser (via the
// cloud relay) so it can derive the key and compute the proof. None of it is
// secret.
type ChallengeResult struct {
	Nonce     string       `json:"nonce"`
	Salt      string       `json:"salt"`
	Params    Argon2Params `json:"params"`
	ExpiresAt time.Time    `json:"expires_at"`
}

// Grants is the in-memory, process-only source of truth for remote-access
// unlocks. It is never persisted: a restart clears all pending challenges and
// grants, so the device fails closed.
type Grants struct {
	mu       sync.Mutex
	pending  map[string]pending // keyed by session_id
	grants   map[string]grant   // keyed by session_id
	grantTTL time.Duration

	failures    int
	lockedUntil time.Time
}

// NewGrants creates a grant manager with the default unlock TTL.
func NewGrants() *Grants {
	return &Grants{
		pending:  make(map[string]pending),
		grants:   make(map[string]grant),
		grantTTL: defaultGrantTTL,
	}
}

// SetTTL overrides the unlock lifetime (clamped to a sane range).
func (g *Grants) SetTTL(d time.Duration) {
	if d < time.Minute {
		d = time.Minute
	}
	if d > time.Hour {
		d = time.Hour
	}
	g.mu.Lock()
	g.grantTTL = d
	g.mu.Unlock()
}

// IsConfigured reports whether a passphrase is set (delegates to on-disk state).
func (g *Grants) IsConfigured() bool { return IsConfigured() }

// Challenge issues a fresh single-use nonce bound to (action, sessionID) and
// returns the public derivation material for the browser.
func (g *Grants) Challenge(action, sessionID string) (*ChallengeResult, error) {
	cfg, err := Load()
	if err != nil {
		return nil, ErrNotConfigured
	}
	nonceBytes := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonceBytes); err != nil {
		return nil, err
	}
	nonce := base64.StdEncoding.EncodeToString(nonceBytes)
	exp := time.Now().Add(nonceTTL)

	g.mu.Lock()
	g.pruneLocked()
	g.pending[sessionID] = pending{nonce: nonce, action: action, exp: exp}
	g.mu.Unlock()

	return &ChallengeResult{
		Nonce:     nonce,
		Salt:      cfg.Salt,
		Params:    cfg.Params,
		ExpiresAt: exp,
	}, nil
}

// Verify checks a browser-supplied proof against an outstanding challenge. On
// success it consumes the challenge and records a session-bound unlock. It
// applies exponential backoff after repeated failures.
func (g *Grants) Verify(action, sessionID, nonce, proof string) error {
	cfg, err := Load()
	if err != nil {
		return ErrNotConfigured
	}
	key, err := cfg.derivedKey()
	if err != nil {
		return ErrNotConfigured
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.lockedUntil.IsZero() && time.Now().Before(g.lockedUntil) {
		return ErrLockedOut
	}

	p, ok := g.pending[sessionID]
	if !ok || time.Now().After(p.exp) || p.nonce != nonce || p.action != action {
		g.recordFailureLocked()
		return ErrNoChallenge
	}

	expected := computeProof(key, nonce, action, sessionID)
	supplied, err := base64.StdEncoding.DecodeString(proof)
	if err != nil || !constantTimeEqual(expected, supplied) {
		g.recordFailureLocked()
		return ErrBadProof
	}

	// Success: consume the challenge, record the unlock, reset backoff.
	delete(g.pending, sessionID)
	g.grants[sessionID] = grant{action: action, exp: time.Now().Add(g.grantTTL)}
	g.failures = 0
	g.lockedUntil = time.Time{}
	return nil
}

// Consume reports whether a valid, unexpired, session-bound unlock exists for
// (action, sessionID) and removes it (single-use). The dispatcher calls this
// immediately before honoring a gated command.
func (g *Grants) Consume(action, sessionID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	gr, ok := g.grants[sessionID]
	if !ok {
		return false
	}
	delete(g.grants, sessionID)
	if gr.action != action || time.Now().After(gr.exp) {
		return false
	}
	return true
}

// ActiveGrant returns the expiry of the soonest-expiring active grant, for
// status display. ok is false when there are no active grants.
func (g *Grants) ActiveGrant() (exp time.Time, ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	for _, gr := range g.grants {
		if now.Before(gr.exp) {
			if !ok || gr.exp.Before(exp) {
				exp, ok = gr.exp, true
			}
		}
	}
	return exp, ok
}

func (g *Grants) recordFailureLocked() {
	g.failures++
	if g.failures >= maxFreeFailures {
		// Exponential backoff once past the free allowance.
		backoff := time.Duration(1<<uint(g.failures-maxFreeFailures)) * time.Second
		if backoff > maxBackoff || backoff <= 0 {
			backoff = maxBackoff
		}
		g.lockedUntil = time.Now().Add(backoff)
	}
}

func (g *Grants) pruneLocked() {
	now := time.Now()
	for k, p := range g.pending {
		if now.After(p.exp) {
			delete(g.pending, k)
		}
	}
	for k, gr := range g.grants {
		if now.After(gr.exp) {
			delete(g.grants, k)
		}
	}
}

// computeProof builds proof = HMAC-SHA256(key, nonce \0 action \0 session_id).
// The byte layout MUST match the browser's derivation exactly.
func computeProof(key []byte, nonce, action, sessionID string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(nonce))
	mac.Write([]byte{0})
	mac.Write([]byte(action))
	mac.Write([]byte{0})
	mac.Write([]byte(sessionID))
	return mac.Sum(nil)
}
