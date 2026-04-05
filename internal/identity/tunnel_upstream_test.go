package identity

import "testing"

func TestEffectiveUpstream_legacy(t *testing.T) {
	tc := &TunnelConfig{ID: "a", LocalPort: 3000}
	p, h, port, err := tc.EffectiveUpstream()
	if err != nil || p != "http" || h != "127.0.0.1" || port != 3000 {
		t.Fatalf("got %s %s %d %v", p, h, port, err)
	}
}

func TestEffectiveUpstream_docker(t *testing.T) {
	tc := &TunnelConfig{
		ID: "ha",
		Upstream: &TunnelUpstream{
			Protocol: "http",
			Host:     "homeassistant",
			Port:     8123,
		},
	}
	p, h, port, err := tc.EffectiveUpstream()
	if err != nil || p != "http" || h != "homeassistant" || port != 8123 {
		t.Fatalf("got %s %s %d %v", p, h, port, err)
	}
}

func TestEffectiveUpstream_httpsLoopback(t *testing.T) {
	tc := &TunnelConfig{
		ID: "x",
		Upstream: &TunnelUpstream{
			Protocol: "https",
			Host:     "",
			Port:     8443,
		},
	}
	p, h, port, err := tc.EffectiveUpstream()
	if err != nil || p != "https" || h != "127.0.0.1" || port != 8443 {
		t.Fatalf("got %s %s %d %v", p, h, port, err)
	}
}
