package remoteaccess

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

// softAuthenticator is a minimal in-process WebAuthn authenticator used to drive
// the agent verifier end-to-end with REAL ES256 signatures. It produces the same
// registration/assertion JSON a browser's navigator.credentials would, so the
// test exercises the full ceremony the cloud relays opaquely — nothing is stubbed
// on the verification path.
type softAuthenticator struct {
	key       *ecdsa.PrivateKey
	credID    []byte
	signCount uint32
}

func newSoftAuthenticator(t *testing.T) *softAuthenticator {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	credID := make([]byte, 16)
	if _, err := rand.Read(credID); err != nil {
		t.Fatalf("rand credID: %v", err)
	}
	return &softAuthenticator{key: key, credID: credID, signCount: 0}
}

// coseKey returns the CBOR-encoded COSE_Key (EC2/P-256/ES256) public key.
func (a *softAuthenticator) coseKey(t *testing.T) []byte {
	t.Helper()
	// Uncompressed point is 0x04 || X(32) || Y(32); take the coordinates from it
	// without touching the deprecated big.Int fields.
	ek, err := a.key.PublicKey.ECDH()
	if err != nil {
		t.Fatalf("ecdh public key: %v", err)
	}
	raw := ek.Bytes()
	x, y := raw[1:33], raw[33:65]
	// COSE_Key labels: 1=kty(2=EC2), 3=alg(-7=ES256), -1=crv(1=P-256), -2=x, -3=y.
	m := map[int]any{1: 2, 3: -7, -1: 1, -2: x, -3: y}
	b, err := cbor.Marshal(m)
	if err != nil {
		t.Fatalf("marshal cose key: %v", err)
	}
	return b
}

// authData builds the authenticator data structure. When attCred is non-nil the
// AT (attested credential data) flag is set and the block is appended.
func authData(t *testing.T, rpID string, uv bool, signCount uint32, attCred []byte) []byte {
	t.Helper()
	h := sha256.Sum256([]byte(rpID))
	var flags byte = 0x01 // UP (user present)
	if uv {
		flags |= 0x04 // UV (user verified)
	}
	if attCred != nil {
		flags |= 0x40 // AT (attested credential data present)
	}
	out := make([]byte, 0, 37+len(attCred))
	out = append(out, h[:]...)
	out = append(out, flags)
	var sc [4]byte
	binary.BigEndian.PutUint32(sc[:], signCount)
	out = append(out, sc[:]...)
	out = append(out, attCred...)
	return out
}

func clientDataJSON(t *testing.T, typ, challenge, origin string) []byte {
	t.Helper()
	cd := map[string]any{
		"type":        typ,
		"challenge":   challenge,
		"origin":      origin,
		"crossOrigin": false,
	}
	b, err := json.Marshal(cd)
	if err != nil {
		t.Fatalf("marshal clientData: %v", err)
	}
	return b
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// register produces the navigator.credentials.create() response JSON.
func (a *softAuthenticator) register(t *testing.T, challenge, rpID, origin string) []byte {
	t.Helper()
	cose := a.coseKey(t)
	// attestedCredentialData = aaguid(16) || credIdLen(2) || credId || COSEpubkey
	att := make([]byte, 0, 18+len(a.credID)+len(cose))
	att = append(att, make([]byte, 16)...) // zero AAGUID
	var idl [2]byte
	binary.BigEndian.PutUint16(idl[:], uint16(len(a.credID)))
	att = append(att, idl[:]...)
	att = append(att, a.credID...)
	att = append(att, cose...)

	ad := authData(t, rpID, true, a.signCount, att)
	attObj, err := cbor.Marshal(map[string]any{
		"fmt":      "none",
		"attStmt":  map[string]any{},
		"authData": ad,
	})
	if err != nil {
		t.Fatalf("marshal attestationObject: %v", err)
	}
	cd := clientDataJSON(t, "webauthn.create", challenge, origin)

	resp := map[string]any{
		"id":    b64url(a.credID),
		"rawId": b64url(a.credID),
		"type":  "public-key",
		"response": map[string]any{
			"attestationObject": b64url(attObj),
			"clientDataJSON":    b64url(cd),
		},
		"clientExtensionResults": map[string]any{},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal registration response: %v", err)
	}
	return b
}

// assert produces the navigator.credentials.get() response JSON, signing over
// authData || sha256(clientDataJSON) and bumping the signature counter.
func (a *softAuthenticator) assert(t *testing.T, challenge, origin string) []byte {
	t.Helper()
	a.signCount++
	ad := authData(t, e2eRPID, true, a.signCount, nil)
	cd := clientDataJSON(t, "webauthn.get", challenge, origin)
	cdHash := sha256.Sum256(cd)
	signed := append(append([]byte{}, ad...), cdHash[:]...)
	digest := sha256.Sum256(signed)
	sig, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	if err != nil {
		t.Fatalf("sign assertion: %v", err)
	}
	resp := map[string]any{
		"id":    b64url(a.credID),
		"rawId": b64url(a.credID),
		"type":  "public-key",
		"response": map[string]any{
			"authenticatorData": b64url(ad),
			"clientDataJSON":    b64url(cd),
			"signature":         b64url(sig),
		},
		"clientExtensionResults": map[string]any{},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal assertion response: %v", err)
	}
	return b
}

const (
	e2eRPID   = "beaconinfra.dev"
	e2eOrigin = "https://beaconinfra.dev"
)

// TestPasskeyE2E drives the entire passkey ceremony through the public agent API:
// enroll (real attestation) → challenge → assert (real signature) → OOB → grant →
// single-use consume. It then covers the security-relevant negatives.
func TestPasskeyE2E(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())
	auth := newSoftAuthenticator(t)

	g := NewGrants()
	note := &recordingNotifier{}
	g.SetNotifier(note)

	// --- Enrollment (registration ceremony) ---------------------------------
	const enrollSess = "enroll-1"
	challenge, err := g.BeginEnroll(enrollSess, e2eRPID, e2eOrigin)
	if err != nil {
		t.Fatalf("BeginEnroll: %v", err)
	}
	regJSON := auth.register(t, challenge, e2eRPID, e2eOrigin)
	if err := g.FinishEnroll(enrollSess, string(regJSON), "Test Touch ID"); err != nil {
		t.Fatalf("FinishEnroll: %v", err)
	}
	if !IsConfigured() {
		t.Fatal("expected gate configured after enrollment")
	}
	creds, _ := ListCredentials()
	if len(creds) != 1 || creds[0].RPID != e2eRPID {
		t.Fatalf("expected one enrolled credential pinned to rpID, got %+v", creds)
	}

	// --- Unlock (assertion ceremony) ---------------------------------------
	const sess = "sess-A"
	ch, err := g.ChallengePasskey("terminal_open", sess)
	if err != nil {
		t.Fatalf("ChallengePasskey: %v", err)
	}
	if note.code == "" {
		t.Fatal("expected an OOB code delivered on challenge")
	}
	if len(ch.AllowCredentials) != 1 || ch.UserVerification != "required" {
		t.Fatalf("unexpected challenge material: %+v", ch)
	}

	asn := auth.assert(t, ch.Challenge, e2eOrigin)

	// Wrong OOB code is rejected (and consumes an attempt).
	if err := g.VerifyPasskey("terminal_open", sess, string(asn), "000000"); err != ErrBadOOBCode {
		t.Fatalf("expected ErrBadOOBCode, got %v", err)
	}

	// Correct ceremony with the delivered OOB code unlocks (fresh challenge).
	ch2, _ := g.ChallengePasskey("terminal_open", sess)
	asn2 := auth.assert(t, ch2.Challenge, e2eOrigin)
	if err := g.VerifyPasskey("terminal_open", sess, string(asn2), note.code); err != nil {
		t.Fatalf("expected unlock, got %v", err)
	}
	if !g.Consume("terminal_open", sess) {
		t.Fatal("expected a consumable single-use grant")
	}
	if g.Consume("terminal_open", sess) {
		t.Fatal("grant must be single-use")
	}

	// --- Negative: wrong origin is rejected --------------------------------
	const sessBad = "sess-B"
	chB, _ := g.ChallengePasskey("terminal_open", sessBad)
	evil := auth.assert(t, chB.Challenge, "https://evil.example")
	if err := g.VerifyPasskey("terminal_open", sessBad, string(evil), note.code); err != ErrBadAssertion {
		t.Fatalf("expected ErrBadAssertion for wrong origin, got %v", err)
	}

	// --- Negative: replayed (stale) assertion is rejected by sign counter ---
	// Persisted counter is now ahead; re-presenting an assertion at an equal or
	// lower counter must fail. Forge one at a counter we know is stale.
	const sessC = "sess-C"
	chC, _ := g.ChallengePasskey("terminal_open", sessC)
	stale := newStaleAssertion(t, auth, chC.Challenge)
	if err := g.VerifyPasskey("terminal_open", sessC, string(stale), note.code); err != ErrBadAssertion {
		t.Fatalf("expected ErrBadAssertion for non-monotonic sign count, got %v", err)
	}
}

// newStaleAssertion produces a correctly-signed assertion at signCount=1, which is
// at or below the counter already persisted after earlier successful asserts.
func newStaleAssertion(t *testing.T, a *softAuthenticator, challenge string) []byte {
	t.Helper()
	clone := &softAuthenticator{key: a.key, credID: a.credID, signCount: 0}
	return clone.assert(t, challenge, e2eOrigin) // bumps to 1
}
