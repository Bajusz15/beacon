package identity

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// DetectHostname returns the best available hostname for this machine.
// It tries multiple strategies in order of preference:
//  1. hostname -f (FQDN, Linux/macOS only)
//  2. os.Hostname() (Go stdlib, works cross-platform)
//  3. /etc/hostname file (Linux fallback)
//
// Returns empty string if all methods fail.
func DetectHostname() string {
	// Try FQDN first on Unix systems
	if runtime.GOOS != "windows" {
		if fqdn := runHostnameCmd("-f"); fqdn != "" && fqdn != "localhost" {
			return fqdn
		}
	}

	// Standard Go hostname (calls gethostname(2) on Unix, GetComputerNameExW on Windows)
	if h, err := os.Hostname(); err == nil {
		h = strings.TrimSpace(h)
		if h != "" && h != "localhost" {
			return h
		}
	}

	// Linux fallback: read /etc/hostname
	if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/etc/hostname"); err == nil {
			h := strings.TrimSpace(string(data))
			if h != "" {
				return h
			}
		}
	}

	return ""
}

func runHostnameCmd(args ...string) string {
	cmd := exec.Command("hostname", args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
