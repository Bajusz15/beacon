package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"beacon/internal/config"
)

func TestDeployCommandOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BEACON_HOME", filepath.Join(home, ".beacon"))

	cfg := &config.Config{
		ProjectName:   "myapp",
		LocalPath:     filepath.Join(home, "app"),
		DeployCommand: "echo hello",
	}
	stdout, stderr, closeLog := deployCommandOutput(cfg, cfg.DeployCommand)
	if _, err := fmt.Fprintln(stdout, "stdout line"); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(stderr, "stderr line"); err != nil {
		t.Fatal(err)
	}
	closeLog()

	data, err := os.ReadFile(filepath.Join(home, ".beacon", "logs", "myapp", "deploy.log"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"deploy command: echo hello", "stdout line", "stderr line"} {
		if !strings.Contains(text, want) {
			t.Fatalf("deploy log missing %q: %s", want, text)
		}
	}
}
