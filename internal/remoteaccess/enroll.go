package remoteaccess

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"beacon/internal/config"
)

// Enrollment is rooted in device-local access, exactly like setting a passphrase:
// `beacon remote-access add-passkey` writes a short-lived, single-use enrollment
// token to disk (only its hash is stored), and the running agent consumes it when
// the browser completes the WebAuthn registration ceremony. A malicious cloud
// cannot forge it because it never has device-local access.

const enrollFileName = "remote-access-enroll.json"

// DefaultEnrollTTL is how long an enrollment token is valid.
const DefaultEnrollTTL = 10 * time.Minute

type enrollToken struct {
	Hash      string `json:"hash"` // base64 sha256(code)
	ExpiresAt string `json:"expires_at"`
}

func enrollPath() (string, error) {
	base, err := config.BeaconHomeDir()
	if err != nil {
		return "", fmt.Errorf("beacon home: %w", err)
	}
	return filepath.Join(base, enrollFileName), nil
}

func hashCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// SetEnrollToken generates a one-time enrollment code, stores its hash + expiry
// on disk (mode 0600), and returns the plaintext code to show the operator.
func SetEnrollToken(ttl time.Duration) (string, error) {
	buf := make([]byte, 6)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", err
	}
	code := make([]byte, len(buf))
	for i := range buf {
		code[i] = '0' + (buf[i] % 10)
	}
	tok := enrollToken{
		Hash:      hashCode(string(code)),
		ExpiresAt: time.Now().Add(ttl).UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(&tok, "", "  ")
	if err != nil {
		return "", err
	}
	path, err := enrollPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), storeDirMode); err != nil {
		return "", err
	}
	if err := writeFileAtomic(path, data, storeFileMode); err != nil {
		return "", err
	}
	return string(code), nil
}

// ConsumeEnrollToken verifies the supplied code against the stored token and,
// on success, deletes it (single-use). It returns false for a missing, expired,
// or non-matching token.
func ConsumeEnrollToken(code string) bool {
	path, err := enrollPath()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var tok enrollToken
	if err := json.Unmarshal(data, &tok); err != nil {
		return false
	}
	exp, err := time.Parse(time.RFC3339, tok.ExpiresAt)
	if err != nil || time.Now().After(exp) {
		_ = os.Remove(path)
		return false
	}
	if !constantTimeEqual([]byte(tok.Hash), []byte(hashCode(code))) {
		return false
	}
	_ = os.Remove(path)
	return true
}

// ClearEnrollToken removes any pending enrollment token.
func ClearEnrollToken() error {
	path, err := enrollPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
