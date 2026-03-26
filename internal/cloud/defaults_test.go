package cloud

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBeaconInfraAPIBase_default(t *testing.T) {
	old := DefaultBeaconInfraAPIURL
	t.Cleanup(func() { DefaultBeaconInfraAPIURL = old })
	DefaultBeaconInfraAPIURL = "https://beaconinfra.dev/api"
	require.Equal(t, "https://beaconinfra.dev/api", BeaconInfraAPIBase())
}

func TestBeaconInfraAPIBase_trimsSlash(t *testing.T) {
	old := DefaultBeaconInfraAPIURL
	t.Cleanup(func() { DefaultBeaconInfraAPIURL = old })
	DefaultBeaconInfraAPIURL = "https://example.com/api/"
	require.Equal(t, "https://example.com/api", BeaconInfraAPIBase())
}
