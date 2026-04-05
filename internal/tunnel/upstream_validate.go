package tunnel

import (
	"fmt"
	"net"
	"strings"
)

// ValidateDialTarget ensures the tunnel only forwards to loopback, private LAN, or safe local hostnames
// (not arbitrary public internet). Values come from local config only.
func ValidateDialTarget(protocol, host string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	p := strings.ToLower(strings.TrimSpace(protocol))
	if p != "http" && p != "https" {
		return fmt.Errorf("protocol must be http or https")
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("upstream host is empty")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if !allowedTunnelIP(ip) {
			return fmt.Errorf("upstream IP %s is not allowed (use loopback, RFC1918, or ULA)", ip)
		}
		return nil
	}
	if !allowedTunnelHostname(host) {
		return fmt.Errorf("invalid upstream hostname %q", host)
	}
	return nil
}

func allowedTunnelIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}
	if ip.IsPrivate() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		// Link-local (e.g. mDNS); block cloud metadata address only.
		if ip4[0] == 169 && ip4[1] == 254 {
			if ip4[2] == 169 && ip4[3] == 254 {
				return false
			}
			return true
		}
	}
	return false
}

func allowedTunnelHostname(h string) bool {
	if len(h) == 0 || len(h) > 253 {
		return false
	}
	for _, label := range strings.Split(h, ".") {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		for i, c := range label {
			isAlnum := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
			if isAlnum {
				continue
			}
			if c == '-' && i > 0 && i < len(label)-1 {
				continue
			}
			if c == '_' {
				continue
			}
			return false
		}
	}
	return true
}
