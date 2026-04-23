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

func isolateBeaconHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BEACON_HOME", dir)
	return dir
}

func TestGenerateKeyPair(t *testing.T) {
	t.Run("valid base64", func(t *testing.T) {
		kp, err := GenerateKeyPair()
		require.NoError(t, err)
		require.NotNil(t, kp)
		require.NoError(t, EnsureBase64(kp.PrivateKey))
		require.NoError(t, EnsureBase64(kp.PublicKey))
		require.NotEqual(t, kp.PrivateKey, kp.PublicKey)

		_, err = wgtypes.ParseKey(kp.PublicKey)
		require.NoError(t, err)
	})

	t.Run("unique each time", func(t *testing.T) {
		a, err := GenerateKeyPair()
		require.NoError(t, err)
		b, err := GenerateKeyPair()
		require.NoError(t, err)
		require.NotEqual(t, a.PrivateKey, b.PrivateKey)
	})
}

func TestLoadOrCreatePrivateKey(t *testing.T) {
	t.Run("persists across calls", func(t *testing.T) {
		isolateBeaconHome(t)

		first, err := LoadOrCreatePrivateKey()
		require.NoError(t, err)
		require.NoError(t, EnsureBase64(first.PrivateKey))

		second, err := LoadOrCreatePrivateKey()
		require.NoError(t, err)
		require.Equal(t, first.PrivateKey, second.PrivateKey)
		require.Equal(t, first.PublicKey, second.PublicKey)
	})

	t.Run("file permissions and encryption", func(t *testing.T) {
		home := isolateBeaconHome(t)

		kp, err := LoadOrCreatePrivateKey()
		require.NoError(t, err)

		keyPath := filepath.Join(home, "vpn", "private.key")
		info, err := os.Stat(keyPath)
		require.NoError(t, err)
		if runtime.GOOS != "windows" {
			require.Equal(t, os.FileMode(0600), info.Mode().Perm())
		}

		raw, err := os.ReadFile(keyPath)
		require.NoError(t, err)
		require.NotEqual(t, []byte(kp.PrivateKey), raw, "stored key must be encrypted")
		require.NotContains(t, string(raw), kp.PrivateKey)

		mkPath := filepath.Join(home, ".master_key")
		mk, err := os.Stat(mkPath)
		require.NoError(t, err)
		if runtime.GOOS != "windows" {
			require.Equal(t, os.FileMode(0600), mk.Mode().Perm())
		}
	})

	t.Run("corrupted file errors", func(t *testing.T) {
		home := isolateBeaconHome(t)

		_, err := LoadOrCreatePrivateKey()
		require.NoError(t, err)

		keyPath := filepath.Join(home, "vpn", "private.key")
		require.NoError(t, os.WriteFile(keyPath, []byte("not encrypted at all"), 0600))

		_, err = LoadOrCreatePrivateKey()
		require.Error(t, err)
	})
}

func TestDeletePrivateKey(t *testing.T) {
	home := isolateBeaconHome(t)

	t.Run("no-op when no key exists", func(t *testing.T) {
		require.NoError(t, DeletePrivateKey())
	})

	t.Run("removes existing key", func(t *testing.T) {
		_, err := LoadOrCreatePrivateKey()
		require.NoError(t, err)
		require.FileExists(t, filepath.Join(home, "vpn", "private.key"))

		require.NoError(t, DeletePrivateKey())
		_, err = os.Stat(filepath.Join(home, "vpn", "private.key"))
		require.True(t, os.IsNotExist(err))
	})

	t.Run("idempotent after delete", func(t *testing.T) {
		require.NoError(t, DeletePrivateKey())
	})
}

func TestEnsureBase64(t *testing.T) {
	t.Run("valid key", func(t *testing.T) {
		good, err := wgtypes.GeneratePrivateKey()
		require.NoError(t, err)
		require.NoError(t, EnsureBase64(good.String()))
	})

	t.Run("empty string", func(t *testing.T) {
		require.Error(t, EnsureBase64(""))
	})

	t.Run("invalid base64", func(t *testing.T) {
		require.Error(t, EnsureBase64("not-base64!!!"))
	})

	t.Run("wrong length", func(t *testing.T) {
		short := base64.StdEncoding.EncodeToString(make([]byte, 16))
		require.Error(t, EnsureBase64(short))
	})
}
