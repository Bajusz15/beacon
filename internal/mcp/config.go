package mcp

import (
	"os"
	"path/filepath"

	beaconcfg "beacon/internal/config"

	"gopkg.in/yaml.v3"
)

// Config holds MCP server configuration
type Config struct {
	DeployEnabled  bool     `yaml:"deploy_enabled"`
	RestartEnabled bool     `yaml:"restart_enabled"`
	AllowedTools   []string `yaml:"allowed_tools,omitempty"`
	AuditLogPath   string   `yaml:"audit_log_path,omitempty"`
}

// LoadConfig loads MCP config from the given path (e.g. ~/.beacon/mcp.yml)
func LoadConfig(configPath string) *Config {
	if configPath == "" {
		base, err := beaconcfg.BeaconHomeDir()
		if err == nil {
			configPath = filepath.Join(base, "mcp.yml")
		}
	}
	if configPath == "" {
		return DefaultConfig()
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return DefaultConfig()
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig()
	}
	return &cfg
}

// DefaultConfig returns the default MCP configuration
func DefaultConfig() *Config {
	return &Config{
		DeployEnabled:  os.Getenv("BEACON_MCP_DEPLOY_ENABLED") == "1",
		RestartEnabled: os.Getenv("BEACON_MCP_RESTART_ENABLED") == "1",
		AllowedTools:   nil,
		AuditLogPath:   "",
	}
}

// IsToolAllowed returns true if the tool is allowed
func (c *Config) IsToolAllowed(tool string) bool {
	if len(c.AllowedTools) == 0 {
		return true
	}
	for _, t := range c.AllowedTools {
		if t == tool {
			return true
		}
	}
	return false
}

// GetAuditLogPath returns the audit log path, defaulting to ~/.beacon/logs/mcp_audit.jsonl
func (c *Config) GetAuditLogPath() string {
	if c.AuditLogPath != "" {
		return c.AuditLogPath
	}
	base, err := beaconcfg.BeaconHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "logs", "mcp_audit.jsonl")
}
