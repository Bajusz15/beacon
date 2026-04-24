package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// VPNPeerInfo is the cloud-side representation of a peer's VPN config.
// The vpn package has its own type with the same shape — they're kept separate
// to avoid an import cycle (cloud → vpn → identity → cloud).
type VPNPeerInfo struct {
	DeviceName string `json:"device_name"`
	PublicKey  string `json:"public_key"`
	Endpoint   string `json:"endpoint"`
	VPNAddress string `json:"vpn_address"`
	AllowedIPs string `json:"allowed_ips"`
}

// VPNClient implements vpn.PeerResolver against the BeaconInfra /api/agent/vpn
// endpoints. It is the only place in the agent that talks HTTP for VPN coordination —
// the rest of the VPN code never sees the cloud directly, which keeps the manager
// trivially unit-testable.
type VPNClient struct {
	baseURL    string // e.g. https://beaconinfra.dev/api
	apiKey     string
	deviceName string
	http       *http.Client
}

// NewVPNClient builds a VPN coordination client. Pass the API base (with /api),
// the user's API key, and the device's local name (so the server can resolve identity).
func NewVPNClient(baseURL, apiKey, deviceName string) *VPNClient {
	return &VPNClient{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:     strings.TrimSpace(apiKey),
		deviceName: strings.TrimSpace(deviceName),
		http:       &http.Client{Timeout: 15 * time.Second},
	}
}

type vpnRegisterPayload struct {
	DeviceName string `json:"device_name"`
	PublicKey  string `json:"public_key"`
	Endpoint   string `json:"endpoint,omitempty"`
	ListenPort int    `json:"listen_port,omitempty"`
	Role       string `json:"role"`
}

type vpnRegisterResponse struct {
	VPNAddress string `json:"vpn_address"`
}

// RegisterVPN posts the local public key + listen port to the cloud and returns
// the VPN address the server allocated. Idempotent — calling it again with the
// same key returns the same address.
func (c *VPNClient) RegisterVPN(ctx context.Context, publicKey, role string, listenPort int, endpoint string) (string, error) {
	payload := vpnRegisterPayload{
		DeviceName: c.deviceName,
		PublicKey:  publicKey,
		Endpoint:   endpoint,
		ListenPort: listenPort,
		Role:       role,
	}
	var resp vpnRegisterResponse
	if err := c.do(ctx, http.MethodPost, "/agent/vpn/register", payload, &resp); err != nil {
		return "", err
	}
	return resp.VPNAddress, nil
}

// GetPeer fetches a peer's public key + endpoint by device name.
// Both devices must belong to the calling user.
func (c *VPNClient) GetPeer(ctx context.Context, deviceName string) (*VPNPeerInfo, error) {
	q := url.Values{}
	q.Set("device_name", deviceName)
	var info VPNPeerInfo
	if err := c.do(ctx, http.MethodGet, "/agent/vpn/peer?"+q.Encode(), nil, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// DeregisterVPN removes the device's VPN config from the cloud. Idempotent.
func (c *VPNClient) DeregisterVPN(ctx context.Context) error {
	q := url.Values{}
	q.Set("device_name", c.deviceName)
	return c.do(ctx, http.MethodDelete, "/agent/vpn/register?"+q.Encode(), nil, nil)
}

// do performs an authenticated request and JSON-decodes the response into out (if non-nil).
func (c *VPNClient) do(ctx context.Context, method, path string, body, out any) error {
	if c.baseURL == "" {
		return fmt.Errorf("vpn client: base url not set")
	}
	if c.apiKey == "" {
		return fmt.Errorf("vpn client: not authenticated (run `beacon cloud login`)")
	}

	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("vpn %s %s: HTTP %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode vpn response: %w", err)
		}
	}
	return nil
}
