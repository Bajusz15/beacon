package systemd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestEnsureLingering(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake loginctl uses a POSIX shell")
	}
	uid := strconv.Itoa(os.Geteuid())
	for _, tc := range []struct {
		name, mode string
		scope      ServiceType
		wantErr    bool
		wantCalls  []string
	}{
		{name: "system services need no lingering", scope: SystemService},
		{name: "already enabled", mode: "already", wantCalls: []string{"show-user " + uid + " --property=Linger --value"}},
		{name: "enable without sudo", mode: "direct", wantCalls: []string{"show-user " + uid + " --property=Linger --value", "--no-ask-password enable-linger " + uid, "show-user " + uid + " --property=Linger --value"}},
		{name: "passwordless sudo fallback", mode: "sudo", wantCalls: []string{"show-user " + uid + " --property=Linger --value", "--no-ask-password enable-linger " + uid, "sudo -n loginctl --no-ask-password enable-linger " + uid, "show-user " + uid + " --property=Linger --value"}},
		{name: "permission denied", mode: "denied", wantErr: true},
		{name: "successful command must actually enable lingering", mode: "nochange", wantErr: true},
		{name: "unregistered user can enable lingering", mode: "unregistered"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PATH", dir)
			t.Setenv("LINGER_TEST_DIR", dir)
			t.Setenv("LINGER_TEST_MODE", tc.mode)
			write := func(name, content string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0700); err != nil {
					t.Fatal(err)
				}
			}
			write("loginctl", `#!/bin/sh
printf '%s\n' "$*" >> "$LINGER_TEST_DIR/calls"
if [ "$1" = show-user ]; then
  if [ -f "$LINGER_TEST_DIR/enabled" ] || [ "$LINGER_TEST_MODE" = already ]; then
    printf 'yes\n'
  elif [ "$LINGER_TEST_MODE" = unregistered ]; then
    exit 1
  else
    printf 'no\n'
  fi
elif [ "$1" = --no-ask-password ] && [ "$2" = enable-linger ]; then
  case "$LINGER_TEST_MODE" in
    direct|unregistered) printf 'yes\n' > "$LINGER_TEST_DIR/enabled" ;;
    nochange) exit 0 ;;
    *) printf 'Access denied\n' >&2; exit 1 ;;
  esac
else
  exit 2
fi
`)
			write("sudo", `#!/bin/sh
printf 'sudo %s\n' "$*" >> "$LINGER_TEST_DIR/calls"
if [ "$LINGER_TEST_MODE" = sudo ]; then
  printf 'yes\n' > "$LINGER_TEST_DIR/enabled"
else
  printf 'a password is required\n' >&2
  exit 1
fi
`)
			err := NewServiceManager(tc.scope).EnsureLingering()
			if (err != nil) != tc.wantErr {
				t.Fatalf("EnsureLingering() = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "sudo loginctl enable-linger "+uid) {
				t.Fatalf("error lacks recovery command: %v", err)
			}
			data, readErr := os.ReadFile(filepath.Join(dir, "calls"))
			if tc.scope == SystemService {
				if !os.IsNotExist(readErr) {
					t.Fatalf("system service invoked a lingering command: %s (%v)", data, readErr)
				}
			} else if tc.wantCalls != nil && string(data) != strings.Join(tc.wantCalls, "\n")+"\n" {
				t.Fatalf("commands = %q, want %q", data, tc.wantCalls)
			}
			if tc.wantErr {
				// Both installation paths must stop before creating a unit if
				// login persistence cannot be established.
				t.Setenv("HOME", dir)
				sm := NewServiceManager(UserService)
				if err := sm.CreateMasterService("/usr/local/bin/beacon start --foreground", dir); err == nil {
					t.Fatal("master installation accepted disabled lingering")
				}
				if err := sm.CreateService(GetDefaultServiceConfig("test", "", dir)); err == nil {
					t.Fatal("project installation accepted disabled lingering")
				}
				if _, err := os.Stat(filepath.Join(dir, ".config", "systemd", "user")); !os.IsNotExist(err) {
					t.Fatal(fmt.Sprintf("failed installation created service directory: %v", err))
				}
			}
		})
	}
}
