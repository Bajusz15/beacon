package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBeaconHomeDir_default(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("BEACON_HOME", "")
	p, err := BeaconHomeDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(tmp, ".beacon"), p)
}

func TestBeaconHomeDir_override(t *testing.T) {
	tmp := t.TempDir()
	custom := filepath.Join(tmp, "bh")
	t.Setenv("HOME", tmp)
	t.Setenv("BEACON_HOME", custom)
	p, err := BeaconHomeDir()
	require.NoError(t, err)
	require.Equal(t, custom, p)
}
