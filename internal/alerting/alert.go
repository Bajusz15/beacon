package alerting

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// AlertSeverity represents the severity level of an alert
type AlertSeverity string

const (
	SeverityCritical AlertSeverity = "critical"
	SeverityWarning  AlertSeverity = "warning"
	SeverityInfo     AlertSeverity = "info"
)

// AlertRouting defines simple alert routing rules
type AlertRouting struct {
	Severity         AlertSeverity `yaml:"severity"`
	Channels         []string      `yaml:"channels"`          // email, webhook, slack
	Recipients       []string      `yaml:"recipients"`        // specific recipients for this severity
	BackupDelay      time.Duration `yaml:"backup_delay"`      // delay before notifying backup (0 = disabled)
	BackupRecipients []string      `yaml:"backup_recipients"` // backup recipients
	Enabled          bool          `yaml:"enabled"`
}

// AlertContext contains information about the alert
type AlertContext struct {
	AlertID     string            `json:"alert_id"`
	Service     string            `json:"service"`
	Severity    AlertSeverity     `json:"severity"`
	Message     string            `json:"message"`
	Timestamp   time.Time         `json:"timestamp"`
	Tags        map[string]string `json:"tags"`
	Source      string            `json:"source"`      // beacon agent, manual, etc.
	Environment string            `json:"environment"` // prod, staging, dev
}

// SimpleAlertManager manages simple alert routing (concurrency-safe)
type SimpleAlertManager struct {
	mu           sync.RWMutex
	routing      map[AlertSeverity]*AlertRouting
	activeAlerts map[string]*ActiveAlert
	cooldowns    map[string]time.Time
}

// ActiveAlert tracks an alert that's been sent
type ActiveAlert struct {
	AlertID        string
	Context        AlertContext
	Routing        *AlertRouting
	SentAt         time.Time
	BackupSentAt   time.Time
	Acknowledged   bool
	AcknowledgedBy string
	AcknowledgedAt time.Time
	Resolved       bool
	ResolvedAt     time.Time
}

// NewSimpleAlertManager creates a new simple alert manager
func NewSimpleAlertManager() *SimpleAlertManager {
	return &SimpleAlertManager{
		routing:      make(map[AlertSeverity]*AlertRouting),
		activeAlerts: make(map[string]*ActiveAlert),
		cooldowns:    make(map[string]time.Time),
	}
}

// LoadRouting loads alert routing configuration
func (sam *SimpleAlertManager) LoadRouting(routing []AlertRouting) error {
	sam.mu.Lock()
	defer sam.mu.Unlock()

	for _, route := range routing {
		if route.Severity == "" {
			return fmt.Errorf("alert routing severity cannot be empty")
		}

		// Validate channels
		validChannels := map[string]bool{
			"email":   true,
			"webhook": true,
			"slack":   true,
		}
		for _, channel := range route.Channels {
			if !validChannels[channel] {
				return fmt.Errorf("invalid alert channel: %s", channel)
			}
		}

		// Copy the loop variable before taking address to avoid pointing to the same variable.
		r := route
		sam.routing[r.Severity] = &r
	}
	return nil
}

// ProcessAlert processes a new alert and routes it appropriately
func (sam *SimpleAlertManager) ProcessAlert(context AlertContext) error {
	// Check if we have routing for this severity
	sam.mu.RLock()
	routing, exists := sam.routing[context.Severity]
	sam.mu.RUnlock()
	if !exists || !routing.Enabled {
		return fmt.Errorf("no routing configured for severity: %s", context.Severity)
	}

	// Check cooldown (simple per-service cooldown)
	cooldownKey := fmt.Sprintf("%s:%s", context.Service, context.Severity)
	sam.mu.RLock()
	lastAlert, exists := sam.cooldowns[cooldownKey]
	sam.mu.RUnlock()
	if exists {
		if time.Since(lastAlert) < 5*time.Minute { // 5 minute cooldown (consider making configurable)
			log.Printf("[ALERT] Alert in cooldown for %s", cooldownKey)
			return nil
		}
	}

	// Create active alert
	activeAlert := &ActiveAlert{
		AlertID:      context.AlertID,
		Context:      context,
		Routing:      routing,
		SentAt:       time.Now(),
		Acknowledged: false,
		Resolved:     false,
	}

	sam.mu.Lock()
	sam.activeAlerts[context.AlertID] = activeAlert
	sam.cooldowns[cooldownKey] = time.Now()
	sam.mu.Unlock()

	// Send alert to configured channels
	return sam.sendAlert(routing, context)
}

// sendAlert sends an alert via the configured channels
func (sam *SimpleAlertManager) sendAlert(routing *AlertRouting, context AlertContext) error {
	recipients := routing.Recipients

	var sendErr error
	for _, channel := range routing.Channels {
		err := sam.sendToChannel(channel, recipients, context)
		if err != nil {
			log.Printf("[ALERT] Failed to send via %s: %v", channel, err)
			// collect the last error; continue other channels
			sendErr = err
		}
	}

	return sendErr
}

// sendToChannel sends an alert via a specific channel
func (sam *SimpleAlertManager) sendToChannel(channel string, recipients []string, context AlertContext) error {
	switch channel {
	case "email":
		return sam.sendEmailAlert(recipients, context)
	case "slack":
		return sam.sendSlackAlert(recipients, context)
	case "webhook":
		return sam.sendWebhookAlert(recipients, context)
	default:
		return fmt.Errorf("unknown alert channel: %s", channel)
	}
}

// AcknowledgeAlert acknowledges an alert
func (sam *SimpleAlertManager) AcknowledgeAlert(alertID, acknowledgedBy string) error {
	sam.mu.Lock()
	defer sam.mu.Unlock()

	activeAlert, exists := sam.activeAlerts[alertID]
	if !exists {
		return fmt.Errorf("alert %s not found", alertID)
	}

	activeAlert.Acknowledged = true
	activeAlert.AcknowledgedBy = acknowledgedBy
	activeAlert.AcknowledgedAt = time.Now()

	return nil
}

// ResolveAlert resolves an alert
func (sam *SimpleAlertManager) ResolveAlert(alertID string) error {
	sam.mu.Lock()
	defer sam.mu.Unlock()

	activeAlert, exists := sam.activeAlerts[alertID]
	if !exists {
		return fmt.Errorf("alert %s not found", alertID)
	}

	activeAlert.Resolved = true
	activeAlert.ResolvedAt = time.Now()

	return nil
}

// GetActiveAlerts returns all active alerts
func (sam *SimpleAlertManager) GetActiveAlerts() map[string]*ActiveAlert {
	sam.mu.RLock()
	defer sam.mu.RUnlock()

	// Return a copy to avoid callers mutating internal state
	out := make(map[string]*ActiveAlert, len(sam.activeAlerts))
	for k, v := range sam.activeAlerts {
		out[k] = v
	}
	return out
}

// GetAlertStatus returns the status of a specific alert
func (sam *SimpleAlertManager) GetAlertStatus(alertID string) (*ActiveAlert, error) {
	sam.mu.RLock()
	defer sam.mu.RUnlock()

	activeAlert, exists := sam.activeAlerts[alertID]
	if !exists {
		return nil, fmt.Errorf("alert %s not found", alertID)
	}
	return activeAlert, nil
}

// Placeholder methods for actual alert sending (would integrate with existing plugins)
func (sam *SimpleAlertManager) sendEmailAlert(recipients []string, context AlertContext) error {
	fmt.Printf("[ALERT] 📧 Email alert to %v: %s (%s)\n", recipients, context.Message, context.Severity)
	return nil
}

func (sam *SimpleAlertManager) sendSlackAlert(recipients []string, context AlertContext) error {
	fmt.Printf("[ALERT] 💬 Slack alert to %v: %s (%s)\n", recipients, context.Message, context.Severity)
	return nil
}

func (sam *SimpleAlertManager) sendWebhookAlert(recipients []string, context AlertContext) error {
	fmt.Printf("[ALERT] 🔗 Webhook alert to %v: %s (%s)\n", recipients, context.Message, context.Severity)
	return nil
}
