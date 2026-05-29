package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Run("project metadata and secrets settings", func(t *testing.T) {
		localPath := filepath.Join(t.TempDir(), "app")
		t.Setenv("BEACON_DEPLOYMENT_TYPE", "git")
		t.Setenv("BEACON_LOCAL_PATH", localPath)
		t.Setenv("BEACON_REPO_URL", "https://example.com/repo.git")
		t.Setenv("BEACON_PORT", "9090")
		t.Setenv("BEACON_POLL_INTERVAL", "2m")
		t.Setenv("BEACON_DEPLOY_CMD", "./deploy.sh")
		t.Setenv("BEACON_SECURE_ENV_PATH", "")
		t.Setenv("BEACON_PROJECT_NAME", "myapp")
		t.Setenv("BEACON_PROJECT_ENV", "prod")
		t.Setenv("BEACON_SECRETS_ENABLED", "true")

		cfg := Load()
		if cfg.ProjectName != "myapp" {
			t.Fatalf("ProjectName = %q, want myapp", cfg.ProjectName)
		}
		if cfg.ProjectEnv != "prod" {
			t.Fatalf("ProjectEnv = %q, want prod", cfg.ProjectEnv)
		}
		if cfg.SecretsEnabled == nil || !*cfg.SecretsEnabled {
			t.Fatalf("SecretsEnabled = %v, want true", cfg.SecretsEnabled)
		}
		if cfg.PollInterval != 2*time.Minute {
			t.Fatalf("PollInterval = %v, want 2m", cfg.PollInterval)
		}
	})

	t.Run("defaults project env and project name", func(t *testing.T) {
		localPath := filepath.Join(t.TempDir(), "fallback-app")
		t.Setenv("BEACON_DEPLOYMENT_TYPE", "git")
		t.Setenv("BEACON_LOCAL_PATH", localPath)
		t.Setenv("BEACON_REPO_URL", "https://example.com/repo.git")
		t.Setenv("BEACON_SECURE_ENV_PATH", "")
		t.Setenv("BEACON_PROJECT_ENV", "")
		t.Setenv("BEACON_PROJECT_NAME", "")
		t.Setenv("BEACON_SECRETS_ENABLED", "")

		cfg := Load()
		if cfg.ProjectEnv != "default" {
			t.Fatalf("ProjectEnv = %q, want default", cfg.ProjectEnv)
		}
		if cfg.ProjectName != "fallback-app" {
			t.Fatalf("ProjectName = %q, want fallback-app", cfg.ProjectName)
		}
	})
}

func TestGetOptionalBoolEnv(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  *bool
	}{
		{name: "missing", value: "", want: nil},
		{name: "true", value: "true", want: boolPtr(true)},
		{name: "one", value: "1", want: boolPtr(true)},
		{name: "false", value: "false", want: boolPtr(false)},
		{name: "zero", value: "0", want: boolPtr(false)},
		{name: "invalid", value: "maybe", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TEST_BOOL", tt.value)
			got := getOptionalBoolEnv("TEST_BOOL")
			if tt.want == nil {
				if got != nil {
					t.Fatalf("getOptionalBoolEnv() = %v, want nil", *got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Fatalf("getOptionalBoolEnv() = %v, want %v", got, *tt.want)
			}
		})
	}
}

func boolPtr(v bool) *bool {
	return &v
}
