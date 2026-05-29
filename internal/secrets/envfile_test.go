package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadEnvFile(t *testing.T) {
	t.Run("parses and expands", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "env")
		content := `
# comment
PLAIN=value
DOUBLE="two words"
SINGLE='single words'
EXPANDED=${BASE}/suffix
MALFORMED
=empty-key
`
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}

		values, err := readEnvFile(path, []string{"BASE=/tmp/base"})
		if err != nil {
			t.Fatalf("readEnvFile() error = %v", err)
		}
		want := map[string]string{
			"PLAIN":    "value",
			"DOUBLE":   "two words",
			"SINGLE":   "single words",
			"EXPANDED": "/tmp/base/suffix",
		}
		for key, expected := range want {
			if values[key] != expected {
				t.Fatalf("%s = %q, want %q", key, values[key], expected)
			}
		}
		if _, ok := values["MALFORMED"]; ok {
			t.Fatal("malformed line was parsed")
		}
	})

	t.Run("missing file errors", func(t *testing.T) {
		if _, err := readEnvFile(filepath.Join(t.TempDir(), "missing"), nil); err == nil {
			t.Fatal("readEnvFile() error = nil, want missing file error")
		}
	})
}
