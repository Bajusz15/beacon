package keys

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func newTestKeyManager(t *testing.T) *KeyManager {
	t.Helper()
	dir := t.TempDir()
	km, err := NewKeyManager(dir)
	require.NoError(t, err)
	return km
}

func TestGenerateSSHKeyPair(t *testing.T) {
	t.Run("returns valid ed25519 keypair", func(t *testing.T) {
		privPEM, pubSSH, err := GenerateSSHKeyPair()
		require.NoError(t, err)
		require.NotEmpty(t, privPEM)
		require.NotEmpty(t, pubSSH)

		// Private key must parse back to ed25519
		parsed, err := ssh.ParseRawPrivateKey(privPEM)
		require.NoError(t, err)
		privKey, ok := parsed.(*ed25519.PrivateKey)
		require.True(t, ok, "expected *ed25519.PrivateKey, got %T", parsed)

		// Public key must parse as authorized_keys format
		sshPub, _, _, _, err := ssh.ParseAuthorizedKey(pubSSH)
		require.NoError(t, err)
		require.Equal(t, "ssh-ed25519", sshPub.Type())

		// Public key derived from private must match
		derivedPub, err := ssh.NewPublicKey(privKey.Public())
		require.NoError(t, err)
		require.Equal(t, sshPub.Marshal(), derivedPub.Marshal())
	})

	t.Run("each call produces a unique keypair", func(t *testing.T) {
		_, pubA, err := GenerateSSHKeyPair()
		require.NoError(t, err)
		_, pubB, err := GenerateSSHKeyPair()
		require.NoError(t, err)
		require.NotEqual(t, pubA, pubB)
	})

	t.Run("private key can sign and public key verifies", func(t *testing.T) {
		privPEM, pubSSH, err := GenerateSSHKeyPair()
		require.NoError(t, err)

		raw, err := ssh.ParseRawPrivateKey(privPEM)
		require.NoError(t, err)
		signer, err := ssh.NewSignerFromKey(raw)
		require.NoError(t, err)

		message := []byte("beacon deploy test payload")
		sig, err := signer.Sign(nil, message)
		require.NoError(t, err)

		sshPub, _, _, _, err := ssh.ParseAuthorizedKey(pubSSH)
		require.NoError(t, err)
		require.NoError(t, sshPub.Verify(message, sig))
	})
}

func TestGenerateAndStoreSSHKey(t *testing.T) {
	t.Run("stores private key and writes .pub file", func(t *testing.T) {
		km := newTestKeyManager(t)

		pubKeyStr, err := km.GenerateAndStoreSSHKey("deploy-key")
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(pubKeyStr, "ssh-ed25519 "))

		// .pub file exists and matches returned value
		pubFile := filepath.Join(km.configDir, "keys", "deploy-key.pub")
		pubData, err := os.ReadFile(pubFile)
		require.NoError(t, err)
		require.Equal(t, pubKeyStr, string(pubData))

		// encrypted private key exists
		privFile := filepath.Join(km.configDir, "keys", "deploy-key.json")
		require.FileExists(t, privFile)
		encryptedData, err := os.ReadFile(privFile)
		require.NoError(t, err)
		// must NOT contain PEM markers in plaintext (it's encrypted)
		require.NotContains(t, string(encryptedData), "OPENSSH PRIVATE KEY")
	})

	t.Run("stored private key can be retrieved and used", func(t *testing.T) {
		km := newTestKeyManager(t)

		pubKeyStr, err := km.GenerateAndStoreSSHKey("roundtrip-key")
		require.NoError(t, err)

		// Retrieve the stored key
		stored, err := km.GetKey("roundtrip-key")
		require.NoError(t, err)
		require.Equal(t, "ssh", stored.Provider)
		require.Equal(t, "SSH deploy key", stored.Description)
		require.True(t, stored.IsActive)

		// Parse the decrypted private key and verify it matches the public key
		raw, err := ssh.ParseRawPrivateKey([]byte(stored.Key))
		require.NoError(t, err)
		privKey, ok := raw.(*ed25519.PrivateKey)
		require.True(t, ok)

		derivedPub, err := ssh.NewPublicKey(privKey.Public())
		require.NoError(t, err)
		derivedPubStr := string(ssh.MarshalAuthorizedKey(derivedPub))
		require.Equal(t, pubKeyStr, derivedPubStr)
	})

	t.Run("multiple keys don't collide", func(t *testing.T) {
		km := newTestKeyManager(t)

		pubA, err := km.GenerateAndStoreSSHKey("key-a")
		require.NoError(t, err)
		pubB, err := km.GenerateAndStoreSSHKey("key-b")
		require.NoError(t, err)

		require.NotEqual(t, pubA, pubB)

		storedA, err := km.GetKey("key-a")
		require.NoError(t, err)
		storedB, err := km.GetKey("key-b")
		require.NoError(t, err)
		require.NotEqual(t, storedA.Key, storedB.Key)
	})
}

func TestReadSSHPublicKey(t *testing.T) {
	t.Run("returns public key after generation", func(t *testing.T) {
		km := newTestKeyManager(t)

		expected, err := km.GenerateAndStoreSSHKey("read-test")
		require.NoError(t, err)

		got := ReadSSHPublicKey(km.configDir, "read-test")
		require.Equal(t, expected, got)
	})

	t.Run("returns empty string for missing key", func(t *testing.T) {
		dir := t.TempDir()
		got := ReadSSHPublicKey(dir, "nonexistent")
		require.Empty(t, got)
	})
}
