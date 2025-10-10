package keys

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

// KeyManager handles API key rotation and management
type KeyManager struct {
	configDir string
	masterKey []byte
}

// StoredKey represents a stored API key with metadata
type StoredKey struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Key         string    `json:"key"`
	Provider    string    `json:"provider"`
	CreatedAt   time.Time `json:"created_at"`
	LastUsed    time.Time `json:"last_used"`
	IsActive    bool      `json:"is_active"`
	Description string    `json:"description,omitempty"`
}

// NewKeyManager creates a new key manager
func NewKeyManager(configDir string) (*KeyManager, error) {
	km := &KeyManager{
		configDir: configDir,
	}

	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Initialize master key for encryption
	if err := km.initMasterKey(); err != nil {
		return nil, fmt.Errorf("failed to initialize master key: %w", err)
	}

	// Ensure keys directory exists
	keysDir := filepath.Join(configDir, "keys")
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create keys directory: %w", err)
	}

	return km, nil
}

// initMasterKey initializes or loads the master encryption key
func (km *KeyManager) initMasterKey() error {
	keyFile := filepath.Join(km.configDir, ".master_key")

	// Try to load existing key
	if data, err := os.ReadFile(keyFile); err == nil {
		km.masterKey = data
		return nil
	}

	// Generate new master key
	km.masterKey = make([]byte, 32)
	if _, err := rand.Read(km.masterKey); err != nil {
		return fmt.Errorf("failed to generate master key: %w", err)
	}

	// Save master key
	if err := os.WriteFile(keyFile, km.masterKey, 0600); err != nil {
		return fmt.Errorf("failed to save master key: %w", err)
	}

	return nil
}

// AddKey stores a new API key
func (km *KeyManager) AddKey(name, key, provider, description string) error {
	storedKey := &StoredKey{
		ID:          generateKeyID(),
		Name:        name,
		Key:         key,
		Provider:    provider,
		CreatedAt:   time.Now(),
		LastUsed:    time.Now(),
		IsActive:    true,
		Description: description,
	}

	return km.saveKey(storedKey)
}

// GetKey retrieves a stored API key
func (km *KeyManager) GetKey(name string) (*StoredKey, error) {
	keyFile := filepath.Join(km.configDir, "keys", name+".json")

	data, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("key not found: %w", err)
	}

	// Decrypt the key data
	decryptedData, err := km.decrypt(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt key: %w", err)
	}

	var storedKey StoredKey
	if err := json.Unmarshal(decryptedData, &storedKey); err != nil {
		return nil, fmt.Errorf("failed to unmarshal key: %w", err)
	}

	// Update last used time
	storedKey.LastUsed = time.Now()
	km.saveKey(&storedKey)

	return &storedKey, nil
}

// ListKeys returns all stored keys
func (km *KeyManager) ListKeys() ([]StoredKey, error) {
	keysDir := filepath.Join(km.configDir, "keys")

	files, err := os.ReadDir(keysDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read keys directory: %w", err)
	}

	var keys []StoredKey
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		name := file.Name()[:len(file.Name())-5] // Remove .json extension
		key, err := km.GetKey(name)
		if err != nil {
			continue // Skip invalid keys
		}

		keys = append(keys, *key)
	}

	return keys, nil
}

// RotateKey replaces an existing key with a new one
func (km *KeyManager) RotateKey(name, newKey string) error {
	// Get existing key
	existingKey, err := km.GetKey(name)
	if err != nil {
		return fmt.Errorf("failed to get existing key: %w", err)
	}

	// Update with new key
	existingKey.Key = newKey
	existingKey.LastUsed = time.Now()

	return km.saveKey(existingKey)
}

// DeleteKey removes a stored API key
func (km *KeyManager) DeleteKey(name string) error {
	keyFile := filepath.Join(km.configDir, "keys", name+".json")

	// Check if key exists
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return fmt.Errorf("key '%s' not found", name)
	}

	// Delete the key file
	err := os.Remove(keyFile)
	if err != nil {
		return fmt.Errorf("failed to delete key file: %w", err)
	}

	return nil
}

// ValidateKey tests if a key is valid by making a test request
func (km *KeyManager) ValidateKey(name string) error {
	key, err := km.GetKey(name)
	if err != nil {
		return fmt.Errorf("failed to get key: %w", err)
	}

	// This would make an actual API call to validate the key
	// For now, just check if key exists and is not empty
	if key.Key == "" {
		return fmt.Errorf("key is empty")
	}

	return nil
}

// saveKey encrypts and saves a key to disk
func (km *KeyManager) saveKey(key *StoredKey) error {
	data, err := json.Marshal(key)
	if err != nil {
		return fmt.Errorf("failed to marshal key: %w", err)
	}

	encryptedData, err := km.encrypt(data)
	if err != nil {
		return fmt.Errorf("failed to encrypt key: %w", err)
	}

	keyFile := filepath.Join(km.configDir, "keys", key.Name+".json")
	if err := os.WriteFile(keyFile, encryptedData, 0600); err != nil {
		return fmt.Errorf("failed to save key file: %w", err)
	}

	return nil
}

// encrypt encrypts data using AES-GCM
func (km *KeyManager) encrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(km.masterKey)
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

	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// decrypt decrypts data using AES-GCM
func (km *KeyManager) decrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(km.masterKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// generateKeyID creates a unique key ID
func generateKeyID() string {
	data := make([]byte, 16)
	rand.Read(data)
	return fmt.Sprintf("%x", data)
}

// deriveKey derives a key from a password using PBKDF2
func deriveKey(password string, salt []byte) []byte {
	return pbkdf2.Key([]byte(password), salt, 4096, 32, sha256.New)
}
