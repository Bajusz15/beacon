package cloud

import "strings"

// DefaultBeaconInfraAPIURL is the production BeaconInfra API base URL (must include /api).
// Forks and self-hosted builds may override at link time, e.g.:
//
//	go build -ldflags "-X beacon/internal/cloud.DefaultBeaconInfraAPIURL=https://example.com/api" ./cmd/beacon
//
// This is the hosted-trial default. A locally saved server can override it;
// environment variables never silently redirect credentials.
var DefaultBeaconInfraAPIURL = "https://beaconinfra.dev/api"

// BeaconInfraAPIBase returns the canonical API base URL for this binary (no trailing slash).
func BeaconInfraAPIBase() string {
	s := strings.TrimSpace(DefaultBeaconInfraAPIURL)
	if s == "" {
		return "https://beaconinfra.dev/api"
	}
	return strings.TrimSuffix(s, "/")
}
