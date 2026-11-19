package cloud

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// NormalizeGatewayURL accepts an HTTPS origin or API base. HTTP is restricted to
// loopback development. Credentials, queries and fragments must not enter URLs
// that will also be used for credential-bearing HTTP and WebSocket requests.
func NormalizeGatewayURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || u.Opaque != "" {
		return "", fmt.Errorf("gateway must be an absolute HTTPS URL")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", fmt.Errorf("gateway must not contain credentials, a query, or a fragment")
	}
	ip := net.ParseIP(u.Hostname())
	loopback := strings.EqualFold(u.Hostname(), "localhost") || (ip != nil && ip.IsLoopback())
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return "", fmt.Errorf("gateway requires HTTPS (HTTP is allowed only on loopback)")
	}
	path := strings.TrimRight(u.Path, "/")
	if path != "" && path != "/api" {
		return "", fmt.Errorf("gateway path must be empty or /api")
	}
	u.Path = "/api"
	u.RawPath = ""
	return strings.TrimRight(u.String(), "/"), nil
}

// RejectGatewayRedirect prevents forwarding credentials to a redirected server.
func RejectGatewayRedirect(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
