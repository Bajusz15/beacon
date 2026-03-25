package identity

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const globalFileName = "global.yml"

// GlobalPath returns ~/.beacon/config/global.yml
func GlobalPath() (string, error) {
	dir, err := beaconConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, globalFileName), nil
}

// GlobalSettings is machine-wide Beacon configuration (not tied to a project).
type GlobalSettings struct {
	CloudReportingEnabled bool   `yaml:"cloud_reporting_enabled"`
	HeartbeatInterval     string `yaml:"heartbeat_interval,omitempty"` // e.g. "60s"
}

// DefaultGlobal returns defaults used when global.yml is missing.
func DefaultGlobal() GlobalSettings {
	return GlobalSettings{
		CloudReportingEnabled: true,
		HeartbeatInterval:     "60s",
	}
}

// HeartbeatDuration parses HeartbeatInterval or returns 60s.
func (g *GlobalSettings) HeartbeatDuration() time.Duration {
	if g == nil {
		return 60 * time.Second
	}
	s := g.HeartbeatInterval
	if s == "" {
		s = "60s"
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < time.Second {
		return 60 * time.Second
	}
	return d
}

// LoadGlobal reads global.yml. If missing, returns defaults and nil error.
func LoadGlobal() (GlobalSettings, error) {
	p, err := GlobalPath()
	if err != nil {
		return GlobalSettings{}, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultGlobal(), nil
		}
		return GlobalSettings{}, err
	}
	var g GlobalSettings
	if err := yaml.Unmarshal(data, &g); err != nil {
		return GlobalSettings{}, fmt.Errorf("parse %s: %w", p, err)
	}
	if g.HeartbeatInterval == "" {
		g.HeartbeatInterval = "60s"
	}
	return g, nil
}

// SaveGlobal writes global settings (mode 0644).
func SaveGlobal(g GlobalSettings) error {
	p, err := GlobalPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	if g.HeartbeatInterval == "" {
		g.HeartbeatInterval = "60s"
	}
	data, err := yaml.Marshal(&g)
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
