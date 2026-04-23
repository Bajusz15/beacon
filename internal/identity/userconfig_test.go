package identity

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func isolateHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BEACON_HOME", "")
}

func TestLoadUserConfig(t *testing.T) {
	t.Run("round trip save and load", func(t *testing.T) {
		isolateHome(t)

		uc := &UserConfig{
			APIKey:                "usr_test_key_123",
			DeviceName:            "test-device",
			HeartbeatInterval:     45,
			CloudReportingEnabled: true,
			DeviceID:              "device-uuid-123",
		}
		require.NoError(t, uc.Save())

		p, err := UserConfigPath()
		require.NoError(t, err)
		info, err := os.Stat(p)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0600), info.Mode().Perm())

		loaded, err := LoadUserConfig()
		require.NoError(t, err)
		require.Equal(t, uc.APIKey, loaded.APIKey)
		require.Equal(t, uc.DeviceName, loaded.DeviceName)
		require.Equal(t, uc.HeartbeatInterval, loaded.HeartbeatInterval)
		require.Equal(t, uc.CloudReportingEnabled, loaded.CloudReportingEnabled)
		require.Equal(t, uc.DeviceID, loaded.DeviceID)
	})

	t.Run("missing file returns nil", func(t *testing.T) {
		isolateHome(t)
		uc, err := LoadUserConfig()
		require.NoError(t, err)
		require.Nil(t, uc)
	})

	t.Run("invalid YAML errors", func(t *testing.T) {
		isolateHome(t)
		beaconDir := filepath.Join(os.Getenv("HOME"), ".beacon")
		require.NoError(t, os.MkdirAll(beaconDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(beaconDir, "config.yaml"), []byte("invalid: yaml: content: ["), 0600))

		_, err := LoadUserConfig()
		require.ErrorContains(t, err, "parse")
	})

	t.Run("nil Save errors", func(t *testing.T) {
		var uc *UserConfig
		require.ErrorContains(t, uc.Save(), "nil")
	})
}

func TestUserConfigPath(t *testing.T) {
	t.Run("default HOME", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("BEACON_HOME", "")

		p, err := UserConfigPath()
		require.NoError(t, err)
		require.Equal(t, filepath.Join(tmp, ".beacon", "config.yaml"), p)
	})

	t.Run("BEACON_HOME override", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("BEACON_HOME", filepath.Join(tmp, "custom-beacon"))

		p, err := UserConfigPath()
		require.NoError(t, err)
		require.Equal(t, filepath.Join(tmp, "custom-beacon", "config.yaml"), p)
	})
}

func TestWriteCloudLogin(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		isolateHome(t)

		require.NoError(t, WriteCloudLogin("usr_my_api_key", "my-device"))
		loaded, err := LoadUserConfig()
		require.NoError(t, err)
		require.Equal(t, "usr_my_api_key", loaded.APIKey)
		require.Equal(t, "my-device", loaded.DeviceName)
		require.True(t, loaded.CloudReportingEnabled)
		require.Equal(t, 30, loaded.HeartbeatInterval)
		require.NotNil(t, loaded.SystemMetrics)
		require.True(t, loaded.SystemMetrics.Enabled)
	})

	t.Run("missing API key", func(t *testing.T) {
		isolateHome(t)
		require.ErrorContains(t, WriteCloudLogin("", "device"), "api_key is required")
	})

	t.Run("cloud URL is compile-time only", func(t *testing.T) {
		isolateHome(t)
		require.NoError(t, WriteCloudLogin("usr_key", "device"))

		loaded, err := LoadUserConfig()
		require.NoError(t, err)
		require.Equal(t, "https://beaconinfra.dev/api", loaded.EffectiveCloudAPIBase())
	})

	t.Run("uses hostname when no device name", func(t *testing.T) {
		isolateHome(t)
		expected := DetectHostname()
		require.NotEmpty(t, expected)

		require.NoError(t, WriteCloudLogin("usr_key", ""))
		loaded, err := LoadUserConfig()
		require.NoError(t, err)
		require.Equal(t, expected, loaded.DeviceName)
	})

	t.Run("merges existing config", func(t *testing.T) {
		isolateHome(t)
		initial := &UserConfig{
			DeviceID: "existing-device-id",
			Projects: []ProjectConfig{
				{ID: "project1", ConfigPath: "/path/to/project1.yml"},
				{ID: "project2", ConfigPath: "/path/to/project2.yml"},
			},
		}
		require.NoError(t, initial.Save())

		require.NoError(t, WriteCloudLogin("usr_new_key", "new-device"))
		loaded, err := LoadUserConfig()
		require.NoError(t, err)
		require.Equal(t, "usr_new_key", loaded.APIKey)
		require.Equal(t, "existing-device-id", loaded.DeviceID)
		require.Len(t, loaded.Projects, 2)
	})
}

func TestWriteCloudLogout(t *testing.T) {
	isolateHome(t)

	require.NoError(t, WriteCloudLogin("usr_before", "dev"))
	require.NoError(t, WriteCloudLogout())

	after, err := LoadUserConfig()
	require.NoError(t, err)
	require.Empty(t, after.APIKey)
	require.False(t, after.CloudReportingEnabled)
	require.Equal(t, "dev", after.DeviceName)
}

func TestWriteUserLocalInit(t *testing.T) {
	t.Run("fresh config", func(t *testing.T) {
		isolateHome(t)

		require.NoError(t, WriteUserLocalInit("my-box", 9100))
		loaded, err := LoadUserConfig()
		require.NoError(t, err)
		require.Equal(t, "my-box", loaded.DeviceName)
		require.False(t, loaded.CloudReportingEnabled)
		require.Equal(t, 9100, loaded.MetricsPort)
		require.NotNil(t, loaded.SystemMetrics)
		require.True(t, loaded.SystemMetrics.Enabled)
		require.Equal(t, time.Minute, loaded.SystemMetrics.Interval)
	})

	t.Run("preserves existing system metrics", func(t *testing.T) {
		isolateHome(t)
		initial := &UserConfig{
			DeviceName:    "keep-me",
			SystemMetrics: &UserSystemMetricsConfig{Enabled: false, Interval: 2 * time.Minute, CPU: true},
		}
		require.NoError(t, initial.Save())

		require.NoError(t, WriteUserLocalInit("new-name", 0))
		loaded, err := LoadUserConfig()
		require.NoError(t, err)
		require.Equal(t, "new-name", loaded.DeviceName)
		require.False(t, loaded.SystemMetrics.Enabled)
		require.Equal(t, 2*time.Minute, loaded.SystemMetrics.Interval)
	})

	t.Run("preserves cloud reporting on re-run", func(t *testing.T) {
		isolateHome(t)
		require.NoError(t, WriteCloudLogin("usr_preserve_key", "dev"))

		require.NoError(t, WriteUserLocalInit("renamed-device", 0))
		after, err := LoadUserConfig()
		require.NoError(t, err)
		require.Equal(t, "renamed-device", after.DeviceName)
		require.True(t, after.CloudReportingEnabled)
		require.Equal(t, "usr_preserve_key", after.APIKey)
	})
}

func TestMergeBootstrapCloudOnly(t *testing.T) {
	isolateHome(t)

	require.NoError(t, MergeBootstrapCloudOnly(true))
	loaded, err := LoadUserConfig()
	require.NoError(t, err)
	require.True(t, loaded.CloudReportingEnabled)

	require.NoError(t, MergeBootstrapCloudOnly(false))
	loaded, err = LoadUserConfig()
	require.NoError(t, err)
	require.False(t, loaded.CloudReportingEnabled)
}

func TestHeartbeatIntervalDuration(t *testing.T) {
	tests := []struct {
		name     string
		interval int
		expected time.Duration
	}{
		{"positive", 45, 45 * time.Second},
		{"zero", 0, 60 * time.Second},
		{"negative", -10, 60 * time.Second},
		{"default 30", 30, 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := &UserConfig{HeartbeatInterval: tt.interval}
			require.Equal(t, tt.expected, uc.HeartbeatIntervalDuration())
		})
	}

	t.Run("nil receiver", func(t *testing.T) {
		var nilUC *UserConfig
		require.Equal(t, 60*time.Second, nilUC.HeartbeatIntervalDuration())
	})
}

func TestEffectiveCloudAPIBase_nilReceiver(t *testing.T) {
	var uc *UserConfig
	require.Empty(t, uc.EffectiveCloudAPIBase())
}

func TestYAMLStructure(t *testing.T) {
	uc := &UserConfig{
		APIKey:                "test_key",
		DeviceName:            "test-device",
		HeartbeatInterval:     30,
		CloudReportingEnabled: true,
		DeviceID:              "uuid",
	}
	data, err := yaml.Marshal(uc)
	require.NoError(t, err)

	yamlStr := string(data)
	require.Contains(t, yamlStr, "api_key:")
	require.Contains(t, yamlStr, "device_name:")
	require.Contains(t, yamlStr, "heartbeat_interval:")
	require.Contains(t, yamlStr, "cloud_reporting_enabled:")
	require.Contains(t, yamlStr, "device_id:")
	require.NotContains(t, yamlStr, "cloud_url:")
}

func TestAppendProjectIfMissing(t *testing.T) {
	isolateHome(t)

	require.NoError(t, AppendProjectIfMissing("p1", "/x/monitor.yml"))
	require.NoError(t, AppendProjectIfMissing("p2", "/y/monitor.yml"))
	loaded, err := LoadUserConfig()
	require.NoError(t, err)
	require.Len(t, loaded.Projects, 2)

	require.NoError(t, AppendProjectIfMissing("p1", "/z/monitor.yml"))
	loaded, err = LoadUserConfig()
	require.NoError(t, err)
	require.Len(t, loaded.Projects, 2)
	require.Equal(t, "/z/monitor.yml", loaded.Projects[0].ConfigPath)
}

func TestTunnelConfig(t *testing.T) {
	t.Run("append", func(t *testing.T) {
		isolateHome(t)

		require.NoError(t, AppendTunnelIfMissing("tun1", 8080))
		loaded, err := LoadUserConfig()
		require.NoError(t, err)
		require.Len(t, loaded.Tunnels, 1)
		require.Equal(t, "tun1", loaded.Tunnels[0].ID)
		require.Equal(t, 8080, loaded.Tunnels[0].LocalPort)

		require.NoError(t, AppendTunnelIfMissing("tun2", 9090))
		loaded, err = LoadUserConfig()
		require.NoError(t, err)
		require.Len(t, loaded.Tunnels, 2)

		// Update existing
		require.NoError(t, AppendTunnelIfMissing("tun1", 7070))
		loaded, err = LoadUserConfig()
		require.NoError(t, err)
		require.Len(t, loaded.Tunnels, 2)
		require.Equal(t, 7070, loaded.Tunnels[0].LocalPort)
	})

	t.Run("append validation", func(t *testing.T) {
		isolateHome(t)
		require.Error(t, AppendTunnelIfMissing("", 8080))
		require.Error(t, AppendTunnelIfMissing("tun1", 0))
		require.Error(t, AppendTunnelIfMissing("tun1", 99999))
	})

	t.Run("upsert upstream", func(t *testing.T) {
		isolateHome(t)

		require.NoError(t, UpsertTunnelUpstream("tun1", "http", "192.168.1.50", 3000))
		loaded, err := LoadUserConfig()
		require.NoError(t, err)
		require.NotNil(t, loaded.Tunnels[0].Upstream)
		require.Equal(t, "http", loaded.Tunnels[0].Upstream.Protocol)
		require.Equal(t, "192.168.1.50", loaded.Tunnels[0].Upstream.Host)
		require.Equal(t, 3000, loaded.Tunnels[0].Upstream.Port)
		require.Zero(t, loaded.Tunnels[0].LocalPort)

		require.NoError(t, UpsertTunnelUpstream("tun1", "https", "", 443))
		loaded, err = LoadUserConfig()
		require.NoError(t, err)
		require.Equal(t, "https", loaded.Tunnels[0].Upstream.Protocol)
	})

	t.Run("upsert upstream validation", func(t *testing.T) {
		isolateHome(t)
		require.Error(t, UpsertTunnelUpstream("", "http", "", 3000))
		require.Error(t, UpsertTunnelUpstream("tun1", "ftp", "", 3000))
		require.Error(t, UpsertTunnelUpstream("tun1", "http", "", 0))
		require.Error(t, UpsertTunnelUpstream("tun1", "http", "", 99999))
	})

	t.Run("remove", func(t *testing.T) {
		isolateHome(t)

		require.NoError(t, RemoveTunnel("tun1")) // no-op on missing config

		require.NoError(t, AppendTunnelIfMissing("tun1", 8080))
		require.NoError(t, AppendTunnelIfMissing("tun2", 9090))
		require.NoError(t, RemoveTunnel("tun1"))

		loaded, err := LoadUserConfig()
		require.NoError(t, err)
		require.Len(t, loaded.Tunnels, 1)
		require.Equal(t, "tun2", loaded.Tunnels[0].ID)

		require.Error(t, RemoveTunnel(""))
	})

	t.Run("set enabled", func(t *testing.T) {
		isolateHome(t)
		require.NoError(t, AppendTunnelIfMissing("tun1", 8080))

		require.NoError(t, SetTunnelEnabled("tun1", false))
		loaded, err := LoadUserConfig()
		require.NoError(t, err)
		require.False(t, *loaded.Tunnels[0].Enabled)

		require.NoError(t, SetTunnelEnabled("tun1", true))
		loaded, err = LoadUserConfig()
		require.NoError(t, err)
		require.True(t, *loaded.Tunnels[0].Enabled)

		require.ErrorContains(t, SetTunnelEnabled("nonexistent", true), "not found")
		require.Error(t, SetTunnelEnabled("", true))
	})
}

func TestVPNConfig(t *testing.T) {
	t.Run("exit node writes YAML", func(t *testing.T) {
		isolateHome(t)
		base := &UserConfig{DeviceName: "n100-pi", APIKey: "usr_test"}
		require.NoError(t, base.Save())

		require.NoError(t, SetVPNExitNode(0, "10.13.37.5"))
		loaded, err := LoadUserConfig()
		require.NoError(t, err)
		require.NotNil(t, loaded.VPN)
		require.True(t, loaded.VPN.Enabled)
		require.Equal(t, "exit_node", loaded.VPN.Role)
		require.Equal(t, 51820, loaded.VPN.ListenPort)
		require.Equal(t, "10.13.37.5", loaded.VPN.VPNAddress)
		require.Empty(t, loaded.VPN.PeerDevice)
		require.Equal(t, "n100-pi", loaded.DeviceName)
		require.Equal(t, "usr_test", loaded.APIKey)
	})

	t.Run("exit node custom port", func(t *testing.T) {
		isolateHome(t)
		require.NoError(t, SetVPNExitNode(41820, "10.13.37.7"))
		loaded, err := LoadUserConfig()
		require.NoError(t, err)
		require.Equal(t, 41820, loaded.VPN.ListenPort)
	})

	t.Run("client writes YAML", func(t *testing.T) {
		isolateHome(t)
		require.NoError(t, SetVPNClient("home-pi", "10.13.37.9"))
		loaded, err := LoadUserConfig()
		require.NoError(t, err)
		require.True(t, loaded.VPN.Enabled)
		require.Equal(t, "client", loaded.VPN.Role)
		require.Equal(t, "home-pi", loaded.VPN.PeerDevice)
		require.Equal(t, "10.13.37.9", loaded.VPN.VPNAddress)
	})

	t.Run("client empty peer rejected", func(t *testing.T) {
		isolateHome(t)
		require.Error(t, SetVPNClient("   ", "10.13.37.9"))
	})

	t.Run("clear is idempotent", func(t *testing.T) {
		isolateHome(t)
		require.NoError(t, ClearVPN()) // no config yet

		require.NoError(t, SetVPNExitNode(51820, "10.13.37.5"))
		require.NoError(t, ClearVPN())
		loaded, err := LoadUserConfig()
		require.NoError(t, err)
		require.Nil(t, loaded.VPN)

		require.NoError(t, ClearVPN()) // already cleared
	})

	t.Run("exit node overwrites client mode", func(t *testing.T) {
		isolateHome(t)
		require.NoError(t, SetVPNClient("home-pi", "10.13.37.9"))
		require.NoError(t, SetVPNExitNode(51820, "10.13.37.1"))

		loaded, err := LoadUserConfig()
		require.NoError(t, err)
		require.Equal(t, "exit_node", loaded.VPN.Role)
		require.Empty(t, loaded.VPN.PeerDevice)
	})
}
