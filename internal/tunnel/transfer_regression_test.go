package tunnel

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTunnelRegressionResponseLimit(t *testing.T) {
	const limit = 10 << 20
	for _, chunked := range []bool{false, true} {
		for _, size := range []int{limit - 1, limit, limit + 1, 11 << 20} {
			t.Run(fmt.Sprintf("chunked=%t/size=%d", chunked, size), func(t *testing.T) {
				data := bytes.Repeat([]byte("x"), size)
				data[len(data)-1] = '!'
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if chunked {
						w.(http.Flusher).Flush()
					} else {
						w.Header().Set("Content-Length", strconv.Itoa(len(data)))
					}
					_, _ = w.Write(data)
				}))
				defer srv.Close()
				resp, err := ProxyHTTPRequest(regressionDialTarget(t, srv.URL), &Message{
					Type: MsgHTTPRequest, RequestID: "size-boundary", Method: http.MethodGet, Path: "/download",
				})
				if size > limit && err != nil {
					return // Explicit rejection is allowed until streaming is supported.
				}
				require.NoError(t, err)
				require.NotNil(t, resp)
				require.Equal(t, MsgHTTPResponse, resp.Type)
				require.Equal(t, "size-boundary", resp.RequestID)
				if size > limit && resp.Status >= 400 && resp.Status <= 599 {
					require.True(t, resp.Error != "" || resp.Body != "", "rejection must explain the failure")
					return
				}
				require.Equal(t, http.StatusOK, resp.Status)
				body, err := base64.StdEncoding.DecodeString(resp.Body)
				require.NoError(t, err)
				require.Equal(t, size, len(body), "successful responses must never silently truncate")
				require.Equal(t, sha256.Sum256(data), sha256.Sum256(body))
			})
		}
	}
}

func TestTunnelRegressionLargeFileRangesAndHead(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789abcdef"), (11<<20)/16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "large.bin", time.Time{}, bytes.NewReader(data))
	}))
	defer srv.Close()
	dial := regressionDialTarget(t, srv.URL)

	head, err := ProxyHTTPRequest(dial, &Message{Method: http.MethodHead, Path: "/download"})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, head.Status)
	require.Empty(t, head.Body)
	require.Equal(t, strconv.Itoa(len(data)), head.Headers["Content-Length"])

	var downloaded []byte
	const chunkSize = 4 << 20
	for start := 0; start < len(data); start += chunkSize {
		end := min(start+chunkSize, len(data)) - 1
		resp, err := ProxyHTTPRequest(dial, &Message{
			Method: http.MethodGet, Path: "/download",
			Headers: map[string]string{"Range": fmt.Sprintf("bytes=%d-%d", start, end)},
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusPartialContent, resp.Status)
		require.Equal(t, fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)), resp.Headers["Content-Range"])
		body, err := base64.StdEncoding.DecodeString(resp.Body)
		require.NoError(t, err)
		require.Equal(t, end-start+1, len(body))
		downloaded = append(downloaded, body...)
	}
	require.Equal(t, sha256.Sum256(data), sha256.Sum256(downloaded))
}

func regressionDialTarget(t *testing.T, rawURL string) DialTarget {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	return DialTarget{Protocol: u.Scheme, Host: u.Hostname(), Port: port}
}
