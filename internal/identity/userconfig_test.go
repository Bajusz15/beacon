package identity

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestUserConfig_SaveAndLoad_roundTrip(t *testing.T) {
	// Create temp dir and override home for test
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	uc := &UserConfig{
		APIKey:                "usr_test_key_123",
		DeviceName:            "test-device",
		HeartbeatInterval:     45,
		CloudReportingEnabled: true,
		DeviceID:              "device-uuid-123",
	}

	err := uc.Save()
	require.NoError(t, err)

	// Verify file exists with correct permissions
	p, err := UserConfigPath()
	require.NoError(t, err)
	info, err := os.Stat(p)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm(), "config.yaml should have 0600 permissions")

	// Load and verify
	loaded, err := LoadUserConfig()
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, uc.APIKey, loaded.APIKey)
	require.Equal(t, uc.DeviceName, loaded.DeviceName)
	require.Equal(t, uc.HeartbeatInterval, loaded.HeartbeatInterval)
	require.Equal(t, uc.CloudReportingEnabled, loaded.CloudReportingEnabled)
	require.Equal(t, uc.DeviceID, loaded.DeviceID)
}

func TestLoadUserConfig_missing(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	// File doesn't exist - should return nil, nil
	uc, err := LoadUserConfig()
	require.NoError(t, err)
	require.Nil(t, uc)
}

func TestLoadUserConfig_invalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	// Create invalid YAML file
	beaconDir := filepath.Join(tmpDir, ".beacon")
	require.NoError(t, os.MkdirAll(beaconDir, 0755))
	configPath := filepath.Join(beaconDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("invalid: yaml: content: ["), 0600))

	_, err := LoadUserConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse")
}

func TestWriteCloudLogin_success(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	err := WriteCloudLogin("usr_my_api_key", "my-device")
	require.NoError(t, err)

	loaded, err := LoadUserConfig()
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, "usr_my_api_key", loaded.APIKey)
	require.Equal(t, "my-device", loaded.DeviceName)
	require.True(t, loaded.CloudReportingEnabled)
	require.Equal(t, 30, loaded.HeartbeatInterval) // default
	require.NotNil(t, loaded.SystemMetrics)
	require.True(t, loaded.SystemMetrics.Enabled)
}

func TestWriteCloudLogin_missingAPIKey(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	err := WriteCloudLogin("", "device")
	require.Error(t, err)
	require.Contains(t, err.Error(), "api_key is required")
}

func TestWriteCloudLogin_cloudURLIsCompileTimeOnly(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	err := WriteCloudLogin("usr_key", "device")
	require.NoError(t, err)

	loaded, err := LoadUserConfig()
	require.NoError(t, err)
	// Cloud URL is not stored in config — it's compile-time only
	require.Equal(t, "https://beaconinfra.dev/api", loaded.EffectiveCloudAPIBase())
}

func TestWriteCloudLogin_usesHostnameWhenNoDeviceName(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	expected := DetectHostname()
	require.NotEmpty(t, expected, "DetectHostname must resolve for this test")

	err := WriteCloudLogin("usr_key", "")
	require.NoError(t, err)

	loaded, err := LoadUserConfig()
	require.NoError(t, err)
	require.Equal(t, expected, loaded.DeviceName)
}

func TestWriteCloudLogout_clearsKeyAndDisablesReporting(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	require.NoError(t, WriteCloudLogin("usr_before", "dev"))
	loaded, err := LoadUserConfig()
	require.NoError(t, err)
	require.True(t, loaded.CloudReportingEnabled)

	require.NoError(t, WriteCloudLogout())
	after, err := LoadUserConfig()
	require.NoError(t, err)
	require.Empty(t, after.APIKey)
	require.False(t, after.CloudReportingEnabled)
	require.Equal(t, "dev", after.DeviceName)
}

func TestWriteCloudLogin_mergesExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	initial := &UserConfig{
		DeviceID: "existing-device-id",
		Projects: []ProjectConfig{
			{ID: "project1", ConfigPath: "/path/to/project1.yml"},
			{ID: "project2", ConfigPath: "/path/to/project2.yml"},
		},
	}
	require.NoError(t, initial.Save())

	err := WriteCloudLogin("usr_new_key", "new-device")
	require.NoError(t, err)

	loaded, err := LoadUserConfig()
	require.NoError(t, err)
	require.Equal(t, "usr_new_key", loaded.APIKey)
	require.Equal(t, "new-device", loaded.DeviceName)
	require.Equal(t, "existing-device-id", loaded.DeviceID)
	require.Len(t, loaded.Projects, 2)
	require.NotNil(t, loaded.SystemMetrics)
	require.True(t, loaded.SystemMetrics.Enabled)
}

func TestWriteUserLocalInit_preservesExistingSystemMetrics(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	custom := &UserSystemMetricsConfig{
		Enabled:  false,
		Interval: 2 * time.Minute,
		CPU:      true,
	}
	initial := &UserConfig{
		DeviceName:            "keep-me",
		CloudReportingEnabled: false,
		SystemMetrics:         custom,
	}
	require.NoError(t, initial.Save())

	require.NoError(t, WriteUserLocalInit("new-name", 0))
	loaded, err := LoadUserConfig()
	require.NoError(t, err)
	require.Equal(t, "new-name", loaded.DeviceName)
	require.NotNil(t, loaded.SystemMetrics)
	require.False(t, loaded.SystemMetrics.Enabled)
	require.Equal(t, 2*time.Minute, loaded.SystemMetrics.Interval)
}

func TestWriteUserLocalInit_preservesCloudReportingWhenReRunning(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	require.NoError(t, WriteCloudLogin("usr_preserve_key", "dev"))
	loaded, err := LoadUserConfig()
	require.NoError(t, err)
	require.True(t, loaded.CloudReportingEnabled)

	require.NoError(t, WriteUserLocalInit("renamed-device", 0))
	after, err := LoadUserConfig()
	require.NoError(t, err)
	require.Equal(t, "renamed-device", after.DeviceName)
	require.True(t, after.CloudReportingEnabled)
	require.Equal(t, "usr_preserve_key", after.APIKey)
}

func TestMergeBootstrapCloudOnly(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	// Call without existing file
	err := MergeBootstrapCloudOnly(true)
	require.NoError(t, err)

	loaded, err := LoadUserConfig()
	require.NoError(t, err)
	require.True(t, loaded.CloudReportingEnabled)

	// Now update to false
	err = MergeBootstrapCloudOnly(false)
	require.NoError(t, err)

	loaded, err = LoadUserConfig()
	require.NoError(t, err)
	require.False(t, loaded.CloudReportingEnabled)
}

func TestUserConfig_HeartbeatIntervalDuration(t *testing.T) {
	tests := []struct {
		name     string
		interval int
		expected time.Duration
	}{
		{"positive value", 45, 45 * time.Second},
		{"zero value", 0, 60 * time.Second},
		{"negative value", -10, 60 * time.Second},
		{"default", 30, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := &UserConfig{HeartbeatInterval: tt.interval}
			require.Equal(t, tt.expected, uc.HeartbeatIntervalDuration())
		})
	}

	// Test nil receiver
	var nilUC *UserConfig
	require.Equal(t, 60*time.Second, nilUC.HeartbeatIntervalDuration())
}

func TestUserConfigPath(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	t.Setenv("BEACON_HOME", "")
	defer func() { _ = os.Setenv("HOME", origHome) }()

	p, err := UserConfigPath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(tmpDir, ".beacon", "config.yaml"), p)
}

func TestUserConfigPath_BEACON_HOME(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origBH := os.Getenv("BEACON_HOME")
	t.Setenv("HOME", tmpDir)
	t.Setenv("BEACON_HOME", filepath.Join(tmpDir, "custom-beacon"))
	defer func() {
		_ = os.Setenv("HOME", origHome)
		_ = os.Setenv("BEACON_HOME", origBH)
	}()

	p, err := UserConfigPath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(tmpDir, "custom-beacon", "config.yaml"), p)
}

func TestWriteUserLocalInit(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	require.NoError(t, WriteUserLocalInit("my-box", 9100))
	loaded, err := LoadUserConfig()
	require.NoError(t, err)
	require.Equal(t, "my-box", loaded.DeviceName)
	require.False(t, loaded.CloudReportingEnabled)
	require.Equal(t, 9100, loaded.MetricsPort)
	require.NotNil(t, loaded.SystemMetrics)
	require.True(t, loaded.SystemMetrics.Enabled)
	require.Equal(t, time.Minute, loaded.SystemMetrics.Interval)
	require.True(t, loaded.SystemMetrics.CPU)
	require.True(t, loaded.SystemMetrics.Memory)
	require.True(t, loaded.SystemMetrics.Disk)
	require.True(t, loaded.SystemMetrics.LoadAverage)
	require.Equal(t, "/", loaded.SystemMetrics.DiskPath)
}

func TestAppendProjectIfMissing(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

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

func TestUserConfig_YAMLStructure(t *testing.T) {
	// Test that YAML output has expected field names
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
	require.NotContains(t, yamlStr, "cloud_url:") // cloud_url is compile-time only
}

func TestSetVPNExitNode_writesYAML(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("BEACON_HOME", "")

	// Pre-populate a baseline config so we can verify the VPN block is added,
	// not that the whole file is replaced.
	base := &UserConfig{DeviceName: "n100-pi", APIKey: "usr_test"}
	require.NoError(t, base.Save())

	require.NoError(t, SetVPNExitNode(0, "10.13.37.5"))

	loaded, err := LoadUserConfig()
	require.NoError(t, err)
	require.NotNil(t, loaded.VPN)
	require.True(t, loaded.VPN.Enabled)
	require.Equal(t, "exit_node", loaded.VPN.Role)
	require.Equal(t, 51820, loaded.VPN.ListenPort, "zero listen port should default to 51820")
	require.Equal(t, "10.13.37.5", loaded.VPN.VPNAddress)
	require.Empty(t, loaded.VPN.PeerDevice)
	// Untouched fields stay put.
	require.Equal(t, "n100-pi", loaded.DeviceName)
	require.Equal(t, "usr_test", loaded.APIKey)
}

func TestSetVPNExitNode_customListenPort(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("BEACON_HOME", "")

	require.NoError(t, SetVPNExitNode(41820, "10.13.37.7"))

	loaded, err := LoadUserConfig()
	require.NoError(t, err)
	require.Equal(t, 41820, loaded.VPN.ListenPort)
}

func TestSetVPNClient_writesYAML(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("BEACON_HOME", "")

	require.NoError(t, SetVPNClient("home-pi", "10.13.37.9"))

	loaded, err := LoadUserConfig()
	require.NoError(t, err)
	require.NotNil(t, loaded.VPN)
	require.True(t, loaded.VPN.Enabled)
	require.Equal(t, "client", loaded.VPN.Role)
	require.Equal(t, "home-pi", loaded.VPN.PeerDevice)
	require.Equal(t, "10.13.37.9", loaded.VPN.VPNAddress)
}

func TestSetVPNClient_emptyPeerRejected(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("BEACON_HOME", "")

	err := SetVPNClient("   ", "10.13.37.9")
	require.Error(t, err)
}

func TestClearVPN_idempotent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("BEACON_HOME", "")

	// ClearVPN against a non-existent config is a no-op.
	require.NoError(t, ClearVPN())

	require.NoError(t, SetVPNExitNode(51820, "10.13.37.5"))
	loaded, err := LoadUserConfig()
	require.NoError(t, err)
	require.NotNil(t, loaded.VPN)

	require.NoError(t, ClearVPN())
	loaded, err = LoadUserConfig()
	require.NoError(t, err)
	require.Nil(t, loaded.VPN, "VPN block should be gone after ClearVPN")

	// And calling Clear again should still succeed.
	require.NoError(t, ClearVPN())
}

func TestSetVPNExitNode_overwritesClientMode(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("BEACON_HOME", "")

	// Start as a client, switch to exit node — peer_device must be cleared
	// or the master would try to do both at once.
	require.NoError(t, SetVPNClient("home-pi", "10.13.37.9"))
	require.NoError(t, SetVPNExitNode(51820, "10.13.37.1"))

	loaded, err := LoadUserConfig()
	require.NoError(t, err)
	require.Equal(t, "exit_node", loaded.VPN.Role)
	require.Empty(t, loaded.VPN.PeerDevice, "switching to exit node must clear peer_device")
}
