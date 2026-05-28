package secrets

import (
	"bytes"
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
}
