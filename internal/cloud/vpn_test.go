package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// recordedRequest captures what the test server saw, so each test can assert
// on the exact wire format the agent sends.
type recordedRequest struct {
	method  string
	path    string
	rawQ    string
	headers http.Header
	body    []byte
}

// newTestServer wires up a stub server with one handler and returns it +
// a *recordedRequest the handler will populate on each call.
func newTestServer(t *testing.T, status int, respBody string) (*httptest.Server, *recordedRequest) {
	t.Helper()
	rec := &recordedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rec.method = r.Method
		rec.path = r.URL.Path
		rec.rawQ = r.URL.RawQuery
		rec.headers = r.Header.Clone()
		rec.body = body
		w.WriteHeader(status)
		_, _ = io.WriteString(w, respBody)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func TestVPNClient_RegisterVPN_success(t *testing.T) {
	srv, rec := newTestServer(t, http.StatusOK, `{"vpn_address":"10.13.37.5"}`)

	c := NewVPNClient(srv.URL, "usr_test_key", "n100-pi")
	addr, err := c.RegisterVPN(context.Background(), "pubkey-base64", "exit_node", 51820, "203.0.113.1:51820")
	require.NoError(t, err)
	require.Equal(t, "10.13.37.5", addr)

	require.Equal(t, http.MethodPost, rec.method)
	require.Equal(t, "/agent/vpn/register", rec.path)
	require.Equal(t, "application/json", rec.headers.Get("Content-Type"))
	require.Equal(t, "Bearer usr_test_key", rec.headers.Get("Authorization"))
	require.Equal(t, "usr_test_key", rec.headers.Get("X-API-Key"))

	var sent vpnRegisterPayload
	require.NoError(t, json.Unmarshal(rec.body, &sent))
	require.Equal(t, "n100-pi", sent.DeviceName)
	require.Equal(t, "pubkey-base64", sent.PublicKey)
	require.Equal(t, "exit_node", sent.Role)
	require.Equal(t, 51820, sent.ListenPort)
	require.Equal(t, "203.0.113.1:51820", sent.Endpoint)
}

func TestVPNClient_RegisterVPN_serverError(t *testing.T) {
	srv, _ := newTestServer(t, http.StatusInternalServerError, `{"error":"db down"}`)

	c := NewVPNClient(srv.URL, "usr_test_key", "n100-pi")
	addr, err := c.RegisterVPN(context.Background(), "pk", "exit_node", 51820, "")
	require.Error(t, err)
	require.Empty(t, addr)
	require.Contains(t, err.Error(), "HTTP 500")
	require.Contains(t, err.Error(), "db down")
}

func TestVPNClient_GetPeer_success(t *testing.T) {
	srv, rec := newTestServer(t, http.StatusOK, `{
		"device_name": "home-pi",
		"public_key": "peer-pubkey",
		"endpoint": "203.0.113.1:51820",
		"vpn_address": "10.13.37.2",
		"allowed_ips": "10.13.37.2/32"
	}`)

	c := NewVPNClient(srv.URL, "usr_test_key", "laptop")
	peer, err := c.GetPeer(context.Background(), "home-pi")
	require.NoError(t, err)
	require.NotNil(t, peer)
	require.Equal(t, "home-pi", peer.DeviceName)
	require.Equal(t, "peer-pubkey", peer.PublicKey)
	require.Equal(t, "203.0.113.1:51820", peer.Endpoint)
	require.Equal(t, "10.13.37.2", peer.VPNAddress)
	require.Equal(t, "10.13.37.2/32", peer.AllowedIPs)

	require.Equal(t, http.MethodGet, rec.method)
	require.Equal(t, "/agent/vpn/peer", rec.path)
	require.Equal(t, "device_name=home-pi", rec.rawQ)
	require.Equal(t, "Bearer usr_test_key", rec.headers.Get("Authorization"))
}

func TestVPNClient_GetPeer_notFound(t *testing.T) {
	srv, _ := newTestServer(t, http.StatusNotFound, `{"error":"peer not registered"}`)

	c := NewVPNClient(srv.URL, "usr_test_key", "laptop")
	peer, err := c.GetPeer(context.Background(), "ghost")
	require.Error(t, err)
	require.Nil(t, peer)
	require.Contains(t, err.Error(), "HTTP 404")
}

func TestVPNClient_DeregisterVPN_success(t *testing.T) {
	srv, rec := newTestServer(t, http.StatusOK, ``)

	c := NewVPNClient(srv.URL, "usr_test_key", "n100-pi")
	require.NoError(t, c.DeregisterVPN(context.Background()))

	require.Equal(t, http.MethodDelete, rec.method)
	require.Equal(t, "/agent/vpn/register", rec.path)
	require.Equal(t, "device_name=n100-pi", rec.rawQ)
	require.Empty(t, rec.body, "DELETE should not send a body")
}

func TestVPNClient_baseURL_trailingSlashTrimmed(t *testing.T) {
	srv, rec := newTestServer(t, http.StatusOK, `{"vpn_address":"10.13.37.9"}`)

	c := NewVPNClient(srv.URL+"/", "usr_test_key", "n100-pi")
	_, err := c.RegisterVPN(context.Background(), "pk", "exit_node", 51820, "")
	require.NoError(t, err)
	// With the trim, the path is /agent/vpn/register (one slash, not two).
	require.Equal(t, "/agent/vpn/register", rec.path)
}

func TestVPNClient_missingAPIKey(t *testing.T) {
	c := NewVPNClient("https://example.com", "", "n100-pi")
	_, err := c.RegisterVPN(context.Background(), "pk", "exit_node", 51820, "")
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "not authenticated")
}

func TestVPNClient_missingBaseURL(t *testing.T) {
	c := NewVPNClient("", "usr_test_key", "n100-pi")
	_, err := c.RegisterVPN(context.Background(), "pk", "exit_node", 51820, "")
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "base url")
}
