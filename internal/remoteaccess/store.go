// Package remoteaccess implements the device-verified remote-access passphrase:
// a second factor that is configured locally on the device and verified locally
// by the agent. BeaconInfra (the cloud) is only a relay — it never sees the
// passphrase or any reusable proof, so a fully compromised cloud still cannot
// open a remote terminal or tunnel without the locally-held secret.
//
// Setup stores an Argon2id hash of the passphrase on disk. At session time the
// agent issues a single-use nonce; the browser derives key = Argon2id(passphrase)
// and returns proof = HMAC-SHA256(key, nonce||action||session_id); the agent
// recomputes and compares the proof in constant time, then records an in-memory,
// session-bound, TTL-limited unlock that is cleared on restart (fail-closed).
package remoteaccess

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"beacon/internal/config"

	"golang.org/x/crypto/argon2"
)

const (
	storeDirMode  = 0700
	storeFileMode = 0600
	storeFileName = "remote-access.json"

	saltSize = 16
	keyLen   = 32
)

// Argon2Params are the Argon2id cost parameters. They are stored alongside the
// hash and sent to the browser in the challenge — they are NOT secret. Defaults
// are tuned to stay usable on low-power devices (Raspberry Pi) while remaining
// expensive enough to grind.
type Argon2Params struct {
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"` // in KiB
	Threads uint8  `json:"threads"`
	KeyLen  uint32 `json:"keyLen"`
}

// DefaultParams returns the default Argon2id parameters (64 MiB, 3 passes).
func DefaultParams() Argon2Params {
	return Argon2Params{Time: 3, Memory: 64 * 1024, Threads: 4, KeyLen: keyLen}
}

// Config is the on-disk passphrase material. The passphrase itself is never
// stored — only the derived Argon2id key, the salt, and the cost parameters.
type Config struct {
	Argon2id  string       `json:"argon2id"` // base64 of the derived key
	Salt      string       `json:"salt"`     // base64 of the salt
	Params    Argon2Params `json:"params"`
	UpdatedAt string       `json:"updated_at"`
}

// derivedKey returns the raw Argon2id key bytes stored in the config.
func (c *Config) derivedKey() ([]byte, error) {
	return base64.StdEncoding.DecodeString(c.Argon2id)
}

func storePath() (string, error) {
	base, err := config.BeaconHomeDir()
	if err != nil {
		return "", fmt.Errorf("beacon home: %w", err)
	}
	return filepath.Join(base, storeFileName), nil
}

// IsConfigured reports whether a remote-access passphrase has been set on this
// device. When false, remote-access gating is off and behavior is unchanged.
func IsConfigured() bool {
	path, err := storePath()
	if err != nil {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Load reads the on-disk passphrase config. It returns os.ErrNotExist when no
// passphrase is configured.
func Load() (*Config, error) {
	path, err := storePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse remote-access config: %w", err)
	}
	if c.Argon2id == "" || c.Salt == "" {
		return nil, fmt.Errorf("remote-access config is incomplete")
	}
	return &c, nil
}

// SetPassphrase derives an Argon2id key from the passphrase with a fresh salt
// and writes it to ~/.beacon/remote-access.json (mode 0600), replacing any
// existing passphrase.
func SetPassphrase(passphrase string) error {
	if len(passphrase) < 8 {
		return fmt.Errorf("passphrase must be at least 8 characters")
	}
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return fmt.Errorf("generate salt: %w", err)
	}
	params := DefaultParams()
	key := deriveKey(passphrase, salt, params)

	cfg := Config{
		Argon2id:  base64.StdEncoding.EncodeToString(key),
		Salt:      base64.StdEncoding.EncodeToString(salt),
		Params:    params,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(&cfg, "", "  ")
	if err != nil {
		return err
	}
	path, err := storePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), storeDirMode); err != nil {
		return err
	}
	return writeFileAtomic(path, data, storeFileMode)
}

// Clear removes the passphrase, disabling remote-access gating. This is the
// local recovery path for a forgotten passphrase (requires local shell access).
func Clear() error {
	path, err := storePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// deriveKey derives the Argon2id key from a passphrase. This is the agent-side
// equivalent of the derivation the browser performs; the two must stay in sync.
func deriveKey(passphrase string, salt []byte, p Argon2Params) []byte {
	return argon2.IDKey([]byte(passphrase), salt, p.Time, p.Memory, p.Threads, p.KeyLen)
}

// constantTimeEqual compares two byte slices in constant time.
func constantTimeEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// writeFileAtomic writes data to path via a temp file + rename, mirroring the
// durability pattern in internal/secrets.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	keepTmp := true
	defer func() {
		if keepTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	keepTmp = false
	return nil
}
