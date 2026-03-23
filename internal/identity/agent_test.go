package identity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestIdentity_SaveAs_roundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "agent.yml")
	id := &Identity{
		ServerURL:   "https://example.com/api",
		DeviceName:  "test-box",
		DeviceToken: "dtk_test",
		DeviceID:    "550e8400-e29b-41d4-a716-446655440000",
	}
	require.NoError(t, id.SaveAs(p))

	raw, err := os.ReadFile(p)
	require.NoError(t, err)
	var got Identity
	require.NoError(t, yaml.Unmarshal(raw, &got))
	require.Equal(t, id.ServerURL, got.ServerURL)
	require.Equal(t, id.DeviceName, got.DeviceName)
	require.Equal(t, id.DeviceToken, got.DeviceToken)
	require.Equal(t, id.DeviceID, got.DeviceID)
}
