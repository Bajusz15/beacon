package tunnel

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
)

func validHTTPMethod(m string) bool {
	switch strings.ToUpper(strings.TrimSpace(m)) {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return true
	default:
		return false
	}
}

// parseTunnelPath validates path+query from the cloud (SSRF protection on the path side).
func parseTunnelPath(pathAndQuery string) (pathPart, rawQuery string, err error) {
	p := strings.TrimSpace(pathAndQuery)
	if p == "" {
		p = "/"
	}
	if strings.ContainsAny(p, "\r\n\x00\\") {
		return "", "", fmt.Errorf("invalid path")
	}
	if strings.Contains(p, "://") || strings.HasPrefix(p, "//") {
		return "", "", fmt.Errorf("invalid path")
	}
	if strings.Contains(p, "@") {
		return "", "", fmt.Errorf("invalid path")
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	pathPart = p
	rawQuery = ""
	if i := strings.IndexByte(p, '?'); i >= 0 {
		pathPart = p[:i]
		rawQuery = p[i+1:]
	}
	pathPart = path.Clean(pathPart)
	if !strings.HasPrefix(pathPart, "/") {
		return "", "", fmt.Errorf("invalid path")
	}
	return pathPart, rawQuery, nil
}

// buildLoopbackURL builds a URL whose host is always 127.0.0.1. pathAndQuery must be a path
// and optional query only (no scheme/host), so remote-controlled strings cannot redirect the
// request off localhost (SSRF / URL parser confusion).
func buildLoopbackURL(scheme string, port int, pathAndQuery string) (*url.URL, error) {
	switch scheme {
	case "http", "https", "ws", "wss":
	default:
		return nil, fmt.Errorf("unsupported scheme")
	}
	pathPart, rawQuery, err := parseTunnelPath(pathAndQuery)
	if err != nil {
		return nil, err
	}
	u := &url.URL{
		Scheme:   scheme,
		Host:     net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		Path:     pathPart,
		RawQuery: rawQuery,
	}
	return u, nil
}

// buildUpstreamURL builds http(s) or ws(s) URL to a fixed host:port from local tunnel config.
func buildUpstreamURL(scheme, host string, port int, pathAndQuery string) (*url.URL, error) {
	switch scheme {
	case "http", "https", "ws", "wss":
	default:
		return nil, fmt.Errorf("unsupported scheme")
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, fmt.Errorf("empty host")
	}
	pathPart, rawQuery, err := parseTunnelPath(pathAndQuery)
	if err != nil {
		return nil, err
	}
	u := &url.URL{
		Scheme:   scheme,
		Host:     net.JoinHostPort(host, strconv.Itoa(port)),
		Path:     pathPart,
		RawQuery: rawQuery,
	}
	return u, nil
}
