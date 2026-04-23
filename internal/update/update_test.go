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

func TestBinaryAssetName(t *testing.T) {
	name := binaryAssetName()
	require.Contains(t, name, "beacon-")
	require.Contains(t, name, runtime.GOOS)
	require.Contains(t, name, runtime.GOARCH)
}

func TestCheckLatest_newerVersion(t *testing.T) {
	setTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name":"v99.0.0","name":"v99.0.0","assets":[{"name":"beacon-linux_amd64","browser_download_url":"http://example.com/dl"}]}`)
	}))

	info, err := CheckLatest(context.Background())
	require.NoError(t, err)
	require.True(t, info.IsNewer)
	require.Equal(t, "v99.0.0", info.Tag)
	require.NotEmpty(t, info.AssetName)
}

func TestCheckLatest_sameVersion(t *testing.T) {
	setTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name":"v0.0.0-dev","name":"dev","assets":[]}`)
	}))

	info, err := CheckLatest(context.Background())
	require.NoError(t, err)
	// "dev" version is always considered outdated (IsNewer=true) to allow manual testing
	require.True(t, info.IsNewer)
}

func TestCheckLatest_notFound(t *testing.T) {
	setTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := CheckLatest(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no releases found")
}

func TestCheckLatest_serverError(t *testing.T) {
	setTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	_, err := CheckLatest(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "500")
}

func TestCheckLatest_invalidJSON(t *testing.T) {
	setTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{invalid`)
	}))

	_, err := CheckLatest(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode release")
}

func TestCheckLatest_setsAcceptHeader(t *testing.T) {
	setTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name":"v1.0.0","assets":[]}`)
	}))

	_, err := CheckLatest(context.Background())
	require.NoError(t, err)
}

func TestFetchExpectedHash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "abc123def456  beacon-linux_amd64")
		fmt.Fprintln(w, "789012fed345  beacon-darwin_arm64")
	}))
	defer srv.Close()

	hash, err := fetchExpectedHash(context.Background(), srv.URL, "beacon-linux_amd64")
	require.NoError(t, err)
	require.Equal(t, "abc123def456", hash)

	hash, err = fetchExpectedHash(context.Background(), srv.URL, "beacon-darwin_arm64")
	require.NoError(t, err)
	require.Equal(t, "789012fed345", hash)

	_, err = fetchExpectedHash(context.Background(), srv.URL, "beacon-windows_amd64")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no checksum")
}

func TestDownloadAndReplace_missingAsset(t *testing.T) {
	setTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name":"v2.0.0","assets":[{"name":"beacon-other_arch","browser_download_url":"http://example.com/dl"}]}`)
	}))

	err := DownloadAndReplace(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no asset")
	require.Contains(t, err.Error(), binaryAssetName())
}

func TestDownloadAndReplace_checksumMismatch(t *testing.T) {
	binaryContent := []byte("fake binary content")
	wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"
	assetName := binaryAssetName()

	mux := http.NewServeMux()
	var srvURL string

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name":"v2.0.0","assets":[{"name":"%s","browser_download_url":"%s/binary"},{"name":"SHA256SUMS.txt","browser_download_url":"%s/sums"}]}`,
			assetName, srvURL, srvURL)
	})
	mux.HandleFunc("/binary", func(w http.ResponseWriter, r *http.Request) {
		w.Write(binaryContent)
	})
	mux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", wrongHash, assetName)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	old := releaseURL
	releaseURL = srv.URL
	defer func() { releaseURL = old }()

	err := DownloadAndReplace(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "checksum mismatch")
}

func TestDownloadAndReplace_checksumMatch(t *testing.T) {
	binaryContent := []byte("valid binary content for test")
	h := sha256.Sum256(binaryContent)
	correctHash := hex.EncodeToString(h[:])
	assetName := binaryAssetName()

	mux := http.NewServeMux()
	var srvURL string

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name":"v2.0.0","assets":[{"name":"%s","browser_download_url":"%s/binary"},{"name":"SHA256SUMS.txt","browser_download_url":"%s/sums"}]}`,
			assetName, srvURL, srvURL)
	})
	mux.HandleFunc("/binary", func(w http.ResponseWriter, r *http.Request) {
		w.Write(binaryContent)
	})
	mux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", correctHash, assetName)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	old := releaseURL
	releaseURL = srv.URL
	defer func() { releaseURL = old }()

	err := DownloadAndReplace(context.Background())
	// Will fail on rename (replacing the test binary itself) but that's expected —
	// the checksum verification and download succeeded.
	if err != nil {
		require.Contains(t, err.Error(), "replace binary")
	}
}

func TestDownloadAndReplace_noChecksumsFile(t *testing.T) {
	binaryContent := []byte("binary without checksums")
	assetName := binaryAssetName()

	mux := http.NewServeMux()
	var srvURL string

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name":"v2.0.0","assets":[{"name":"%s","browser_download_url":"%s/binary"}]}`,
			assetName, srvURL)
	})
	mux.HandleFunc("/binary", func(w http.ResponseWriter, r *http.Request) {
		w.Write(binaryContent)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	old := releaseURL
	releaseURL = srv.URL
	defer func() { releaseURL = old }()

	err := DownloadAndReplace(context.Background())
	// Without checksums, download proceeds but rename will fail (expected).
	if err != nil {
		require.Contains(t, err.Error(), "replace binary")
	}
}

func TestDownloadAndReplace_emptyBody(t *testing.T) {
	assetName := binaryAssetName()

	mux := http.NewServeMux()
	var srvURL string

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name":"v2.0.0","assets":[{"name":"%s","browser_download_url":"%s/binary"}]}`,
			assetName, srvURL)
	})
	mux.HandleFunc("/binary", func(w http.ResponseWriter, r *http.Request) {
		// Return empty body
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	old := releaseURL
	releaseURL = srv.URL
	defer func() { releaseURL = old }()

	err := DownloadAndReplace(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty")
}

func TestDownloadAndReplace_downloadHTTPError(t *testing.T) {
	assetName := binaryAssetName()

	mux := http.NewServeMux()
	var srvURL string

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name":"v2.0.0","assets":[{"name":"%s","browser_download_url":"%s/binary"}]}`,
			assetName, srvURL)
	})
	mux.HandleFunc("/binary", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	old := releaseURL
	releaseURL = srv.URL
	defer func() { releaseURL = old }()

	err := DownloadAndReplace(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "403")
}
