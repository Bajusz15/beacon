package deploy

import (
	"os"
	"path/filepath"
	"testing"

	"beacon/internal/config"
	"beacon/internal/secrets"
)

func TestCommandEnvMissingSecretsDoesNotFail(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())
	cfg := &config.Config{ProjectName: "myapp", ProjectEnv: "prod"}
	if _, err := CommandEnv(cfg); err != nil {
		t.Fatalf("CommandEnv() error = %v", err)
	}
}

func TestCommandEnvLoadsSecureEnvFile(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())
	envFile := filepath.Join(t.TempDir(), "secure.env")
	if err := os.WriteFile(envFile, []byte("FROM_ENV=1\nQUOTED=\"hello\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{ProjectName: "myapp", ProjectEnv: "prod", SecureEnvPath: envFile}
	env, err := CommandEnv(cfg)
	if err != nil {
		t.Fatalf("CommandEnv() error = %v", err)
	}
	if got := envValue(env, "FROM_ENV"); got != "1" {
		t.Fatalf("FROM_ENV = %q, want 1", got)
	}
	if got := envValue(env, "QUOTED"); got != "hello" {
		t.Fatalf("QUOTED = %q, want hello", got)
	}
}

func TestCommandEnvSecretsOverrideSecureEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BEACON_HOME", home)
	envFile := filepath.Join(t.TempDir(), "secure.env")
	if err := os.WriteFile(envFile, []byte("TOKEN=from-env\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := secrets.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("myapp", "prod", "TOKEN", "from-secret"); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{ProjectName: "myapp", ProjectEnv: "prod", SecureEnvPath: envFile}
	env, err := CommandEnv(cfg)
	if err != nil {
		t.Fatalf("CommandEnv() error = %v", err)
	}
	if got := envValue(env, "TOKEN"); got != "from-secret" {
		t.Fatalf("TOKEN = %q, want from-secret", got)
	}
}

func TestCommandEnvCorruptedExistingSecretsFails(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())
	store, err := secrets.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("myapp", "prod", "TOKEN", "secret"); err != nil {
		t.Fatal(err)
	}
	path, err := store.Path("myapp", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{ProjectName: "myapp", ProjectEnv: "prod"}
	if _, err := CommandEnv(cfg); err == nil {
		t.Fatal("CommandEnv() error = nil, want corrupted secrets error")
	}
}

func TestCommandEnvSecretsDisabledSkipsExistingFile(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())
	store, err := secrets.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("myapp", "prod", "TOKEN", "secret"); err != nil {
		t.Fatal(err)
	}
	path, err := store.Path("myapp", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	disabled := false
	cfg := &config.Config{ProjectName: "myapp", ProjectEnv: "prod", SecretsEnabled: &disabled}
	if _, err := CommandEnv(cfg); err != nil {
		t.Fatalf("CommandEnv() error = %v, want nil when secrets disabled", err)
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, item := range env {
		if len(item) >= len(prefix) && item[:len(prefix)] == prefix {
			return item[len(prefix):]
		}
	}
	return ""
}
