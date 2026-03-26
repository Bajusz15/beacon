package identity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"beacon/internal/cloud"
	"beacon/internal/config"

	"gopkg.in/yaml.v3"
)

// UserConfig is ~/.beacon/config.yaml (v2 identity).
type UserConfig struct {
	APIKey                string          `yaml:"api_key,omitempty"`
	DeviceName            string          `yaml:"device_name,omitempty"`
	CloudURL              string          `yaml:"cloud_url,omitempty"`
	HeartbeatInterval     int             `yaml:"heartbeat_interval,omitempty"`
	CloudReportingEnabled bool            `yaml:"cloud_reporting_enabled"`
	DeviceID              string          `yaml:"device_id,omitempty"`
	MetricsPort           int             `yaml:"metrics_port,omitempty"`
	MetricsListenAddr     string          `yaml:"metrics_listen_addr,omitempty"` // default "127.0.0.1"; set "0.0.0.0" for Docker
	Projects              []ProjectConfig `yaml:"projects,omitempty"`
}

// ProjectConfig defines a project that the master will spawn a child agent for.
type ProjectConfig struct {
	ID         string `yaml:"id"`          // Unique project identifier
	ConfigPath string `yaml:"config_path"` // Path to the project's monitor.yml
	// Enabled is tri-state:
	// nil => omitted in YAML (default: true)
	// true/false => explicitly set.
	Enabled *bool `yaml:"enabled,omitempty"`
}

// UserConfigPath returns the path to config.yaml under the Beacon home directory.
func UserConfigPath() (string, error) {
	base, err := config.BeaconHomeDir()
	if err != nil {
		return "", fmt.Errorf("beacon home: %w", err)
	}
	return filepath.Join(base, "config.yaml"), nil
}

// LoadUserConfig reads config.yaml. Returns (nil, nil) if the file is missing.
func LoadUserConfig() (*UserConfig, error) {
	p, err := UserConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	var f UserConfig
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	return &f, nil
}

func (f *UserConfig) Save() error {
	if f == nil {
		return errors.New("identity: nil UserConfig")
	}
	p, err := UserConfigPath()
	if err != nil {
		return err
	}
	return saveUserConfig(p, f)
}

// EffectiveCloudAPIBase returns the API base URL for heartbeats: config cloud_url if set, else the
// compile-time default (see beacon/internal/cloud).
func (uc *UserConfig) EffectiveCloudAPIBase() string {
	if uc == nil {
		return ""
	}
	if s := strings.TrimSpace(uc.CloudURL); s != "" {
		return strings.TrimSuffix(s, "/")
	}
	return cloud.BeaconInfraAPIBase()
}

// WriteUserLocalInit writes or merges ~/.beacon/config.yaml for local-only use: no API key required,
// cloud_reporting_enabled false. Preserves existing api_key and projects if present.
func WriteUserLocalInit(deviceName string, metricsPort int) error {
	name := strings.TrimSpace(deviceName)
	if name == "" {
		h, err := os.Hostname()
		if err != nil || strings.TrimSpace(h) == "" {
			return errors.New("device name is required (--name) or hostname must be set")
		}
		name = strings.TrimSpace(h)
	}
	p, err := UserConfigPath()
	if err != nil {
		return err
	}
	f, err := readExistingUserConfig(p)
	if err != nil {
		return err
	}
	if f == nil {
		f = &UserConfig{}
	}
	f.DeviceName = name
	f.CloudReportingEnabled = false
	if f.HeartbeatInterval <= 0 {
		f.HeartbeatInterval = 30
	}
	if metricsPort > 0 {
		f.MetricsPort = metricsPort
	}
	return saveUserConfig(p, f)
}

// AppendProjectIfMissing adds or updates a project entry in ~/.beacon/config.yaml for the master agent.
func AppendProjectIfMissing(projectID, configPath string) error {
	projectID = strings.TrimSpace(projectID)
	configPath = strings.TrimSpace(configPath)
	if projectID == "" || configPath == "" {
		return errors.New("project id and config path are required")
	}
	p, err := UserConfigPath()
	if err != nil {
		return err
	}
	f, err := readExistingUserConfig(p)
	if err != nil {
		return err
	}
	if f == nil {
		f = &UserConfig{}
	}
	for i := range f.Projects {
		if f.Projects[i].ID == projectID {
			f.Projects[i].ConfigPath = configPath
			return saveUserConfig(p, f)
		}
	}
	f.Projects = append(f.Projects, ProjectConfig{ID: projectID, ConfigPath: configPath})
	return saveUserConfig(p, f)
}

// WriteCloudLogin writes BeaconInfra API credentials and enables cloud reporting.
// If cloudURL is empty, the compile-time default URL is used (not environment variables).
func WriteCloudLogin(apiKey, deviceName, cloudURL string) error {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return errors.New("api_key is required")
	}
	name := strings.TrimSpace(deviceName)
	if name == "" {
		h, err := os.Hostname()
		if err != nil || strings.TrimSpace(h) == "" {
			return errors.New("device name is required (--name) or hostname must be set")
		}
		name = strings.TrimSpace(h)
	}
	url := strings.TrimSpace(cloudURL)
	if url == "" {
		url = cloud.BeaconInfraAPIBase()
	} else {
		url = strings.TrimSuffix(url, "/")
	}

	p, err := UserConfigPath()
	if err != nil {
		return err
	}
	f, err := readExistingUserConfig(p)
	if err != nil {
		return err
	}
	if f == nil {
		f = &UserConfig{}
	}
	f.APIKey = key
	f.DeviceName = name
	f.CloudURL = url
	if f.HeartbeatInterval <= 0 {
		f.HeartbeatInterval = 30
	}
	f.CloudReportingEnabled = true
	return saveUserConfig(p, f)
}

// WriteCloudLogout clears cloud credentials and disables reporting.
func WriteCloudLogout() error {
	p, err := UserConfigPath()
	if err != nil {
		return err
	}
	f, err := readExistingUserConfig(p)
	if err != nil {
		return err
	}
	if f == nil {
		f = &UserConfig{}
	}
	f.APIKey = ""
	f.CloudURL = ""
	f.CloudReportingEnabled = false
	return saveUserConfig(p, f)
}

func MergeBootstrapCloudOnly(enabled bool) error {
	p, err := UserConfigPath()
	if err != nil {
		return err
	}
	f, err := readExistingUserConfig(p)
	if err != nil {
		return err
	}
	if f == nil {
		f = &UserConfig{}
	}
	f.CloudReportingEnabled = enabled
	return saveUserConfig(p, f)
}

func (f *UserConfig) HeartbeatIntervalDuration() time.Duration {
	if f == nil || f.HeartbeatInterval <= 0 {
		return 60 * time.Second
	}
	return time.Duration(f.HeartbeatInterval) * time.Second
}

func readExistingUserConfig(p string) (*UserConfig, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var f UserConfig
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

func saveUserConfig(p string, f *UserConfig) error {
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	out, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, out, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
