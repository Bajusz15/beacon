package plugins

import (
	"fmt"
	"log"
	"sync"
	"time"

	"beacon/internal/errors"
)

// Manager handles plugin registration, initialization, and alert routing
type Manager struct {
	plugins     map[string]Plugin
	configs     map[string]*PluginConfig
	rules       []AlertRule
	mu          sync.RWMutex
	lastAlert   map[string]time.Time // Track last alert time for cooldown
	cooldowns   map[string]time.Duration
}

// NewManager creates a new plugin manager
func NewManager() *Manager {
	return &Manager{
		plugins:   make(map[string]Plugin),
		configs:   make(map[string]*PluginConfig),
		rules:     make([]AlertRule, 0),
		lastAlert: make(map[string]time.Time),
		cooldowns: make(map[string]time.Duration),
	}
}

// RegisterPlugin registers a plugin with the manager
func (m *Manager) RegisterPlugin(plugin Plugin) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	name := plugin.Name()
	if _, exists := m.plugins[name]; exists {
		return fmt.Errorf("plugin %s already registered", name)
	}
	
	m.plugins[name] = plugin
	log.Printf("[Beacon] Registered plugin: %s", name)
	return nil
}

// LoadConfigs loads plugin configurations from the monitor config
func (m *Manager) LoadConfigs(configs []PluginConfig, rules []AlertRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Clear existing configs
	m.configs = make(map[string]*PluginConfig)
	m.rules = rules
	
	// Parse cooldown durations
	for _, rule := range rules {
		if rule.Cooldown != "" {
			duration, err := time.ParseDuration(rule.Cooldown)
			if err != nil {
				beaconErr := errors.NewBeaconError(errors.ErrorTypeConfig, "Invalid cooldown duration", err).
					WithTroubleshooting(
						"Invalid duration format",
						"Unsupported time unit",
					).WithNextSteps(
						"Use valid duration format (e.g., '5m', '1h', '30s')",
						"Check alert rule configuration",
					)
				return beaconErr
			}
			m.cooldowns[rule.Check] = duration
		}
	}
	
	// Load plugin configs
	for _, config := range configs {
		if config.Enabled {
			m.configs[config.Name] = &config
			
			// Initialize plugin if registered
			if plugin, exists := m.plugins[config.Name]; exists {
				if err := plugin.Init(config.Config); err != nil {
					beaconErr := errors.NewPluginError(config.Name, err)
					return beaconErr
				}
				log.Printf("[Beacon] Initialized plugin: %s", config.Name)
			} else {
				log.Printf("[Beacon] Warning: plugin %s not registered", config.Name)
			}
		}
	}
	
	return nil
}

// SendAlert sends an alert to all configured plugins based on alert rules
func (m *Manager) SendAlert(checkResult *CheckResult) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Find applicable rules for this check
	var applicableRules []AlertRule
	for _, rule := range m.rules {
		if rule.Check == checkResult.Name {
			applicableRules = append(applicableRules, rule)
		}
	}
	
	// If no specific rules, check if check failed and send to all enabled plugins
	if len(applicableRules) == 0 && checkResult.Status != "up" {
		// Create a default rule for failed checks
		applicableRules = []AlertRule{{
			Check:    checkResult.Name,
			Severity: SeverityWarning,
			Plugins:  m.getEnabledPluginNames(),
		}}
	}
	
	// Process each applicable rule
	for _, rule := range applicableRules {
		if err := m.processRule(rule, checkResult); err != nil {
			log.Printf("[Beacon] Error processing alert rule for %s: %v", rule.Check, err)
		}
	}
	
	return nil
}

// processRule processes a single alert rule
func (m *Manager) processRule(rule AlertRule, checkResult *CheckResult) error {
	// Check cooldown
	if cooldown, exists := m.cooldowns[rule.Check]; exists {
		lastAlertTime, exists := m.lastAlert[rule.Check]
		if exists && time.Since(lastAlertTime) < cooldown {
			log.Printf("[Beacon] Alert for %s in cooldown period", rule.Check)
			return nil
		}
	}
	
	// Create alert
	alert := Alert{
		Title:     fmt.Sprintf("Beacon Alert: %s", checkResult.Name),
		Message:   m.formatAlertMessage(checkResult),
		Severity:  rule.Severity,
		Timestamp: time.Now(),
		Device:    checkResult.Device,
		Check:     checkResult,
		Metadata: map[string]interface{}{
			"rule": rule.Check,
		},
	}
	
	// Send to all plugins specified in the rule
	var errors []error
	for _, pluginName := range rule.Plugins {
		if plugin, exists := m.plugins[pluginName]; exists {
			if err := plugin.SendAlert(alert); err != nil {
				errors = append(errors, &PluginError{
					PluginName: pluginName,
					Message:    "failed to send alert",
					Err:        err,
				})
			} else {
				log.Printf("[Beacon] Alert sent via plugin: %s", pluginName)
			}
		} else {
			errors = append(errors, &PluginError{
				PluginName: pluginName,
				Message:    "plugin not found",
			})
		}
	}
	
	// Update last alert time
	m.lastAlert[rule.Check] = time.Now()
	
	// Return combined errors if any
	if len(errors) > 0 {
		return fmt.Errorf("failed to send alerts: %v", errors)
	}
	
	return nil
}

// formatAlertMessage formats the alert message based on check result
func (m *Manager) formatAlertMessage(checkResult *CheckResult) string {
	switch checkResult.Status {
	case "up":
		return fmt.Sprintf("Check '%s' is now UP (was down)", checkResult.Name)
	case "down":
		return fmt.Sprintf("Check '%s' is DOWN: %s", checkResult.Name, checkResult.Error)
	case "error":
		return fmt.Sprintf("Check '%s' encountered an ERROR: %s", checkResult.Name, checkResult.Error)
	default:
		return fmt.Sprintf("Check '%s' status changed to %s", checkResult.Name, checkResult.Status)
	}
}

// getEnabledPluginNames returns names of all enabled plugins
func (m *Manager) getEnabledPluginNames() []string {
	var names []string
	for name, config := range m.configs {
		if config.Enabled {
			names = append(names, name)
		}
	}
	return names
}

// HealthCheck performs health checks on all enabled plugins
func (m *Manager) HealthCheck() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var errors []error
	for name, plugin := range m.plugins {
		if config, exists := m.configs[name]; exists && config.Enabled {
			if err := plugin.HealthCheck(); err != nil {
				errors = append(errors, &PluginError{
					PluginName: name,
					Message:    "health check failed",
					Err:        err,
				})
			}
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("plugin health check failures: %v", errors)
	}
	
	return nil
}

// Close closes all plugins
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	var errors []error
	for name, plugin := range m.plugins {
		if err := plugin.Close(); err != nil {
			errors = append(errors, &PluginError{
				PluginName: name,
				Message:    "failed to close",
				Err:        err,
			})
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("plugin close errors: %v", errors)
	}
	
	return nil
}

// GetPluginStatus returns the status of all plugins
func (m *Manager) GetPluginStatus() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	status := make(map[string]interface{})
	for name, plugin := range m.plugins {
		pluginStatus := map[string]interface{}{
			"name":    name,
			"enabled": false,
			"healthy": false,
		}
		
		if config, exists := m.configs[name]; exists {
			pluginStatus["enabled"] = config.Enabled
		}
		
		if err := plugin.HealthCheck(); err == nil {
			pluginStatus["healthy"] = true
		} else {
			pluginStatus["error"] = err.Error()
		}
		
		status[name] = pluginStatus
	}
	
	return status
}
