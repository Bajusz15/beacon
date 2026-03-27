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

// buildLoopbackURL builds a URL whose host is always 127.0.0.1. pathAndQuery must be a path
// and optional query only (no scheme/host), so remote-controlled strings cannot redirect the
// request off localhost (SSRF / URL parser confusion).
func buildLoopbackURL(scheme string, port int, pathAndQuery string) (*url.URL, error) {
	switch scheme {
	case "http", "https", "ws", "wss":
	default:
		return nil, fmt.Errorf("unsupported scheme")
	}
	p := strings.TrimSpace(pathAndQuery)
	if p == "" {
		p = "/"
	}
	if strings.ContainsAny(p, "\r\n\x00\\") {
		return nil, fmt.Errorf("invalid path")
	}
	// Block absolute URLs and scheme-relative URLs that could change the request target.
	if strings.Contains(p, "://") || strings.HasPrefix(p, "//") {
		return nil, fmt.Errorf("invalid path")
	}
	if strings.Contains(p, "@") {
		return nil, fmt.Errorf("invalid path")
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	pathPart := p
	rawQuery := ""
	if i := strings.IndexByte(p, '?'); i >= 0 {
		pathPart = p[:i]
		rawQuery = p[i+1:]
	}
	pathPart = path.Clean(pathPart)
	if !strings.HasPrefix(pathPart, "/") {
		return nil, fmt.Errorf("invalid path")
	}
	u := &url.URL{
		Scheme:   scheme,
		Host:     net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		Path:     pathPart,
		RawQuery: rawQuery,
	}
	return u, nil
}
