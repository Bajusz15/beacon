package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"beacon/internal/config"
)

const (
	keySize         = 32
	keyFileMode     = 0600
	secretsDirMode  = 0700
	secretFileMode  = 0600
	envelopeVersion = 1
)

var ErrNotFound = errors.New("secret not found")

type envelope struct {
	Version    int    `json:"version"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type Store struct {
	baseDir string
	keyPath string
}

func NewStore() (*Store, error) {
	base, err := config.BeaconHomeDir()
	if err != nil {
		return nil, fmt.Errorf("beacon home: %w", err)
	}
	secretsDir := filepath.Join(base, "secrets")
	return &Store{
		baseDir: secretsDir,
		keyPath: filepath.Join(secretsDir, "key"),
	}, nil
}

func NewStoreAt(baseDir string) *Store {
	return &Store{
		baseDir: baseDir,
		keyPath: filepath.Join(baseDir, "key"),
	}
}

func (s *Store) Path(project, env string) (string, error) {
	if err := ValidateProject(project); err != nil {
		return "", err
	}
	if err := ValidateEnv(env); err != nil {
		return "", err
	}
	return filepath.Join(s.baseDir, project, env+".enc"), nil
}

func (s *Store) Exists(project, env string) (bool, error) {
	path, err := s.Path(project, env)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *Store) Load(project, env string) (map[string]string, error) {
	path, err := s.Path(project, env)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	key, err := s.loadOrCreateKey()
	if err != nil {
		return nil, err
	}
	plain, err := decrypt(key, data)
	if err != nil {
		return nil, err
	}
	var values map[string]string
	if err := json.Unmarshal(plain, &values); err != nil {
		return nil, fmt.Errorf("parse secrets payload: %w", err)
	}
	if values == nil {
		values = map[string]string{}
	}
	return values, nil
}

func (s *Store) Set(project, env, key, value string) error {
	if err := ValidateSecretKey(key); err != nil {
		return err
	}
	values, err := s.Load(project, env)
	if err != nil {
		return err
	}
	values[key] = value
	return s.save(project, env, values)
}

func (s *Store) Get(project, env, key string) (string, error) {
	if err := ValidateSecretKey(key); err != nil {
		return "", err
	}
	values, err := s.Load(project, env)
	if err != nil {
		return "", err
	}
	value, ok := values[key]
	if !ok {
		return "", ErrNotFound
	}
	return value, nil
}

func (s *Store) List(project, env string) ([]string, error) {
	values, err := s.Load(project, env)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *Store) Remove(project, env, key string) error {
	if err := ValidateSecretKey(key); err != nil {
		return err
	}
	values, err := s.Load(project, env)
	if err != nil {
		return err
	}
	if _, ok := values[key]; !ok {
		return ErrNotFound
	}
	delete(values, key)
	return s.save(project, env, values)
}

func (s *Store) save(project, env string, values map[string]string) error {
	path, err := s.Path(project, env)
	if err != nil {
		return err
	}
	key, err := s.loadOrCreateKey()
	if err != nil {
		return err
	}
	plain, err := json.Marshal(values)
	if err != nil {
		return err
	}
	encrypted, err := encrypt(key, plain)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), secretsDirMode); err != nil {
		return err
	}
	return writeFileAtomic(path, encrypted, secretFileMode)
}

func (s *Store) loadOrCreateKey() ([]byte, error) {
	if err := os.MkdirAll(s.baseDir, secretsDirMode); err != nil {
		return nil, err
	}
	info, err := os.Stat(s.keyPath)
	if err == nil {
		if info.Mode().Perm() != keyFileMode {
			return nil, fmt.Errorf("secrets key %s has permissions %o, want 600", s.keyPath, info.Mode().Perm())
		}
		key, err := os.ReadFile(s.keyPath)
		if err != nil {
			return nil, err
		}
		if len(key) != keySize {
			return nil, fmt.Errorf("secrets key %s has invalid length %d", s.keyPath, len(key))
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if err := writeFileExclusive(s.keyPath, key, keyFileMode); err != nil {
		if os.IsExist(err) {
			return s.loadOrCreateKey()
		}
		return nil, err
	}
	return key, nil
}

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
	syncDir(dir)
	return nil
}

// syncDir best-effort fsyncs a directory so that a preceding rename/create is
// durable across a crash. The data was already written and renamed
// successfully, so a sync failure is non-fatal: it is only logged as a warning.
// Some filesystems and platforms do not support syncing a directory handle.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		logger.Warnf("could not open directory %s to fsync: %v", dir, err)
		return
	}
	if err := d.Sync(); err != nil {
		logger.Warnf("could not fsync directory %s: %v", dir, err)
	}
	if err := d.Close(); err != nil {
		logger.Warnf("could not close directory %s after fsync: %v", dir, err)
	}
}

func writeFileExclusive(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	syncDir(filepath.Dir(path))
	return nil
}

func encrypt(key, plain []byte) ([]byte, error) {
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
	env := envelope{
		Version:    envelopeVersion,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, plain, nil)),
	}
	return json.Marshal(env)
}

func decrypt(key, data []byte) ([]byte, error) {
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("parse encrypted secrets envelope: %w", err)
	}
	if env.Version != envelopeVersion {
		return nil, fmt.Errorf("unsupported secrets envelope version %d", env.Version)
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decode nonce: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt secrets: %w", err)
	}
	return plain, nil
}

func ValidateProject(name string) error {
	paths, err := config.NewBeaconPaths()
	if err != nil {
		return err
	}
	return paths.ValidateProjectName(name)
}

func ValidateEnv(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("env name cannot be empty")
	}
	return validateSafeName("env name", name)
}

func ValidateSecretKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("secret key cannot be empty")
	}
	for _, char := range key {
		if (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') &&
			char != '_' {
			return fmt.Errorf("secret key contains invalid character %q. Only uppercase letters, numbers, and underscores are allowed", char)
		}
	}
	return nil
}

func validateSafeName(label, name string) error {
	for _, char := range name {
		if (char < 'a' || char > 'z') &&
			(char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') &&
			char != '-' && char != '_' {
			return fmt.Errorf("%s contains invalid character %q. Only letters, numbers, hyphens, and underscores are allowed", label, char)
		}
	}
	return nil
}
