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
	"strings"
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

// Config is the on-disk remote-access material. It can hold a passphrase factor
// (the derived Argon2id key + salt + params — the passphrase itself is never
// stored) and/or one or more passkey credentials (only the PUBLIC key is stored,
// which is strictly safer than a brute-forceable hash). Either factor satisfies
// the gate; passkeys are preferred and passphrase is the fallback/recovery path.
type Config struct {
	// Passphrase fallback (optional). Empty when no passphrase is set.
	Argon2id string       `json:"argon2id,omitempty"` // base64 of the derived key
	Salt     string       `json:"salt,omitempty"`     // base64 of the salt
	Params   Argon2Params `json:"params,omitempty"`
	// Passkeys (preferred). Empty when none are enrolled.
	Credentials []PasskeyCredential `json:"credentials,omitempty"`
	// OOB selects how the per-session out-of-band approval code is delivered.
	// When nil, delivery falls back to the device's configured alert channel.
	OOB       *OOBConfig `json:"oob,omitempty"`
	UpdatedAt string     `json:"updated_at"`
}

// PasskeyCredential is a stored WebAuthn credential. Only public material is
// kept: the agent verifies assertions against PublicKey and pins RPID/Origin to
// the values captured at enrollment so a malicious relay cannot present an
// assertion from a different origin.
type PasskeyCredential struct {
	ID        string `json:"id"`         // credential ID, base64url (no padding)
	PublicKey string `json:"public_key"` // COSE public key, base64 std
	RPID      string `json:"rp_id"`      // e.g. "beaconinfra.dev"
	Origin    string `json:"origin"`     // e.g. "https://beaconinfra.dev"
	SignCount uint32 `json:"sign_count"` // anti-clone counter, monotonic
	Label     string `json:"label,omitempty"`
	AddedAt   string `json:"added_at"`
}

// OOBConfig selects the out-of-band delivery channel for approval codes. Channel
// is one of "webhook" or "email"; Target is the channel-specific destination
// (webhook URL or email address). When empty the device alert channel is used.
type OOBConfig struct {
	Channel string `json:"channel,omitempty"`
	Target  string `json:"target,omitempty"`
}

// HasPassphrase reports whether a passphrase fallback is configured.
func (c *Config) HasPassphrase() bool { return c.Argon2id != "" && c.Salt != "" }

// HasCredentials reports whether at least one passkey is enrolled.
func (c *Config) HasCredentials() bool { return len(c.Credentials) > 0 }

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
	if !c.HasPassphrase() && !c.HasCredentials() {
		return nil, fmt.Errorf("remote-access config has no passphrase or passkey")
	}
	return &c, nil
}

// loadOrNew returns the existing config, or an empty one when none is configured.
func loadOrNew() (*Config, error) {
	c, err := Load()
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		// A config that exists but has neither factor is treated as empty so
		// callers can repopulate it rather than failing.
		if strings.Contains(err.Error(), "no passphrase or passkey") {
			return &Config{}, nil
		}
		return nil, err
	}
	return c, nil
}

// save writes the config atomically, or removes the file when neither factor
// remains so the gate is fully disabled (IsConfigured -> false).
func save(c *Config) error {
	path, err := storePath()
	if err != nil {
		return err
	}
	if !c.HasPassphrase() && !c.HasCredentials() {
		if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
			return rmErr
		}
		return nil
	}
	c.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), storeDirMode); err != nil {
		return err
	}
	return writeFileAtomic(path, data, storeFileMode)
}

// SetPassphrase derives an Argon2id key from the passphrase with a fresh salt
// and stores it, preserving any enrolled passkeys.
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

	cfg, err := loadOrNew()
	if err != nil {
		return err
	}
	cfg.Argon2id = base64.StdEncoding.EncodeToString(key)
	cfg.Salt = base64.StdEncoding.EncodeToString(salt)
	cfg.Params = params
	return save(cfg)
}

// ClearPassphrase removes only the passphrase factor, leaving passkeys intact.
// When no passkey remains, the gate is fully disabled (the file is removed).
func ClearPassphrase() error {
	cfg, err := loadOrNew()
	if err != nil {
		return err
	}
	cfg.Argon2id = ""
	cfg.Salt = ""
	cfg.Params = Argon2Params{}
	return save(cfg)
}

// AddCredential enrolls a passkey, preserving the passphrase and other passkeys.
// A credential with the same ID replaces the existing one.
func AddCredential(cred PasskeyCredential) error {
	if cred.ID == "" || cred.PublicKey == "" {
		return fmt.Errorf("credential id and public key are required")
	}
	cfg, err := loadOrNew()
	if err != nil {
		return err
	}
	cred.AddedAt = time.Now().UTC().Format(time.RFC3339)
	replaced := false
	for i := range cfg.Credentials {
		if cfg.Credentials[i].ID == cred.ID {
			cfg.Credentials[i] = cred
			replaced = true
			break
		}
	}
	if !replaced {
		cfg.Credentials = append(cfg.Credentials, cred)
	}
	return save(cfg)
}

// ListCredentials returns the enrolled passkeys (empty when none).
func ListCredentials() ([]PasskeyCredential, error) {
	cfg, err := loadOrNew()
	if err != nil {
		return nil, err
	}
	return cfg.Credentials, nil
}

// RemoveCredential deletes a passkey by credential ID or label. When the last
// factor is removed the gate is disabled.
func RemoveCredential(idOrLabel string) error {
	cfg, err := loadOrNew()
	if err != nil {
		return err
	}
	kept := cfg.Credentials[:0]
	removed := false
	for _, c := range cfg.Credentials {
		if c.ID == idOrLabel || (c.Label != "" && c.Label == idOrLabel) {
			removed = true
			continue
		}
		kept = append(kept, c)
	}
	if !removed {
		return fmt.Errorf("no passkey matching %q", idOrLabel)
	}
	cfg.Credentials = kept
	return save(cfg)
}

// UpdateSignCount persists the post-verification signature counter for a
// credential (anti-clone). It is best-effort and preserves all other state.
func UpdateSignCount(credID string, signCount uint32) error {
	cfg, err := loadOrNew()
	if err != nil {
		return err
	}
	for i := range cfg.Credentials {
		if cfg.Credentials[i].ID == credID {
			cfg.Credentials[i].SignCount = signCount
			return save(cfg)
		}
	}
	return nil
}

// Clear removes ALL remote-access material (passphrase and passkeys), disabling
// the gate. Local recovery path; requires local shell access.
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
