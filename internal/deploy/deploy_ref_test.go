package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"beacon/internal/config"
	"beacon/internal/state"
)

func TestDeployRefDeploysSpecificCommit(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", ".")
	git(t, repo, "config", "user.email", "test@example.com")
	git(t, repo, "config", "user.name", "Beacon Test")

	requireWriteFile(t, filepath.Join(repo, "version.txt"), "first\n")
	git(t, repo, "add", "version.txt")
	git(t, repo, "commit", "-m", "first")
	first := gitOutput(t, repo, "rev-parse", "HEAD")

	requireWriteFile(t, filepath.Join(repo, "version.txt"), "second\n")
	git(t, repo, "add", "version.txt")
	git(t, repo, "commit", "-m", "second")

	home := t.TempDir()
	t.Setenv("HOME", home)
	localPath := filepath.Join(t.TempDir(), "checkout")
	status := state.NewStatus(filepath.Join(home, ".beacon", "test-project"))

	err := DeployRef(&config.Config{
		RepoURL:        repo,
		LocalPath:      localPath,
		ProjectDir:     "test-project",
		ProjectName:    "test-project",
		DeployCommand:  "test -f version.txt",
		SecureEnvPath:  "",
		DeploymentType: "git",
	}, first, status)
	if err != nil {
		t.Fatalf("DeployRef() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(localPath, "version.txt"))
	if err != nil {
		t.Fatalf("ReadFile deployed version: %v", err)
	}
	if string(data) != "first\n" {
		t.Fatalf("deployed content = %q, want first commit", string(data))
	}

	last, deployed := status.Get()
	if last != first {
		t.Fatalf("status last tag/ref = %q, want %q", last, first)
	}
	if deployed.IsZero() {
		t.Fatal("status deployed time was not recorded")
	}
}

func TestDeployRefRejectsUnsafeRefs(t *testing.T) {
	for _, ref := range []string{"", "-main", "main..other", "main@{1}", "main;rm"} {
		if err := validateGitRef(ref); err == nil {
			t.Fatalf("validateGitRef(%q) returned nil", ref)
		}
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func requireWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
