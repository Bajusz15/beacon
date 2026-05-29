package backup

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"beacon/internal/identity"
)

func TestNewResticRunnerValidatesConfigBeforeBinary(t *testing.T) {
	_, err := newResticRunner(&identity.BackupConfig{Enabled: true})
	if err == nil || !strings.Contains(err.Error(), "at least one backup path") {
		t.Fatalf("expected paths validation error, got %v", err)
	}
}

func TestConfigEqualIncludesRetentionAndEnv(t *testing.T) {
	a := &identity.BackupConfig{
		Enabled:      true,
		Schedule:     "0 3 * * *",
		Paths:        []string{"/data"},
		Destination:  "/backup/beacon",
		PasswordFile: "/data/restic-password",
		Retention:    &identity.RetentionPolicy{KeepDaily: 7},
		Env:          map[string]string{"RESTIC_COMPRESSION": "auto"},
	}
	b := *a
	b.Retention = &identity.RetentionPolicy{KeepDaily: 14}
	if configEqual(a, &b) {
		t.Fatal("expected retention changes to affect config equality")
	}
	b = *a
	b.Env = map[string]string{"RESTIC_COMPRESSION": "max"}
	if configEqual(a, &b) {
		t.Fatal("expected env changes to affect config equality")
	}
}

func TestRunNowStartsAsynchronously(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test fixture is unix-only")
	}

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	fakeRestic := filepath.Join(binDir, "restic")
	if err := os.WriteFile(fakeRestic, []byte(`#!/bin/sh
case "$*" in
  *" backup"*) sleep 0.2 ;;
esac
if [ "$1" = "--repo" ] && [ "$5" = "--json" ]; then
  printf '[]'
fi
exit 0
`), 0755); err != nil {
		t.Fatal(err)
	}

	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)

	passwordFile := filepath.Join(dir, "password")
	if err := os.WriteFile(passwordFile, []byte("test-password\n"), 0600); err != nil {
		t.Fatal(err)
	}

	m := NewManager(nil)
	cfg := &identity.BackupConfig{
		Enabled:      true,
		Schedule:     "0 3 * * *",
		Paths:        []string{dir},
		Destination:  filepath.Join(dir, "repo"),
		PasswordFile: passwordFile,
	}
	if err := m.Reconcile(context.Background(), cfg); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	defer m.Stop()

	start := time.Now()
	if err := m.RunNow(context.Background()); err != nil {
		t.Fatalf("run now: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("RunNow blocked for %v", elapsed)
	}

	eventually(t, 5*time.Second, func() bool {
		return !m.Status().Running && !m.Status().LastRunAt.IsZero()
	})
}

func eventually(t *testing.T, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
