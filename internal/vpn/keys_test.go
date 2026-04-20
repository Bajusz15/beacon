package vpn

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// isolateBeaconHome points BEACON_HOME at a per-test tempdir so key files
// don't leak across tests or stomp on the user's real ~/.beacon.
func isolateBeaconHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BEACON_HOME", dir)
	return dir
}

func TestGenerateKeyPair_validBase64(t *testing.T) {
	kp, err := GenerateKeyPair()
	require.NoError(t, err)
	require.NotNil(t, kp)
	require.NoError(t, EnsureBase64(kp.PrivateKey))
	require.NoError(t, EnsureBase64(kp.PublicKey))
	require.NotEqual(t, kp.PrivateKey, kp.PublicKey, "private and public must differ")

	// Round-trip the public key through wgtypes — that's what the WireGuard
	// device will do when configuring a peer, so it must parse cleanly.
	_, err = wgtypes.ParseKey(kp.PublicKey)
	require.NoError(t, err)
}

func TestGenerateKeyPair_unique(t *testing.T) {
	a, err := GenerateKeyPair()
	require.NoError(t, err)
	b, err := GenerateKeyPair()
	require.NoError(t, err)
	require.NotEqual(t, a.PrivateKey, b.PrivateKey, "every key pair should be unique")
}

func TestLoadOrCreatePrivateKey_persistsAcrossCalls(t *testing.T) {
	isolateBeaconHome(t)

	first, err := LoadOrCreatePrivateKey()
	require.NoError(t, err)
	require.NotNil(t, first)
	require.NoError(t, EnsureBase64(first.PrivateKey))

	// A second call should return the same key — the whole point is persistence.
	second, err := LoadOrCreatePrivateKey()
	require.NoError(t, err)
	require.Equal(t, first.PrivateKey, second.PrivateKey)
	require.Equal(t, first.PublicKey, second.PublicKey)
}

func TestLoadOrCreatePrivateKey_filePermissionsAndEncryption(t *testing.T) {
	home := isolateBeaconHome(t)

	kp, err := LoadOrCreatePrivateKey()
	require.NoError(t, err)
	require.NotEmpty(t, kp.PrivateKey)

	keyPath := filepath.Join(home, "vpn", "private.key")
	info, err := os.Stat(keyPath)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0600), info.Mode().Perm(), "private key must be 0600")
	}

	// The file on disk MUST NOT be the plaintext base64 key — that's the
	// whole reason we encrypt it under the master key.
	raw, err := os.ReadFile(keyPath)
	require.NoError(t, err)
	require.NotEqual(t, []byte(kp.PrivateKey), raw, "stored key must be encrypted, not plaintext")
	require.NotContains(t, string(raw), kp.PrivateKey, "ciphertext must not contain the plaintext key")

	// And the master key file should also exist with locked-down perms.
	mkPath := filepath.Join(home, ".master_key")
	mk, err := os.Stat(mkPath)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0600), mk.Mode().Perm())
	}
}

func TestLoadOrCreatePrivateKey_corruptedFile(t *testing.T) {
	home := isolateBeaconHome(t)

	// Bootstrap a valid key + master key, then clobber the encrypted blob.
	_, err := LoadOrCreatePrivateKey()
	require.NoError(t, err)

	keyPath := filepath.Join(home, "vpn", "private.key")
	require.NoError(t, os.WriteFile(keyPath, []byte("not encrypted at all"), 0600))

	_, err = LoadOrCreatePrivateKey()
	require.Error(t, err, "decryption of garbage should fail loudly, not silently regenerate")
}

func TestDeletePrivateKey_idempotent(t *testing.T) {
	home := isolateBeaconHome(t)

	// Delete with no key present is a no-op.
	require.NoError(t, DeletePrivateKey())

	_, err := LoadOrCreatePrivateKey()
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(home, "vpn", "private.key"))

	require.NoError(t, DeletePrivateKey())
	_, err = os.Stat(filepath.Join(home, "vpn", "private.key"))
	require.True(t, os.IsNotExist(err), "key file should be gone after Delete")

	// Calling Delete again must still succeed.
	require.NoError(t, DeletePrivateKey())
}

func TestEnsureBase64(t *testing.T) {
	good, err := wgtypes.GeneratePrivateKey()
	require.NoError(t, err)
	require.NoError(t, EnsureBase64(good.String()))

	require.Error(t, EnsureBase64(""), "empty string is not valid base64 for a wg key")
	require.Error(t, EnsureBase64("not-base64!!!"))

	// Wrong length: 16 random bytes -> base64 -> too short for a WG key.
	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	require.Error(t, EnsureBase64(short))
}
