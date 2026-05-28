package keys

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

// GenerateSSHKeyPair creates an ed25519 keypair and returns the private key
// in PEM format and the public key in OpenSSH authorized_keys format.
func GenerateSSHKeyPair() (privateKeyPEM []byte, publicKeyOpenSSH []byte, err error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate ed25519 key: %w", err)
	}

	sshPub, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return nil, nil, fmt.Errorf("convert to ssh public key: %w", err)
	}
	publicKeyOpenSSH = ssh.MarshalAuthorizedKey(sshPub)

	pemBlock, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return nil, nil, fmt.Errorf("marshal private key: %w", err)
	}
	privateKeyPEM = pem.EncodeToMemory(pemBlock)

	return privateKeyPEM, publicKeyOpenSSH, nil
}

// GenerateAndStoreSSHKey creates an ed25519 keypair, stores the private key
// in the encrypted key manager, and writes {name}.pub alongside it.
// Returns the public key in OpenSSH format.
func (km *KeyManager) GenerateAndStoreSSHKey(name string) (string, error) {
	privPEM, pubSSH, err := GenerateSSHKeyPair()
	if err != nil {
		return "", err
	}

	if err := km.AddKey(name, string(privPEM), "ssh", "SSH deploy key"); err != nil {
		return "", fmt.Errorf("store private key: %w", err)
	}

	pubFile := filepath.Join(km.configDir, "keys", name+".pub")
	if err := os.WriteFile(pubFile, pubSSH, 0644); err != nil {
		return "", fmt.Errorf("write public key: %w", err)
	}

	return string(pubSSH), nil
}

// ReadSSHPublicKey reads a .pub file for the named key, if it exists.
func ReadSSHPublicKey(configDir, name string) string {
	pubFile := filepath.Join(configDir, "keys", name+".pub")
	data, err := os.ReadFile(pubFile)
	if err != nil {
		return ""
	}
	return string(data)
}
