package vpn

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"beacon/internal/config"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// KeyPair holds a WireGuard private/public key pair (base64-encoded, the format
// every WireGuard tool consumes).
type KeyPair struct {
	PrivateKey string
	PublicKey  string
}

// GenerateKeyPair creates a fresh WireGuard key pair using the same primitives
// (Curve25519) as `wg genkey` / `wg pubkey`.
func GenerateKeyPair() (*KeyPair, error) {
	priv, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return nil, fmt.Errorf("generate wireguard key: %w", err)
	}
	return &KeyPair{
		PrivateKey: priv.String(),
		PublicKey:  priv.PublicKey().String(),
	}, nil
}

// vpnKeyDir returns ~/.beacon/vpn — created lazily on first key write.
func vpnKeyDir() (string, error) {
	base, err := config.BeaconHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "vpn"), nil
}

// privateKeyPath returns the on-disk path of the encrypted WireGuard private key.
func privateKeyPath() (string, error) {
	dir, err := vpnKeyDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "private.key"), nil
}

// LoadOrCreatePrivateKey returns the device's WireGuard key pair, generating
// a new one on first call. The private key is stored AES-GCM encrypted under
// the existing master key (~/.beacon/.master_key) so a stolen disk image
// without the master key can't be used to impersonate the device.
//
// Note: in v1 the master key sits next to the encrypted blob, so this is mainly
// defense-in-depth (raises the bar against casual snooping / scraped backups).
// A future iteration may protect the master key with a passphrase.
func LoadOrCreatePrivateKey() (*KeyPair, error) {
	path, err := privateKeyPath()
	if err != nil {
		return nil, err
	}
	masterKey, err := loadOrCreateMasterKey()
	if err != nil {
		return nil, err
	}

	if data, err := os.ReadFile(path); err == nil {
		plain, err := decryptAESGCM(masterKey, data)
		if err != nil {
			return nil, fmt.Errorf("decrypt vpn private key: %w", err)
		}
		priv, err := wgtypes.ParseKey(string(plain))
		if err != nil {
			return nil, fmt.Errorf("parse stored vpn key: %w", err)
		}
		return &KeyPair{PrivateKey: priv.String(), PublicKey: priv.PublicKey().String()}, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read vpn private key: %w", err)
	}

	kp, err := GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	enc, err := encryptAESGCM(masterKey, []byte(kp.PrivateKey))
	if err != nil {
		return nil, fmt.Errorf("encrypt vpn private key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("mkdir vpn dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, enc, 0600); err != nil {
		return nil, fmt.Errorf("write vpn private key: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return nil, fmt.Errorf("rename vpn private key: %w", err)
	}
	return kp, nil
}

// DeletePrivateKey removes the stored private key (used by `beacon vpn disable`).
func DeletePrivateKey() error {
	path, err := privateKeyPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// loadOrCreateMasterKey mirrors internal/keys.KeyManager.initMasterKey so the VPN
// package doesn't pull in the full KeyManager (and avoids a circular import via
// projects). The file format is identical: 32 raw bytes at ~/.beacon/.master_key.
func loadOrCreateMasterKey() ([]byte, error) {
	base, err := config.BeaconHomeDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(base, ".master_key")
	if data, err := os.ReadFile(path); err == nil {
		if len(data) != 32 {
			return nil, errors.New("master key file has wrong length")
		}
		return data, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0600); err != nil {
		return nil, err
	}
	return key, nil
}

func encryptAESGCM(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func decryptAESGCM(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}

// EnsureBase64 validates that a string is a valid base64-encoded WireGuard key
// (32 bytes after decoding). Returned by helpers that accept user-supplied keys.
func EnsureBase64(s string) error {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return fmt.Errorf("not valid base64: %w", err)
	}
	if len(raw) != wgtypes.KeyLen {
		return fmt.Errorf("wrong key length: got %d bytes, want %d", len(raw), wgtypes.KeyLen)
	}
	return nil
}
