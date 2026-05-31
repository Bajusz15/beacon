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

	// methodPassphrase / methodPasskey tag a challenge so an unlock is verified
	// with the matching factor.
	methodPassphrase = "passphrase"
	methodPasskey    = "passkey"

	// oobCodeDigits is the length of the out-of-band approval code.
	oobCodeDigits = 6
)

var (
	// ErrNotConfigured is returned when no remote-access factor is set.
	ErrNotConfigured = errors.New("remote-access not configured")
	// ErrLockedOut is returned while the device is backing off after repeated
	// verification failures.
	ErrLockedOut = errors.New("too many failed attempts; try again later")
	// ErrNoChallenge is returned when an unlock arrives with no matching
	// outstanding challenge (unknown/expired nonce).
	ErrNoChallenge = errors.New("no matching challenge (expired or unknown)")
	// ErrBadProof is returned when the supplied proof does not match.
	ErrBadProof = errors.New("invalid passphrase proof")
	// ErrBadAssertion is returned when no enrolled passkey verifies the assertion.
	ErrBadAssertion = errors.New("invalid passkey assertion")
	// ErrBadOOBCode is returned when the out-of-band approval code is wrong.
	ErrBadOOBCode = errors.New("invalid or missing out-of-band approval code")
	// ErrOOBDelivery is returned when the out-of-band code cannot be delivered.
	ErrOOBDelivery = errors.New("could not deliver out-of-band approval code")
)

// Notifier delivers the per-session out-of-band approval code through a channel
// the cloud does not mediate (the device's own alert channel). It is what closes
// the challenge-swap gap: the cloud can relay a ceremony but cannot obtain this
// code, so it cannot unilaterally complete an unlock.
type Notifier interface {
	SendOOBCode(action, code string) error
}

type pending struct {
	nonce   string
	action  string
	method  string
	oobCode string // "" when OOB delivery is not configured
	exp     time.Time
}

type grant struct {
	action string
	exp    time.Time
}

// pendingEnroll holds an in-flight passkey registration ceremony, keyed by
// session_id. rpID/origin are pinned here and re-checked against the browser's
// actual ceremony at finish, so a malicious relay cannot bind a credential to a
// different origin.
type pendingEnroll struct {
	challenge string // base64url
	rpID      string
	origin    string
	exp       time.Time
}

// ChallengeResult is the passphrase derivation material the agent hands back to
// the browser (via the cloud relay). None of it is secret.
type ChallengeResult struct {
	Nonce     string       `json:"nonce"`
	Salt      string       `json:"salt"`
	Params    Argon2Params `json:"params"`
	ExpiresAt time.Time    `json:"expires_at"`
}

// PasskeyAllowCredential is one entry in the WebAuthn allowCredentials list.
type PasskeyAllowCredential struct {
	ID   string `json:"id"`   // base64url credential id
	Type string `json:"type"` // "public-key"
}

// PasskeyChallengeResult is the WebAuthn request material handed to the browser.
// None of it is secret.
type PasskeyChallengeResult struct {
	Challenge        string                   `json:"challenge"` // base64url
	RPID             string                   `json:"rpId"`
	AllowCredentials []PasskeyAllowCredential `json:"allowCredentials"`
	UserVerification string                   `json:"userVerification"`
	ExpiresAt        time.Time                `json:"expires_at"`
}

// Grants is the in-memory, process-only source of truth for remote-access
// unlocks. It is never persisted: a restart clears all pending challenges and
// grants, so the device fails closed.
type Grants struct {
	mu       sync.Mutex
	pending  map[string]pending       // keyed by session_id
	grants   map[string]grant         // keyed by session_id
	enrolls  map[string]pendingEnroll // keyed by session_id
	grantTTL time.Duration
	notifier Notifier

	failures    int
	lockedUntil time.Time
}

// SetNotifier wires the out-of-band code delivery channel. When set, every
// challenge mints a code that must be supplied at unlock; when nil, OOB
// confirmation is disabled (the gate still works with the in-band factor).
func (g *Grants) SetNotifier(n Notifier) {
	g.mu.Lock()
	g.notifier = n
	g.mu.Unlock()
}

// NewGrants creates a grant manager with the default unlock TTL.
func NewGrants() *Grants {
	return &Grants{
		pending:  make(map[string]pending),
		grants:   make(map[string]grant),
		enrolls:  make(map[string]pendingEnroll),
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

// Challenge issues a fresh single-use passphrase nonce bound to (action,
// sessionID), delivers the out-of-band code (when configured), and returns the
// public derivation material for the browser.
func (g *Grants) Challenge(action, sessionID string) (*ChallengeResult, error) {
	cfg, err := Load()
	if err != nil || !cfg.HasPassphrase() {
		return nil, ErrNotConfigured
	}
	nonceBytes := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonceBytes); err != nil {
		return nil, err
	}
	nonce := base64.StdEncoding.EncodeToString(nonceBytes)
	code, err := g.deliverOOB(action)
	if err != nil {
		return nil, err
	}
	exp := time.Now().Add(nonceTTL)

	g.mu.Lock()
	g.pruneLocked()
	g.pending[sessionID] = pending{nonce: nonce, action: action, method: methodPassphrase, oobCode: code, exp: exp}
	g.mu.Unlock()

	return &ChallengeResult{
		Nonce:     nonce,
		Salt:      cfg.Salt,
		Params:    cfg.Params,
		ExpiresAt: exp,
	}, nil
}

// ChallengePasskey issues a fresh single-use WebAuthn challenge bound to (action,
// sessionID), delivers the out-of-band code (when configured), and returns the
// allowCredentials + rpId for the browser's navigator.credentials.get().
func (g *Grants) ChallengePasskey(action, sessionID string) (*PasskeyChallengeResult, error) {
	cfg, err := Load()
	if err != nil || !cfg.HasCredentials() {
		return nil, ErrNotConfigured
	}
	nonceBytes := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonceBytes); err != nil {
		return nil, err
	}
	// WebAuthn challenges are base64url (no padding); the browser echoes this
	// exact string in clientDataJSON.challenge.
	challenge := base64.RawURLEncoding.EncodeToString(nonceBytes)
	allow := make([]PasskeyAllowCredential, 0, len(cfg.Credentials))
	for _, c := range cfg.Credentials {
		allow = append(allow, PasskeyAllowCredential{ID: c.ID, Type: "public-key"})
	}
	rpID := cfg.Credentials[0].RPID
	code, err := g.deliverOOB(action)
	if err != nil {
		return nil, err
	}
	exp := time.Now().Add(nonceTTL)

	g.mu.Lock()
	g.pruneLocked()
	g.pending[sessionID] = pending{nonce: challenge, action: action, method: methodPasskey, oobCode: code, exp: exp}
	g.mu.Unlock()

	return &PasskeyChallengeResult{
		Challenge:        challenge,
		RPID:             rpID,
		AllowCredentials: allow,
		UserVerification: "required",
		ExpiresAt:        exp,
	}, nil
}

// deliverOOB mints and delivers the out-of-band approval code. It returns "" when
// no notifier is configured (OOB disabled), the code on success, or an error when
// a configured channel fails to deliver (the gate then stays closed).
func (g *Grants) deliverOOB(action string) (string, error) {
	g.mu.Lock()
	n := g.notifier
	g.mu.Unlock()
	if n == nil {
		return "", nil
	}
	code, err := genOOBCode(oobCodeDigits)
	if err != nil {
		return "", err
	}
	if err := n.SendOOBCode(action, code); err != nil {
		return "", ErrOOBDelivery
	}
	return code, nil
}

// checkOOBLocked verifies the supplied OOB code against the pending challenge.
// When the challenge has no code (OOB disabled), it always passes.
func checkOOBLocked(p pending, supplied string) bool {
	if p.oobCode == "" {
		return true
	}
	return constantTimeEqual([]byte(p.oobCode), []byte(supplied))
}

// Verify checks a browser-supplied passphrase proof and the out-of-band code
// against an outstanding challenge. On success it consumes the challenge and
// records a session-bound unlock. It applies exponential backoff after repeated
// failures.
func (g *Grants) Verify(action, sessionID, nonce, proof, oobCode string) error {
	cfg, err := Load()
	if err != nil || !cfg.HasPassphrase() {
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
	if !ok || p.method != methodPassphrase || time.Now().After(p.exp) || p.nonce != nonce || p.action != action {
		g.recordFailureLocked()
		return ErrNoChallenge
	}
	if !checkOOBLocked(p, oobCode) {
		g.recordFailureLocked()
		return ErrBadOOBCode
	}

	expected := computeProof(key, nonce, action, sessionID)
	supplied, err := base64.StdEncoding.DecodeString(proof)
	if err != nil || !constantTimeEqual(expected, supplied) {
		g.recordFailureLocked()
		return ErrBadProof
	}

	g.grantLocked(sessionID, action)
	return nil
}

// VerifyPasskey checks a browser-supplied WebAuthn assertion and the out-of-band
// code against an outstanding passkey challenge. The assertion is verified
// against each enrolled credential's pinned public key, rpId, and origin; on
// success the credential's signature counter is persisted and a session-bound
// unlock is recorded.
func (g *Grants) VerifyPasskey(action, sessionID, assertionJSON, oobCode string) error {
	cfg, err := Load()
	if err != nil || !cfg.HasCredentials() {
		return ErrNotConfigured
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.lockedUntil.IsZero() && time.Now().Before(g.lockedUntil) {
		return ErrLockedOut
	}

	p, ok := g.pending[sessionID]
	if !ok || p.method != methodPasskey || time.Now().After(p.exp) || p.action != action {
		g.recordFailureLocked()
		return ErrNoChallenge
	}
	if !checkOOBLocked(p, oobCode) {
		g.recordFailureLocked()
		return ErrBadOOBCode
	}

	for _, c := range cfg.Credentials {
		pub, decErr := base64.StdEncoding.DecodeString(c.PublicKey)
		if decErr != nil {
			continue
		}
		newCount, vErr := VerifyAssertion([]byte(assertionJSON), p.nonce, c.RPID, []string{c.Origin}, pub, c.SignCount)
		if vErr != nil {
			continue
		}
		// Persist the anti-clone counter best-effort; do not fail the unlock if
		// the write fails (the in-memory grant still gates this session).
		_ = UpdateSignCount(c.ID, newCount)
		g.grantLocked(sessionID, action)
		return nil
	}

	g.recordFailureLocked()
	return ErrBadAssertion
}

// grantLocked consumes the pending challenge and records a session-bound unlock,
// resetting backoff. Caller must hold g.mu.
func (g *Grants) grantLocked(sessionID, action string) {
	delete(g.pending, sessionID)
	g.grants[sessionID] = grant{action: action, exp: time.Now().Add(g.grantTTL)}
	g.failures = 0
	g.lockedUntil = time.Time{}
}

// BeginEnroll starts a passkey registration ceremony: it returns a fresh
// base64url challenge and records the pending enrollment bound to (rpID, origin).
// The caller must have already authorized enrollment (device-local token or a
// passphrase unlock).
func (g *Grants) BeginEnroll(sessionID, rpID, origin string) (string, error) {
	if sessionID == "" || rpID == "" || origin == "" {
		return "", errors.New("session_id, rp_id and origin are required")
	}
	nonceBytes := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonceBytes); err != nil {
		return "", err
	}
	challenge := base64.RawURLEncoding.EncodeToString(nonceBytes)
	g.mu.Lock()
	g.pruneEnrollLocked()
	g.enrolls[sessionID] = pendingEnroll{challenge: challenge, rpID: rpID, origin: origin, exp: time.Now().Add(nonceTTL)}
	g.mu.Unlock()
	return challenge, nil
}

// FinishEnroll verifies a registration response against the pending enrollment
// and stores the new credential. It consumes the pending enrollment.
func (g *Grants) FinishEnroll(sessionID, responseJSON, label string) error {
	g.mu.Lock()
	pe, ok := g.enrolls[sessionID]
	if ok {
		delete(g.enrolls, sessionID)
	}
	g.mu.Unlock()
	if !ok || time.Now().After(pe.exp) {
		return ErrNoChallenge
	}
	res, err := VerifyRegistration([]byte(responseJSON), pe.challenge, pe.rpID, []string{pe.origin})
	if err != nil {
		return err
	}
	return AddCredential(PasskeyCredential{
		ID:        base64.RawURLEncoding.EncodeToString(res.CredentialID),
		PublicKey: base64.StdEncoding.EncodeToString(res.PublicKey),
		RPID:      pe.rpID,
		Origin:    pe.origin,
		SignCount: res.SignCount,
		Label:     label,
	})
}

func (g *Grants) pruneEnrollLocked() {
	now := time.Now()
	for k, e := range g.enrolls {
		if now.After(e.exp) {
			delete(g.enrolls, k)
		}
	}
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

// genOOBCode returns a zero-padded numeric code of n digits using crypto/rand.
func genOOBCode(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = '0' + (buf[i] % 10)
	}
	return string(out), nil
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
