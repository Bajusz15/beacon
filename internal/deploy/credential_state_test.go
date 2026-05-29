package deploy

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCredentialErrorPath(t *testing.T) {
	t.Run("rejects unsafe project ids", func(t *testing.T) {
		stateDir := t.TempDir()
		unsafeProjectIDs := []string{
			"../other",
			"..",
			".",
			"/tmp/other",
			`nested/project`,
			`nested\project`,
			"project id",
		}

		for _, projectID := range unsafeProjectIDs {
			if _, err := credentialErrorPath(stateDir, projectID); err == nil {
				t.Fatalf("credentialErrorPath(%q) error = nil, want error", projectID)
			}
		}
	})

	t.Run("allows safe project id", func(t *testing.T) {
		stateDir := t.TempDir()
		path, err := credentialErrorPath(stateDir, "project_1-api.prod")
		if err != nil {
			t.Fatalf("credentialErrorPath() error = %v", err)
		}
		want := filepath.Join(stateDir, "project_1-api.prod", credentialErrorsFile)
		if path != want {
			t.Fatalf("credentialErrorPath() = %q, want %q", path, want)
		}
	})
}

func TestCredentialErrorsLifecycle(t *testing.T) {
	t.Run("record read replace and clear", func(t *testing.T) {
		stateDir := t.TempDir()
		authErr := &AuthError{Type: "git", Code: 401, Message: "bad token"}
		if err := RecordCredentialError(stateDir, "myapp", authErr); err != nil {
			t.Fatalf("RecordCredentialError() error = %v", err)
		}

		records := ReadCredentialErrors(stateDir, "myapp")
		if len(records) != 1 {
			t.Fatalf("records len = %d, want 1", len(records))
		}
		if records[0].Type != "git" || records[0].ErrorCode != 401 || records[0].Message != "bad token" {
			t.Fatalf("record = %#v, want persisted auth error", records[0])
		}

		authErr.Message = "new token error"
		if err := RecordCredentialError(stateDir, "myapp", authErr); err != nil {
			t.Fatalf("RecordCredentialError() replace error = %v", err)
		}
		records = ReadCredentialErrors(stateDir, "myapp")
		if len(records) != 1 || records[0].Message != "new token error" {
			t.Fatalf("records after replace = %#v", records)
		}

		ClearCredentialErrors(stateDir, "myapp")
		if records := ReadCredentialErrors(stateDir, "myapp"); len(records) != 0 {
			t.Fatalf("records after clear = %#v, want empty", records)
		}
	})

	t.Run("filters expired and invalid records", func(t *testing.T) {
		stateDir := t.TempDir()
		path, err := credentialErrorPath(stateDir, "myapp")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-credentialErrorMaxAge - time.Hour)
		fresh := time.Now()
		data := `[
  {"type":"git","message":"old","detected_at":"` + old.Format(time.RFC3339Nano) + `"},
  {"type":"docker","message":"fresh","detected_at":"` + fresh.Format(time.RFC3339Nano) + `"}
]`
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		records := ReadCredentialErrors(stateDir, "myapp")
		if len(records) != 1 || records[0].Type != "docker" {
			t.Fatalf("records = %#v, want only fresh docker record", records)
		}

		if err := os.WriteFile(path, []byte("not-json"), 0600); err != nil {
			t.Fatal(err)
		}
		if records := ReadCredentialErrors(stateDir, "myapp"); len(records) != 0 {
			t.Fatalf("invalid json records = %#v, want empty", records)
		}
	})

	t.Run("record rejects unsafe project id", func(t *testing.T) {
		err := RecordCredentialError(t.TempDir(), "../bad", &AuthError{Type: "git", Message: "bad"})
		if err == nil {
			t.Fatal("RecordCredentialError() error = nil, want unsafe project id error")
		}
	})
}

func TestClearCredentialErrors(t *testing.T) {
	t.Run("rejects traversal", func(t *testing.T) {
		stateDir := t.TempDir()
		outsideDir := t.TempDir()
		outsideFile := filepath.Join(outsideDir, credentialErrorsFile)
		if err := os.WriteFile(outsideFile, []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}

		ClearCredentialErrors(stateDir, filepath.Join("..", filepath.Base(outsideDir)))

		if _, err := os.Stat(outsideFile); err != nil {
			t.Fatalf("outside file was affected: %v", err)
		}
	})

	t.Run("ignores missing file", func(t *testing.T) {
		ClearCredentialErrors(t.TempDir(), "myapp")
	})

	t.Run("handles remove error", func(t *testing.T) {
		stateDir := t.TempDir()
		path, err := credentialErrorPath(stateDir, "myapp")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		ClearCredentialErrors(stateDir, "myapp")
		if _, err := os.Stat(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stat path after failed clear: %v", err)
		}
	})
}
