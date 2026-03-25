package identity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// UserConfig is ~/.beacon/config.yaml (v2 identity).
type UserConfig struct {
	APIKey                string          `yaml:"api_key,omitempty"`
	DeviceName            string          `yaml:"device_name,omitempty"`
	CloudURL              string          `yaml:"cloud_url,omitempty"`
	HeartbeatInterval     int             `yaml:"heartbeat_interval,omitempty"`
	CloudReportingEnabled bool            `yaml:"cloud_reporting_enabled,omitempty"`
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

// UserConfigPath returns the path to ~/.beacon/config.yaml.
func UserConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home: %w", err)
	}
	return filepath.Join(home, ".beacon", "config.yaml"), nil
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

// WriteUserInit writes or updates ~/.beacon/config.yaml with API key, device name, and cloud URL.
func WriteUserInit(apiKey, deviceName, cloudURL string) error {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return errors.New("api_key is required (--api-key or BEACON_API_KEY)")
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
		url = strings.TrimSpace(os.Getenv("BEACON_CLOUD_URL"))
	}
	if url == "" {
		url = strings.TrimSpace(os.Getenv("BEACON_SERVER_URL"))
	}
	if url == "" {
		return errors.New("cloud_url is required (--cloud-url or BEACON_CLOUD_URL / BEACON_SERVER_URL)")
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
