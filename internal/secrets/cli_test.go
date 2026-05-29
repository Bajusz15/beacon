package secrets

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandSetListGetRemove(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())

	run := func(args ...string) (string, error) {
		cmd := Command()
		var out bytes.Buffer
		var errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		cmd.SetArgs(args)
		err := cmd.Execute()
		return out.String() + errOut.String(), err
	}

	if _, err := run("--project", "myapp", "--env", "prod", "set", "API_KEY", "secret"); err != nil {
		t.Fatalf("set error = %v", err)
	}
	out, err := run("--project", "myapp", "--env", "prod", "list")
	if err != nil {
		t.Fatalf("list error = %v", err)
	}
	if strings.TrimSpace(out) != "API_KEY" {
		t.Fatalf("list output = %q, want API_KEY", out)
	}
	out, err = run("--project", "myapp", "--env", "prod", "list", "--reveal")
	if err != nil {
		t.Fatalf("list --reveal error = %v", err)
	}
	if strings.TrimSpace(out) != "API_KEY='secret'" {
		t.Fatalf("list --reveal output = %q, want API_KEY='secret'", out)
	}
	out, err = run("--project", "myapp", "--env", "prod", "get", "API_KEY", "--reveal")
	if err != nil {
		t.Fatalf("get error = %v", err)
	}
	if strings.TrimSpace(out) != "secret" {
		t.Fatalf("get output = %q, want secret", out)
	}
	if _, err := run("--project", "myapp", "--env", "prod", "get", "API_KEY"); err == nil {
		t.Fatal("get without --reveal error = nil, want error")
	}
	if _, err := run("--project", "myapp", "--env", "prod", "export"); err == nil {
		t.Fatal("export without --reveal error = nil, want error")
	}
	out, err = run("--project", "myapp", "--env", "prod", "export", "--reveal")
	if err != nil {
		t.Fatalf("export env error = %v", err)
	}
	if strings.TrimSpace(out) != "API_KEY='secret'" {
		t.Fatalf("export env output = %q, want API_KEY='secret'", out)
	}
	out, err = run("--project", "myapp", "--env", "prod", "export", "--reveal", "--format", "json")
	if err != nil {
		t.Fatalf("export json error = %v", err)
	}
	if !strings.Contains(out, `"API_KEY": "secret"`) {
		t.Fatalf("export json output = %q, want API_KEY value", out)
	}
	if _, err := run("--project", "myapp", "--env", "prod", "remove", "API_KEY"); err != nil {
		t.Fatalf("remove error = %v", err)
	}
	out, err = run("--project", "myapp", "--env", "prod", "list")
	if err != nil {
		t.Fatalf("list after remove error = %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("list after remove output = %q, want empty", out)
	}

	if _, err := run("--project", "myapp", "--env", "prod", "set", "API_KEY", "secret"); err != nil {
		t.Fatalf("set for unsupported format error = %v", err)
	}
	if _, err := run("--project", "myapp", "--env", "prod", "export", "--reveal", "--format", "yaml"); err == nil {
		t.Fatal("export unsupported format error = nil, want error")
	}
}

func TestResolveCLIEnv(t *testing.T) {
	t.Run("flag beats env file", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("BEACON_HOME", home)
		configDir := filepath.Join(home, "config", "projects", "myapp")
		if err := os.MkdirAll(configDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(configDir, "env"), []byte("BEACON_PROJECT_ENV=from_file\n"), 0600); err != nil {
			t.Fatal(err)
		}

		env, err := resolveCLIEnv("myapp", "from_flag")
		if err != nil {
			t.Fatalf("resolve flag env error = %v", err)
		}
		if env != "from_flag" {
			t.Fatalf("env = %q, want from_flag", env)
		}

		env, err = resolveCLIEnv("myapp", "")
		if err != nil {
			t.Fatalf("resolve file env error = %v", err)
		}
		if env != "from_file" {
			t.Fatalf("env = %q, want from_file", env)
		}
	})

	t.Run("user config beats env file", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("BEACON_HOME", home)
		if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("projects:\n  - id: myapp\n    config_path: /tmp/monitor.yml\n    env: from_config\n"), 0600); err != nil {
			t.Fatal(err)
		}
		configDir := filepath.Join(home, "config", "projects", "myapp")
		if err := os.MkdirAll(configDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(configDir, "env"), []byte("BEACON_PROJECT_ENV=from_file\n"), 0600); err != nil {
			t.Fatal(err)
		}

		env, err := resolveCLIEnv("myapp", "")
		if err != nil {
			t.Fatalf("resolve env error = %v", err)
		}
		if env != "from_config" {
			t.Fatalf("env = %q, want from_config", env)
		}
	})

	t.Run("default and invalid inputs", func(t *testing.T) {
		t.Setenv("BEACON_HOME", t.TempDir())
		env, err := resolveCLIEnv("myapp", "")
		if err != nil {
			t.Fatalf("resolve default env error = %v", err)
		}
		if env != "default" {
			t.Fatalf("env = %q, want default", env)
		}
		if _, err := resolveCLIEnv("bad/project", ""); err == nil {
			t.Fatal("resolve invalid project error = nil, want error")
		}
		if _, err := resolveCLIEnv("myapp", "bad/env"); err == nil {
			t.Fatal("resolve invalid env error = nil, want error")
		}
	})
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "", want: "''"},
		{name: "plain", value: "secret", want: "'secret'"},
		{name: "single quote", value: "it'is", want: "'it'\\''is'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shellQuote(tt.value); got != tt.want {
				t.Fatalf("shellQuote(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
