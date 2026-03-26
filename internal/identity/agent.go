package identity

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"beacon/internal/config"

	"gopkg.in/yaml.v3"
)

const agentYAMLName = "agent.yml"

// Identity is persisted at ~/.beacon/config/agent.yml (mode 0600 when written).
type Identity struct {
	ServerURL   string `yaml:"server_url,omitempty"`
	DeviceName  string `yaml:"device_name,omitempty"`
	DeviceToken string `yaml:"device_token,omitempty"`
	DeviceID    string `yaml:"device_id,omitempty"`
}

func beaconConfigDir() (string, error) {
	base, err := config.BeaconHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "config"), nil
}

// AgentYAMLPath returns the absolute path to agent.yml.
func AgentYAMLPath() (string, error) {
	dir, err := beaconConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, agentYAMLName), nil
}

// LoadAgent reads agent.yml. Returns (nil, nil) if the file is missing.
func LoadAgent() (*Identity, error) {
	p, err := AgentYAMLPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var id Identity
	if err := yaml.Unmarshal(data, &id); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	return &id, nil
}

// Save writes identity to the default agent.yml path.
func (i *Identity) Save() error {
	p, err := AgentYAMLPath()
	if err != nil {
		return err
	}
	return i.SaveAs(p)
}

// SaveAs writes identity to path (parent dirs created). File mode 0600.
func (i *Identity) SaveAs(path string) error {
	if i == nil {
		return fmt.Errorf("identity is nil")
	}
	path = filepath.Clean(path)
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}
	}
	data, err := yaml.Marshal(i)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// EffectiveDeviceName returns a non-empty device label for the API.
func (i *Identity) EffectiveDeviceName(fallback string) string {
	if i != nil {
		if s := strings.TrimSpace(i.DeviceName); s != "" {
			return s
		}
	}
	return strings.TrimSpace(fallback)
}
