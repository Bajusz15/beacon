package secrets

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStoreKeyGenerationAndMode(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	if err := store.Set("myapp", "default", "API_KEY", "secret"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	info, err := os.Stat(store.keyPath)
	if err != nil {
		t.Fatalf("stat key: %v", err)
	}
	if got := info.Mode().Perm(); got != keyFileMode {
		t.Fatalf("key mode = %o, want %o", got, keyFileMode)
	}
}

func TestStoreRejectsOpenKeyPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix file permission test")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dir, "key")
	if err := os.WriteFile(keyPath, make([]byte, keySize), 0644); err != nil {
		t.Fatal(err)
	}
	store := NewStoreAt(dir)
	if err := store.Set("myapp", "default", "API_KEY", "secret"); err == nil {
		t.Fatal("Set() error = nil, want permission error")
	}
}

func TestStoreSetListGetRemove(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	if err := store.Set("myapp", "prod", "TOKEN", "abc"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	keys, err := store.List("myapp", "prod")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(keys) != 1 || keys[0] != "TOKEN" {
		t.Fatalf("List() = %#v, want TOKEN", keys)
	}
	value, err := store.Get("myapp", "prod", "TOKEN")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != "abc" {
		t.Fatalf("Get() = %q, want abc", value)
	}
	if err := store.Remove("myapp", "prod", "TOKEN"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := store.Get("myapp", "prod", "TOKEN"); err != ErrNotFound {
		t.Fatalf("Get() after remove error = %v, want ErrNotFound", err)
	}
}

func TestStoreProjectEnvIsolation(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	if err := store.Set("myapp", "prod", "TOKEN", "prod"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("myapp", "staging", "TOKEN", "staging"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("otherapp", "prod", "TOKEN", "other"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		project string
		env     string
		want    string
	}{
		{"myapp", "prod", "prod"},
		{"myapp", "staging", "staging"},
		{"otherapp", "prod", "other"},
	}
	for _, tt := range tests {
		got, err := store.Get(tt.project, tt.env, "TOKEN")
		if err != nil {
			t.Fatalf("Get(%s/%s) error = %v", tt.project, tt.env, err)
		}
		if got != tt.want {
			t.Fatalf("Get(%s/%s) = %q, want %q", tt.project, tt.env, got, tt.want)
		}
	}
}

func TestStoreCorruptedEncryptedFileFails(t *testing.T) {
	store := NewStoreAt(t.TempDir())
	if err := store.Set("myapp", "prod", "TOKEN", "abc"); err != nil {
		t.Fatal(err)
	}
	path, err := store.Path("myapp", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("myapp", "prod"); err == nil {
		t.Fatal("Load() error = nil, want corrupted file error")
	}
}
