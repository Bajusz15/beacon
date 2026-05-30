package remoteaccess

import (
	"encoding/base64"
	"testing"
)

// TestCrossLanguageVector pins a deterministic proof for fixed inputs. The same
// value was independently computed by the browser implementation (hash-wasm
// Argon2id + HMAC-SHA256 in frontend/src/lib/remoteAccess.ts). If this ever
// changes, the browser and agent will silently disagree and every unlock fails.
func TestCrossLanguageVector(t *testing.T) {
	const (
		passphrase = "correct horse battery staple"
		nonce      = "test-nonce"
		action     = "terminal_open"
		sessionID  = "grant-123"
		// Pinned proof, confirmed equal across Go and hash-wasm.
		want = "fSr1yXGG/gzDukiV2pUlzzFeUF2+rZYZGY3p3w8A6GE="
	)
	// 16 fixed salt bytes (0x01..0x10).
	salt := make([]byte, 16)
	for i := range salt {
		salt[i] = byte(i + 1)
	}
	params := Argon2Params{Time: 3, Memory: 65536, Threads: 4, KeyLen: 32}

	key := deriveKey(passphrase, salt, params)
	proof := base64.StdEncoding.EncodeToString(computeProof(key, nonce, action, sessionID))
	if proof != want {
		t.Fatalf("proof mismatch: got %s want %s (browser/agent crypto diverged)", proof, want)
	}
}
