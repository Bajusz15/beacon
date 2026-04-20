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
	HeartbeatInterval     int             `yaml:"heartbeat_interval,omitempty"`
	CloudReportingEnabled bool            `yaml:"cloud_reporting_enabled"`
	DeviceID              string          `yaml:"device_id,omitempty"`
	MetricsPort           int             `yaml:"metrics_port,omitempty"`
	MetricsListenAddr     string          `yaml:"metrics_listen_addr,omitempty"` // default "127.0.0.1"; set "0.0.0.0" for Docker
	LogLevel              string          `yaml:"log_level,omitempty"`           // debug|info|warn|error; default "info"
	Projects              []ProjectConfig `yaml:"projects,omitempty"`
	Tunnels               []TunnelConfig  `yaml:"tunnels,omitempty"`
	VPN                   *VPNConfig      `yaml:"vpn,omitempty"`
	// SystemMetrics configures host metrics sent with cloud heartbeats (~/.beacon/config.yaml only).
	// Per-project monitor.yml should not duplicate this; omit system_metrics there.
	SystemMetrics *UserSystemMetricsConfig `yaml:"system_metrics,omitempty"`
}

// UserSystemMetricsConfig is the ~/.beacon/config.yaml block for CPU/memory/disk reporting to BeaconInfra.
type UserSystemMetricsConfig struct {
	Enabled     bool          `yaml:"enabled"`
	Interval    time.Duration `yaml:"interval,omitempty"`
	CPU         bool          `yaml:"cpu,omitempty"`
	Memory      bool          `yaml:"memory,omitempty"`
	Disk        bool          `yaml:"disk,omitempty"`
	LoadAverage bool          `yaml:"load_average,omitempty"`
	DiskPath    string        `yaml:"disk_path,omitempty"`
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

// VPNConfig is the per-device WireGuard VPN section in ~/.beacon/config.yaml.
// The master watches this struct: a non-nil Enabled toggles the VPN manager,
// Role and PeerDevice describe the topology, and VPNAddress is filled in by
// the server after the first /api/agent/vpn/register call.
type VPNConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Role       string `yaml:"role"`                  // exit_node | client
	PeerDevice string `yaml:"peer_device,omitempty"` // client mode: device-name to connect to
	ListenPort int    `yaml:"listen_port,omitempty"` // default 51820
	VPNAddress string `yaml:"vpn_address,omitempty"` // assigned by server, cached locally
}

// TunnelConfig defines a tunnel the master can open to the cloud on demand (tunnel_connect piggyback only).
type TunnelConfig struct {
	ID        string          `yaml:"id"`
	LocalPort int             `yaml:"local_port,omitempty"`
	Upstream  *TunnelUpstream `yaml:"upstream,omitempty"`
	// Enabled is tri-state: nil => omitted in YAML (default: true), true/false => explicitly set.
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

func ensureDefaultSystemMetrics(f *UserConfig) {
	if f == nil || f.SystemMetrics != nil {
		return
	}
	f.SystemMetrics = defaultUserSystemMetrics()
}

func defaultUserSystemMetrics() *UserSystemMetricsConfig {
	return &UserSystemMetricsConfig{
		Enabled:     true,
		Interval:    time.Minute,
		CPU:         true,
		Memory:      true,
		Disk:        true,
		LoadAverage: true,
		DiskPath:    "/",
	}
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

// EffectiveCloudAPIBase returns the compile-time API base URL for heartbeats.
// The URL is baked into the binary and cannot be overridden at runtime (security).
func (uc *UserConfig) EffectiveCloudAPIBase() string {
	if uc == nil {
		return ""
	}
	return cloud.BeaconInfraAPIBase()
}

// WriteUserLocalInit writes or merges ~/.beacon/config.yaml for local identity fields (device name, etc.).
// Does not require an API key. On a brand-new config file, sets cloud_reporting_enabled to false.
// If the file already exists, preserves api_key, projects, tunnels, cloud_reporting_enabled, and other fields.
func WriteUserLocalInit(deviceName string, metricsPort int) error {
	name := strings.TrimSpace(deviceName)
	if name == "" {
		name = DetectHostname()
		if name == "" {
			return errors.New("device name is required (--name) or hostname must be set")
		}
	}
	p, err := UserConfigPath()
	if err != nil {
		return err
	}
	f, err := readExistingUserConfig(p)
	if err != nil {
		return err
	}
	newFile := f == nil
	if f == nil {
		f = &UserConfig{}
	}
	f.DeviceName = name
	if newFile {
		f.CloudReportingEnabled = false
	}
	if f.HeartbeatInterval <= 0 {
		f.HeartbeatInterval = 30
	}
	if metricsPort > 0 {
		f.MetricsPort = metricsPort
	}
	ensureDefaultSystemMetrics(f)
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

// AppendTunnelIfMissing adds or updates a tunnel entry in ~/.beacon/config.yaml.
func AppendTunnelIfMissing(tunnelID string, localPort int) error {
	tunnelID = strings.TrimSpace(tunnelID)
	if tunnelID == "" {
		return errors.New("tunnel id is required")
	}
	if localPort <= 0 || localPort > 65535 {
		return errors.New("local_port must be between 1 and 65535")
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
	for i := range f.Tunnels {
		if f.Tunnels[i].ID == tunnelID {
			f.Tunnels[i].LocalPort = localPort
			f.Tunnels[i].Upstream = nil
			return saveUserConfig(p, f)
		}
	}
	f.Tunnels = append(f.Tunnels, TunnelConfig{ID: tunnelID, LocalPort: localPort})
	return saveUserConfig(p, f)
}

// UpsertTunnelUpstream sets or adds a tunnel with an explicit upstream (LAN/Docker or loopback).
// protocol is http or https; empty host defaults to 127.0.0.1 in EffectiveUpstream.
func UpsertTunnelUpstream(tunnelID, protocol, host string, port int) error {
	tunnelID = strings.TrimSpace(tunnelID)
	if tunnelID == "" {
		return errors.New("tunnel id is required")
	}
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" {
		protocol = "http"
	}
	if protocol != "http" && protocol != "https" {
		return errors.New("protocol must be http or https")
	}
	if port <= 0 || port > 65535 {
		return errors.New("port must be between 1 and 65535")
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
	u := &TunnelUpstream{Protocol: protocol, Host: strings.TrimSpace(host), Port: port}
	for i := range f.Tunnels {
		if f.Tunnels[i].ID == tunnelID {
			f.Tunnels[i].Upstream = u
			f.Tunnels[i].LocalPort = 0
			return saveUserConfig(p, f)
		}
	}
	f.Tunnels = append(f.Tunnels, TunnelConfig{ID: tunnelID, Upstream: u})
	return saveUserConfig(p, f)
}

// RemoveTunnel removes a tunnel entry from ~/.beacon/config.yaml.
func RemoveTunnel(tunnelID string) error {
	tunnelID = strings.TrimSpace(tunnelID)
	if tunnelID == "" {
		return errors.New("tunnel id is required")
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
		return nil
	}
	filtered := f.Tunnels[:0]
	for _, t := range f.Tunnels {
		if t.ID != tunnelID {
			filtered = append(filtered, t)
		}
	}
	f.Tunnels = filtered
	return saveUserConfig(p, f)
}

// SetTunnelEnabled enables or disables a tunnel in ~/.beacon/config.yaml.
func SetTunnelEnabled(tunnelID string, enabled bool) error {
	tunnelID = strings.TrimSpace(tunnelID)
	if tunnelID == "" {
		return errors.New("tunnel id is required")
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
		return fmt.Errorf("tunnel %q not found", tunnelID)
	}
	for i := range f.Tunnels {
		if f.Tunnels[i].ID == tunnelID {
			f.Tunnels[i].Enabled = &enabled
			return saveUserConfig(p, f)
		}
	}
	return fmt.Errorf("tunnel %q not found", tunnelID)
}

// SetVPNExitNode marks this device as an exit node in ~/.beacon/config.yaml.
// `beacon vpn enable` calls this; the master picks the change up on its next config reload.
func SetVPNExitNode(listenPort int, vpnAddress string) error {
	if listenPort <= 0 {
		listenPort = 51820
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
	f.VPN = &VPNConfig{
		Enabled:    true,
		Role:       "exit_node",
		ListenPort: listenPort,
		VPNAddress: strings.TrimSpace(vpnAddress),
	}
	return saveUserConfig(p, f)
}

// SetVPNClient marks this device as a VPN client of the given peer device.
func SetVPNClient(peerDevice string, vpnAddress string) error {
	peerDevice = strings.TrimSpace(peerDevice)
	if peerDevice == "" {
		return errors.New("peer device is required")
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
	f.VPN = &VPNConfig{
		Enabled:    true,
		Role:       "client",
		PeerDevice: peerDevice,
		ListenPort: 51820,
		VPNAddress: strings.TrimSpace(vpnAddress),
	}
	return saveUserConfig(p, f)
}

// ClearVPN removes the VPN block from ~/.beacon/config.yaml. Idempotent.
func ClearVPN() error {
	p, err := UserConfigPath()
	if err != nil {
		return err
	}
	f, err := readExistingUserConfig(p)
	if err != nil {
		return err
	}
	if f == nil {
		return nil
	}
	f.VPN = nil
	return saveUserConfig(p, f)
}

// WriteCloudLogin writes BeaconInfra API credentials and enables cloud reporting.
// The cloud URL is baked into the binary at compile time and cannot be overridden.
// If deviceName is empty and config already has a device_name, the existing name is preserved.
func WriteCloudLogin(apiKey, deviceName string) error {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return errors.New("api_key is required")
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

	// Only update device name if explicitly provided or not yet set
	name := strings.TrimSpace(deviceName)
	if name != "" {
		f.DeviceName = name
	}
	if strings.TrimSpace(f.DeviceName) == "" {
		f.DeviceName = DetectHostname()
		if f.DeviceName == "" {
			return errors.New("device name is required (--name) or hostname must be set")
		}
	}

	if f.HeartbeatInterval <= 0 {
		f.HeartbeatInterval = 30
	}
	f.CloudReportingEnabled = true
	ensureDefaultSystemMetrics(f)
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
