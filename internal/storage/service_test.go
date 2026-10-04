package storage

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"beacon/internal/identity"

	"github.com/stretchr/testify/require"
)

func TestServiceListAndMetadata(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "photos"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "readme.txt"), []byte("hello"), 0600))

	svc := NewService(testStorageConfig(root))
	shares, err := svc.Shares()
	require.NoError(t, err)
	require.Equal(t, []Share{{ID: "media", Name: "Media", ReadOnly: true}}, shares)

	list, err := svc.List("media", "")
	require.NoError(t, err)
	require.Len(t, list.Entries, 2)
	require.Equal(t, "photos", list.Entries[0].Name)
	require.Equal(t, "directory", list.Entries[0].Type)
	require.Equal(t, "readme.txt", list.Entries[1].Name)
	require.Equal(t, "text/plain; charset=utf-8", list.Entries[1].ContentType)

	meta, err := svc.Metadata("media", "readme.txt")
	require.NoError(t, err)
	require.Equal(t, int64(5), meta.Entry.Size)
	require.Equal(t, "file", meta.Entry.Type)
}

func TestServiceOpen(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "movie.bin"), []byte("0123456789"), 0600))

	resp, err := NewService(testStorageConfig(root)).Open("media", "movie.bin")
	require.NoError(t, err)
	defer func() { _ = resp.File.Close() }()

	data, err := io.ReadAll(resp.File)
	require.NoError(t, err)
	require.Equal(t, "0123456789", string(data))
	require.Equal(t, "movie.bin", resp.Name)
	require.Equal(t, "application/octet-stream", resp.ContentType)
}

func TestServiceRejectsTraversalPayloads(t *testing.T) {
	root := t.TempDir()
	svc := NewService(testStorageConfig(root))

	badPaths := []string{
		"../secret.txt",
		"photos/../../secret.txt",
		"/absolute",
		"photos//nested",
		"photos/./nested",
		`photos\nested`,
		"photos\x00nested",
	}
	for _, p := range badPaths {
		t.Run(p, func(t *testing.T) {
			_, err := svc.Metadata("media", p)
			require.ErrorIs(t, err, ErrForbidden)
		})
	}
}

func TestServiceRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0600))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "escape.txt")))

	_, err := NewService(testStorageConfig(root)).Open("media", "escape.txt")
	require.ErrorIs(t, err, ErrForbidden)
}

func TestServiceAllowsSymlinkWithinShare(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on windows")
	}
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "safe"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "safe", "file.txt"), []byte("ok"), 0600))
	require.NoError(t, os.Symlink(filepath.Join(root, "safe", "file.txt"), filepath.Join(root, "alias.txt")))

	resp, err := NewService(testStorageConfig(root)).Open("media", "alias.txt")
	require.NoError(t, err)
	defer func() { _ = resp.File.Close() }()
}

func TestServiceDisabledAndMissingShares(t *testing.T) {
	root := t.TempDir()
	disabled := false
	svc := NewService(&identity.StorageConfig{
		Enabled: true,
		Shares:  []identity.StorageShareConfig{{ID: "off", Root: root, Enabled: &disabled}},
	})

	_, err := svc.List("missing", "")
	require.ErrorIs(t, err, ErrShareNotFound)
	_, err = svc.List("off", "")
	require.ErrorIs(t, err, ErrShareNotFound)

	_, err = NewService(nil).Shares()
	require.ErrorIs(t, err, ErrNotConfigured)
}

func TestServiceDirectoryDownloadRejected(t *testing.T) {
	_, err := NewService(testStorageConfig(t.TempDir())).Open("media", "")
	require.True(t, errors.Is(err, ErrIsDirectory), "err = %v", err)
}

func testStorageConfig(root string) *identity.StorageConfig {
	return &identity.StorageConfig{
		Enabled: true,
		Shares: []identity.StorageShareConfig{
			{ID: "media", Name: "Media", Root: root},
		},
	}
}
