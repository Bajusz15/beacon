package vpn

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// wgDevice wraps a userspace WireGuard device + its TUN. Userspace mode is used
// (instead of the kernel module) so the same code path runs on Linux, macOS, and
// inside Docker without root-loaded kernel modules. Performance is acceptable for
// homelab traffic; if it becomes a bottleneck we can switch Linux to the kernel
// module behind a build tag.
type wgDevice struct {
	name     string
	tunDev   tun.Device
	dev      *device.Device
	mtu      int
	listenPt int

	mu        sync.Mutex
	closed    bool
	peerKey   string // base64
	peerEnd   string
}

// createWGDevice brings up a userspace WireGuard interface and configures the
// local private key + listen port. Peers are added separately via configurePeer.
func createWGDevice(name string, privateKey string, listenPort int) (*wgDevice, error) {
	if name == "" {
		name = InterfaceName
	}
	if listenPort <= 0 {
		listenPort = DefaultListenPort
	}
	priv, err := wgtypes.ParseKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	// MTU 1420 is the standard WireGuard default (1500 - 80 byte WG overhead).
	const mtu = 1420
	tunDev, err := tun.CreateTUN(name, mtu)
	if err != nil {
		return nil, fmt.Errorf("create tun %q: %w", name, err)
	}

	logger := device.NewLogger(device.LogLevelError, fmt.Sprintf("[vpn:%s] ", name))
	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)

	// Userspace WireGuard config is set via the UAPI text protocol.
	// Keys must be hex-encoded in this protocol (not base64).
	cfg := fmt.Sprintf("private_key=%s\nlisten_port=%d\n", hexFromKey(priv), listenPort)
	if err := dev.IpcSet(cfg); err != nil {
		dev.Close()
		return nil, fmt.Errorf("configure wireguard: %w", err)
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("bring wireguard up: %w", err)
	}

	return &wgDevice{
		name:     name,
		tunDev:   tunDev,
		dev:      dev,
		mtu:      mtu,
		listenPt: listenPort,
	}, nil
}

// configurePeer (re)installs a single peer on the device. Phase 1 only ever has
// one peer per device (one client → one exit node), so we replace whatever was
// there before.
func (w *wgDevice) configurePeer(peerPublicKey, endpoint, allowedIPs string, persistentKeepalive time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("device closed")
	}
	pub, err := wgtypes.ParseKey(peerPublicKey)
	if err != nil {
		return fmt.Errorf("parse peer key: %w", err)
	}
	if strings.TrimSpace(allowedIPs) == "" {
		return errors.New("allowed_ips is required")
	}

	var b strings.Builder
	// `replace_peers=true` clears any prior peers — Phase 1 has at most one.
	b.WriteString("replace_peers=true\n")
	fmt.Fprintf(&b, "public_key=%s\n", hexFromKey(pub))
	if endpoint != "" {
		fmt.Fprintf(&b, "endpoint=%s\n", endpoint)
	}
	b.WriteString("replace_allowed_ips=true\n")
	for _, cidr := range strings.Split(allowedIPs, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		fmt.Fprintf(&b, "allowed_ip=%s\n", cidr)
	}
	if persistentKeepalive > 0 {
		fmt.Fprintf(&b, "persistent_keepalive_interval=%d\n", int(persistentKeepalive.Seconds()))
	}

	if err := w.dev.IpcSet(b.String()); err != nil {
		return fmt.Errorf("set peer: %w", err)
	}
	w.peerKey = peerPublicKey
	w.peerEnd = endpoint
	return nil
}

// stats reads transfer counters from the userspace device via UAPI.
// Returns (rx, tx, lastHandshake). All zero values mean "no peer / no data yet".
func (w *wgDevice) stats() (rx uint64, tx uint64, lastHandshake time.Time, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, 0, time.Time{}, errors.New("device closed")
	}
	var sb strings.Builder
	if err := w.dev.IpcGetOperation(&sb); err != nil {
		return 0, 0, time.Time{}, err
	}
	for _, line := range strings.Split(sb.String(), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "rx_bytes":
			fmt.Sscanf(v, "%d", &rx)
		case "tx_bytes":
			fmt.Sscanf(v, "%d", &tx)
		case "last_handshake_time_sec":
			var sec int64
			fmt.Sscanf(v, "%d", &sec)
			if sec > 0 {
				lastHandshake = time.Unix(sec, 0)
			}
		}
	}
	return rx, tx, lastHandshake, nil
}

// close tears down the WireGuard device and TUN. Safe to call multiple times.
func (w *wgDevice) close() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	w.mu.Unlock()

	if w.dev != nil {
		w.dev.Close()
	}
}

// hexFromKey converts a wgtypes.Key (which prints as base64) to the hex form
// the UAPI protocol expects.
func hexFromKey(k wgtypes.Key) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(k)*2)
	for i, b := range k {
		out[i*2] = hexdigits[b>>4]
		out[i*2+1] = hexdigits[b&0x0f]
	}
	return string(out)
}
