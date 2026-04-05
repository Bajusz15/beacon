package identity

import (
	"errors"
	"fmt"
	"strings"
)

// TunnelUpstream is the HTTP(S) service the tunnel forwards to (LAN, Docker DNS, or loopback).
type TunnelUpstream struct {
	Protocol string `yaml:"protocol,omitempty"` // http or https; default http
	Host     string `yaml:"host,omitempty"`     // default 127.0.0.1 when upstream block is present
	Port     int    `yaml:"port,omitempty"`
}

// EffectiveUpstream returns protocol, host, port for dialing the tunneled service.
// Legacy configs with only local_port map to http://127.0.0.1:<local_port>.
func (t *TunnelConfig) EffectiveUpstream() (protocol, host string, port int, err error) {
	if t == nil {
		return "", "", 0, errors.New("nil tunnel config")
	}
	if t.Upstream != nil && (t.Upstream.Port > 0 || strings.TrimSpace(t.Upstream.Host) != "" || strings.TrimSpace(t.Upstream.Protocol) != "") {
		protocol = strings.ToLower(strings.TrimSpace(t.Upstream.Protocol))
		if protocol == "" {
			protocol = "http"
		}
		if protocol != "http" && protocol != "https" {
			return "", "", 0, fmt.Errorf("upstream.protocol must be http or https")
		}
		host = strings.TrimSpace(t.Upstream.Host)
		if host == "" {
			host = "127.0.0.1"
		}
		port = t.Upstream.Port
		if port <= 0 || port > 65535 {
			return "", "", 0, fmt.Errorf("upstream.port must be between 1 and 65535")
		}
		return protocol, host, port, nil
	}
	if t.LocalPort <= 0 || t.LocalPort > 65535 {
		return "", "", 0, fmt.Errorf("local_port must be between 1 and 65535 when upstream is omitted")
	}
	return "http", "127.0.0.1", t.LocalPort, nil
}
