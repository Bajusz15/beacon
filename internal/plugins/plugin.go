package plugins

import (
	"fmt"
	"time"
)

// Plugin defines the interface that all alert plugins must implement
type Plugin interface {
	// Name returns the plugin name
	Name() string

	// Init initializes the plugin with the given configuration
	Init(config map[string]interface{}) error

	// SendAlert sends an alert notification
	SendAlert(alert Alert) error

	// HealthCheck verifies the plugin is working correctly
	HealthCheck() error

	// Close cleans up plugin resources
	Close() error
}

// Alert represents an alert notification
type Alert struct {
	Title     string                 `json:"title"`
	Message   string                 `json:"message"`
	Severity  string                 `json:"severity"` // critical, warning, info
	Timestamp time.Time              `json:"timestamp"`
	Device    DeviceConfig           `json:"device"`
	Check     *CheckResult           `json:"check,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// DeviceConfig represents device information
type DeviceConfig struct {
	Name        string   `json:"name"`
	Location    string   `json:"location,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Environment string   `json:"environment,omitempty"`
}

// CheckResult represents the result of a health check
type CheckResult struct {
	Name           string        `json:"name"`
	Type           string        `json:"type"`
	Status         string        `json:"status"` // "up", "down", "error"
	Duration       time.Duration `json:"duration"`
	Timestamp      time.Time     `json:"timestamp"`
	Error          string        `json:"error,omitempty"`
	HTTPStatusCode int           `json:"http_status_code,omitempty"`
	ResponseTime   time.Duration `json:"response_time,omitempty"`
	CommandOutput  string        `json:"command_output,omitempty"`
	CommandError   string        `json:"command_error,omitempty"`
	Device         DeviceConfig  `json:"device,omitempty"`
}

// PluginConfig represents the configuration for a plugin
type PluginConfig struct {
	Name    string                 `yaml:"name"`
	Enabled bool                   `yaml:"enabled"`
	Config  map[string]interface{} `yaml:",inline"`
}

// AlertRule defines when and how alerts should be triggered
type AlertRule struct {
	Check     string   `yaml:"check"`     // Name of the check to monitor
	Severity  string   `yaml:"severity"`  // Alert severity level
	Plugins   []string `yaml:"plugins"`   // List of plugins to notify
	Threshold string   `yaml:"threshold"` // Optional threshold condition
	Cooldown  string   `yaml:"cooldown"`  // Optional cooldown period
}

// PluginError represents an error from a plugin
type PluginError struct {
	PluginName string
	Message    string
	Err        error
}

func (e *PluginError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("plugin %s: %s: %v", e.PluginName, e.Message, e.Err)
	}
	return fmt.Sprintf("plugin %s: %s", e.PluginName, e.Message)
}

// Severity levels
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// Default alert template
const DefaultAlertTemplate = `🚨 Beacon Alert

**{{.Title}}**
{{.Message}}

**Device:** {{.Device.Name}}
**Severity:** {{.Severity}}
**Time:** {{.Timestamp.Format "2006-01-02 15:04:05 MST"}}

{{if .Check}}**Check Details:**
- Name: {{.Check.Name}}
- Type: {{.Check.Type}}
- Status: {{.Check.Status}}
- Duration: {{.Check.Duration}}
{{if .Check.Error}}- Error: {{.Check.Error}}{{end}}
{{end}}`
