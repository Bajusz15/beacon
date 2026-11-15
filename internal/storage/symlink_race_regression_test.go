package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// A NAS writer can replace an ancestor directory after path validation. Exercise
// all read operations; errors during replacement are fine, outside data is not.
func TestStorageRegressionConcurrentSymlinkReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unprivileged symlink creation")
	}
	for _, operation := range []string{"open", "list", "metadata"} {
		t.Run(operation, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			slot, parked := filepath.Join(root, "slot"), filepath.Join(root, "parked")
			require.NoError(t, os.Mkdir(slot, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(slot, "file.txt"), []byte("inside"), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(outside, "file.txt"), []byte("outside-secret"), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(outside, "outside-only.txt"), []byte("secret"), 0600))
			svc := NewService(testStorageConfig(root))
			check := func() (bool, error) {
				switch operation {
				case "open":
					resp, err := svc.Open("media", "slot/file.txt")
					if err != nil {
						return false, nil
					}
					body, err := io.ReadAll(resp.File)
					_ = resp.File.Close()
					if err != nil {
						return false, err
					}
					if string(body) != "inside" {
						return true, fmt.Errorf("opened outside data: %q", body)
					}
				case "list":
					resp, err := svc.List("media", "slot")
					if err != nil {
						return false, nil
					}
					for _, entry := range resp.Entries {
						if entry.Name != "file.txt" || entry.Size != int64(len("inside")) {
							return true, fmt.Errorf("listed outside metadata: %+v", entry)
						}
					}
				case "metadata":
					resp, err := svc.Metadata("media", "slot/file.txt")
					if err != nil {
						return false, nil
					}
					if resp.Entry.Size != int64(len("inside")) {
						return true, fmt.Errorf("returned outside metadata: %+v", resp.Entry)
					}
				}
				return true, nil
			}
			ok, err := check()
			require.NoError(t, err)
			require.True(t, ok, "in-root operation must work before the race")

			stop, done := make(chan struct{}), make(chan error, 1)
			go func() {
				for {
					select {
					case <-stop:
						done <- nil
						return
					default:
					}
					if err := os.Rename(slot, parked); err != nil {
						done <- err
						return
					}
					if err := os.Symlink(outside, slot); err != nil {
						done <- err
						return
					}
					runtime.Gosched()
					if err := os.Remove(slot); err != nil {
						done <- err
						return
					}
					if err := os.Rename(parked, slot); err != nil {
						done <- err
						return
					}
					runtime.Gosched()
				}
			}()
			defer func() {
				close(stop)
				require.NoError(t, <-done, "directory replacement worker")
			}()
			for range 4000 {
				_, err := check()
				require.NoError(t, err)
			}
		})
	}
}
