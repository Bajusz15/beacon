package master

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"beacon/internal/audit"
	"beacon/internal/identity"
	"beacon/internal/tunnel"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func storageRegressionServer(t *testing.T) (*StatusServer, *identity.UserConfig, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("BEACON_HOME", home)
	audit.SetPathForTesting(filepath.Join(home, "audit.jsonl"))
	t.Cleanup(func() { audit.SetPathForTesting("") })
	root := t.TempDir()
	cfg := &identity.UserConfig{Storage: &identity.StorageConfig{
		Enabled: true,
		Shares:  []identity.StorageShareConfig{{ID: "media", Root: root}},
	}}
	require.NoError(t, cfg.Save())
	return NewStatusServer(NewStatusCache(nil, nil, cfg), 0), cfg, root
}

func TestStorageRegressionDownloadHeaders(t *testing.T) {
	srv, _, root := storageRegressionServer(t)
	for _, name := range []string{"page.html", "script.js", "image.svg", `quoted "file".html`} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("<script>alert(1)</script>"), 0600))
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			for _, ranged := range []bool{false, true} {
				t.Run(name+"/"+method+map[bool]string{false: "/full", true: "/range"}[ranged], func(t *testing.T) {
					req := httptest.NewRequest(method, "/api/storage/shares/media/download?path="+url.QueryEscape(name), nil)
					wantStatus := http.StatusOK
					if ranged {
						req.Header.Set("Range", "bytes=0-5")
						wantStatus = http.StatusPartialContent
					}
					rr := httptest.NewRecorder()
					srv.handleAPIStorageShare(rr, req)
					require.Equal(t, wantStatus, rr.Code)
					assert.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
					disposition, params, err := mime.ParseMediaType(rr.Header().Get("Content-Disposition"))
					require.NoError(t, err)
					require.Equal(t, "attachment", disposition)
					require.Equal(t, name, params["filename"])
					if method == http.MethodHead {
						require.Empty(t, rr.Body.Bytes())
					}
				})
			}
		}
	}
}

func TestStorageRegressionDisableWithoutCloud(t *testing.T) {
	for _, cloudEnabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "cloud_disabled", true: "missing_api_key"}[cloudEnabled], func(t *testing.T) {
			srv, cfg, root := storageRegressionServer(t)
			cfg.CloudReportingEnabled = cloudEnabled
			require.NoError(t, cfg.Save())
			require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("private"), 0600))
			request := func(action string) *httptest.ResponseRecorder {
				rr := httptest.NewRecorder()
				path := "file.txt"
				if action == "list" {
					path = ""
				}
				srv.handleAPIStorageShare(rr, httptest.NewRequest(http.MethodGet,
					"/api/storage/shares/media/"+action+"?path="+path, nil))
				return rr
			}
			require.Equal(t, http.StatusOK, request("download").Code)
			require.NoError(t, identity.SetStorageShareEnabled("media", false))
			beat := &heartbeatLoop{ctx: context.Background(), statusCache: srv.cache,
				dispatcher: NewCommandDispatcher(nil, nil)}
			beat.tryBeat()
			// Exercise both periodic refresh paths without manually updating the cache.
			srv.cache.Refresh()
			for _, action := range []string{"download", "metadata", "list"} {
				rr := request(action)
				require.Equal(t, http.StatusNotFound, rr.Code, "%s still accessible: %s", action, rr.Body.String())
			}
			require.NoError(t, identity.SetStorageShareEnabled("media", true))
			beat.tryBeat()
			srv.cache.Refresh()
			require.Equal(t, http.StatusOK, request("download").Code, "re-enabling must also reload")
		})
	}
}

func TestStorageRegressionDownloadThroughTunnel(t *testing.T) {
	srv, _, root := storageRegressionServer(t)
	data := bytes.Repeat([]byte("0123456789abcdef"), (11<<20)/16)
	require.NoError(t, os.WriteFile(filepath.Join(root, "large.bin"), data, 0600))
	upstream := httptest.NewServer(http.HandlerFunc(srv.handleAPIStorageShare))
	defer upstream.Close()
	u, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	dial := tunnel.DialTarget{Protocol: u.Scheme, Host: u.Hostname(), Port: port}
	for _, ranged := range []bool{false, true} {
		t.Run(map[bool]string{false: "full", true: "range"}[ranged], func(t *testing.T) {
			msg := &tunnel.Message{Method: http.MethodGet, Path: "/api/storage/shares/media/download?path=large.bin"}
			wantStatus, wantBody := http.StatusOK, data
			if ranged {
				msg.Headers = map[string]string{"Range": "bytes=10485760-11534335"}
				wantStatus, wantBody = http.StatusPartialContent, data[10<<20:]
			}
			resp, err := tunnel.ProxyHTTPRequest(dial, msg)
			if !ranged && err != nil {
				return // Full delivery or explicit refusal are both safe outcomes.
			}
			require.NoError(t, err)
			require.NotNil(t, resp)
			if !ranged && resp.Status >= 400 && resp.Status <= 599 {
				require.True(t, resp.Error != "" || resp.Body != "", "rejection must explain the failure")
				return
			}
			require.Equal(t, wantStatus, resp.Status)
			body, err := base64.StdEncoding.DecodeString(resp.Body)
			require.NoError(t, err)
			require.Equal(t, len(wantBody), len(body), "storage download was silently truncated by the tunnel")
			require.Equal(t, sha256.Sum256(wantBody), sha256.Sum256(body))
			if ranged {
				require.Equal(t, "bytes 10485760-11534335/11534336", resp.Headers["Content-Range"])
			}
		})
	}
}
