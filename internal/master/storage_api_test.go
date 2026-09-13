package master

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"beacon/internal/audit"
	"beacon/internal/identity"

	"github.com/stretchr/testify/require"
)

func TestStorageAPI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BEACON_HOME", filepath.Join(home, ".beacon"))
	audit.SetPathForTesting(filepath.Join(home, ".beacon", "logs", "audit.jsonl"))
	t.Cleanup(func() { audit.SetPathForTesting("") })

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "clip.txt"), []byte("0123456789"), 0600))
	cache := NewStatusCache(nil, nil, &identity.UserConfig{
		Storage: &identity.StorageConfig{
			Enabled: true,
			Shares:  []identity.StorageShareConfig{{ID: "media", Name: "Media", Root: root}},
		},
	})
	server := NewStatusServer(cache, 0)

	t.Run("shares", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/storage/shares", nil)
		rr := httptest.NewRecorder()
		server.handleAPIStorageShares(rr, req)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		require.NotContains(t, rr.Body.String(), root)
		require.Contains(t, rr.Body.String(), `"id":"media"`)
	})

	t.Run("list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/storage/shares/media/list", nil)
		rr := httptest.NewRecorder()
		server.handleAPIStorageShare(rr, req)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		require.Contains(t, rr.Body.String(), `"name":"clip.txt"`)
	})

	t.Run("reject traversal", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/storage/shares/media/metadata?path=../secret.txt", nil)
		rr := httptest.NewRecorder()
		server.handleAPIStorageShare(rr, req)
		require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
	})

	t.Run("range download", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/storage/shares/media/download?path=clip.txt", nil)
		req.Header.Set("Range", "bytes=2-5")
		rr := httptest.NewRecorder()
		server.handleAPIStorageShare(rr, req)
		require.Equal(t, http.StatusPartialContent, rr.Code, rr.Body.String())
		require.Equal(t, "2345", rr.Body.String())
		require.Equal(t, "bytes 2-5/10", rr.Header().Get("Content-Range"))
	})

	t.Run("does not support write methods", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/storage/shares/media/download?path=clip.txt", strings.NewReader(""))
		rr := httptest.NewRecorder()
		server.handleAPIStorageShare(rr, req)
		require.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	var listed struct {
		Entries []audit.Entry `json:"entries"`
	}
	req := httptest.NewRequest(http.MethodGet, "/api/audit?action=storage_download", nil)
	rr := httptest.NewRecorder()
	server.handleAPIAudit(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &listed))
	require.NotEmpty(t, listed.Entries)
	require.NotContains(t, rr.Body.String(), "clip.txt")
}
