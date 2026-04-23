package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"beacon/internal/version"

	"github.com/stretchr/testify/require"
)

func setTestServer(t *testing.T, handler http.Handler) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	old := releaseURL
	releaseURL = srv.URL
	t.Cleanup(func() { releaseURL = old })
}

func overrideVersion(t *testing.T, v string) {
	t.Helper()
	old := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = old })
}

// downloadServer sets up a mock GitHub release server with a binary asset,
// optional SHA256SUMS, and configurable responses for each path.
func downloadServer(t *testing.T, binary []byte, checksumHash string) {
	t.Helper()
	assetName := binaryAssetName()
	mux := http.NewServeMux()
	var srvURL string

	assets := fmt.Sprintf(`{"name":"%s","browser_download_url":"SRVURL/binary"}`, assetName)
	if checksumHash != "" {
		assets += `,{"name":"SHA256SUMS.txt","browser_download_url":"SRVURL/sums"}`
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := fmt.Sprintf(`{"tag_name":"v2.0.0","assets":[%s]}`, assets)
		// Replace placeholder with actual server URL
		fmt.Fprint(w, resp)
	})
	mux.HandleFunc("/binary", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(binary)
	})
	if checksumHash != "" {
		mux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "%s  %s\n", checksumHash, assetName)
		})
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Rewrite asset URLs to point to actual test server
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	srvURL = srv.URL

	// Now we need to re-register with the correct URLs embedded in JSON.
	// Easier: just override releaseURL and fix the JSON inline.
	realMux := http.NewServeMux()
	realMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		a := fmt.Sprintf(`{"name":"%s","browser_download_url":"%s/binary"}`, assetName, srvURL)
		if checksumHash != "" {
			a += fmt.Sprintf(`,{"name":"SHA256SUMS.txt","browser_download_url":"%s/sums"}`, srvURL)
		}
		fmt.Fprintf(w, `{"tag_name":"v2.0.0","assets":[%s]}`, a)
	})
	realMux.HandleFunc("/binary", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(binary)
	})
	if checksumHash != "" {
		realMux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "%s  %s\n", checksumHash, assetName)
		})
	}
	srv.Config.Handler = realMux

	old := releaseURL
	releaseURL = srvURL
	t.Cleanup(func() { releaseURL = old })
}

func TestBinaryAssetName(t *testing.T) {
	name := binaryAssetName()
	require.Contains(t, name, "beacon-")
	require.Contains(t, name, runtime.GOOS)
	require.Contains(t, name, runtime.GOARCH)
}

func TestCheckLatest(t *testing.T) {
	t.Run("newer version", func(t *testing.T) {
		overrideVersion(t, "0.3.1")
		setTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"tag_name":"v99.0.0","assets":[]}`)
		}))

		info, err := CheckLatest(context.Background())
		require.NoError(t, err)
		require.True(t, info.IsNewer)
		require.Equal(t, "v99.0.0", info.Tag)
		require.NotEmpty(t, info.AssetName)
	})

	t.Run("same version", func(t *testing.T) {
		overrideVersion(t, "1.0.0")
		setTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"tag_name":"v1.0.0","assets":[]}`)
		}))

		info, err := CheckLatest(context.Background())
		require.NoError(t, err)
		require.False(t, info.IsNewer)
	})

	t.Run("dev version never triggers update", func(t *testing.T) {
		overrideVersion(t, "dev")
		setTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"tag_name":"v99.0.0","assets":[]}`)
		}))

		info, err := CheckLatest(context.Background())
		require.NoError(t, err)
		require.False(t, info.IsNewer)
	})

	t.Run("404 no releases", func(t *testing.T) {
		setTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		_, err := CheckLatest(context.Background())
		require.ErrorContains(t, err, "no releases found")
	})

	t.Run("500 server error", func(t *testing.T) {
		setTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		_, err := CheckLatest(context.Background())
		require.ErrorContains(t, err, "500")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		setTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{invalid`)
		}))
		_, err := CheckLatest(context.Background())
		require.ErrorContains(t, err, "decode release")
	})

	t.Run("sets Accept header", func(t *testing.T) {
		setTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"tag_name":"v1.0.0","assets":[]}`)
		}))
		_, err := CheckLatest(context.Background())
		require.NoError(t, err)
	})
}

func TestFetchExpectedHash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "abc123def456  beacon-linux_amd64")
		fmt.Fprintln(w, "789012fed345  beacon-darwin_arm64")
	}))
	defer srv.Close()

	t.Run("finds matching asset", func(t *testing.T) {
		hash, err := fetchExpectedHash(context.Background(), srv.URL, "beacon-linux_amd64")
		require.NoError(t, err)
		require.Equal(t, "abc123def456", hash)
	})

	t.Run("finds second asset", func(t *testing.T) {
		hash, err := fetchExpectedHash(context.Background(), srv.URL, "beacon-darwin_arm64")
		require.NoError(t, err)
		require.Equal(t, "789012fed345", hash)
	})

	t.Run("missing asset errors", func(t *testing.T) {
		_, err := fetchExpectedHash(context.Background(), srv.URL, "beacon-windows_amd64")
		require.ErrorContains(t, err, "no checksum")
	})
}

func TestDownloadAndReplace(t *testing.T) {
	t.Run("missing asset", func(t *testing.T) {
		setTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"tag_name":"v2.0.0","assets":[{"name":"beacon-other_arch","browser_download_url":"http://example.com/dl"}]}`)
		}))
		err := DownloadAndReplace(context.Background())
		require.ErrorContains(t, err, "no asset")
		require.ErrorContains(t, err, binaryAssetName())
	})

	t.Run("checksum mismatch", func(t *testing.T) {
		wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"
		downloadServer(t, []byte("fake binary"), wrongHash)
		err := DownloadAndReplace(context.Background())
		require.ErrorContains(t, err, "checksum mismatch")
	})

	t.Run("checksum match", func(t *testing.T) {
		content := []byte("valid binary content for test")
		h := sha256.Sum256(content)
		downloadServer(t, content, hex.EncodeToString(h[:]))

		err := DownloadAndReplace(context.Background())
		// Rename will fail (can't replace test binary) — but checksum passed.
		if err != nil {
			require.ErrorContains(t, err, "replace binary")
		}
	})

	t.Run("no checksums file", func(t *testing.T) {
		downloadServer(t, []byte("binary without checksums"), "")
		err := DownloadAndReplace(context.Background())
		if err != nil {
			require.ErrorContains(t, err, "replace binary")
		}
	})

	t.Run("empty body", func(t *testing.T) {
		downloadServer(t, nil, "")
		err := DownloadAndReplace(context.Background())
		require.ErrorContains(t, err, "empty")
	})

	t.Run("download HTTP error", func(t *testing.T) {
		assetName := binaryAssetName()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/binary" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"tag_name":"v2.0.0","assets":[{"name":"%s","browser_download_url":"%s/binary"}]}`,
				assetName, "http://"+r.Host)
		}))
		t.Cleanup(srv.Close)
		old := releaseURL
		releaseURL = srv.URL
		t.Cleanup(func() { releaseURL = old })

		err := DownloadAndReplace(context.Background())
		require.ErrorContains(t, err, "403")
	})
}
