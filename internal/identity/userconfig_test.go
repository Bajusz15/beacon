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

	hostname, err := os.Hostname()
	require.NoError(t, err)

	err = WriteCloudLogin("usr_key", "")
	require.NoError(t, err)

	loaded, err := LoadUserConfig()
	require.NoError(t, err)
	require.Equal(t, hostname, loaded.DeviceName)
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
