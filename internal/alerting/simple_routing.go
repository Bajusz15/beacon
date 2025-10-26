package alerting

import (
	"fmt"
	"log"
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
	Channels         []string      `yaml:"channels"`          // email, discord, telegram, webhook
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

// SimpleAlertManager manages simple alert routing
type SimpleAlertManager struct {
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
	for _, route := range routing {
		if route.Severity == "" {
			return fmt.Errorf("alert routing severity cannot be empty")
		}

		// Validate channels
		validChannels := map[string]bool{
			"email": true, "discord": true, "telegram": true,
			"webhook": true, "slack": true,
		}
		for _, channel := range route.Channels {
			if !validChannels[channel] {
				return fmt.Errorf("invalid alert channel: %s", channel)
			}
		}

		sam.routing[route.Severity] = &route
	}
	return nil
}

// ProcessAlert processes a new alert and routes it appropriately
func (sam *SimpleAlertManager) ProcessAlert(context AlertContext) error {
	// Check if we have routing for this severity
	routing, exists := sam.routing[context.Severity]
	if !exists || !routing.Enabled {
		return fmt.Errorf("no routing configured for severity: %s", context.Severity)
	}

	// Check cooldown (simple per-service cooldown)
	cooldownKey := fmt.Sprintf("%s:%s", context.Service, context.Severity)
	if lastAlert, exists := sam.cooldowns[cooldownKey]; exists {
		if time.Since(lastAlert) < 5*time.Minute { // 5 minute cooldown
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

	sam.activeAlerts[context.AlertID] = activeAlert
	sam.cooldowns[cooldownKey] = time.Now()

	// Send alert to configured channels
	return sam.sendAlert(routing, context)
}

// sendAlert sends an alert via the configured channels
func (sam *SimpleAlertManager) sendAlert(routing *AlertRouting, context AlertContext) error {
	recipients := routing.Recipients

	for _, channel := range routing.Channels {
		err := sam.sendToChannel(channel, recipients, context)
		if err != nil {
			log.Printf("[ALERT] Failed to send via %s: %v", channel, err)
			// Continue with other channels even if one fails
		}
	}

	return nil
}

// sendToChannel sends an alert via a specific channel
func (sam *SimpleAlertManager) sendToChannel(channel string, recipients []string, context AlertContext) error {
	switch channel {
	case "email":
		return sam.sendEmailAlert(recipients, context)
	case "discord":
		return sam.sendDiscordAlert(recipients, context)
	case "telegram":
		return sam.sendTelegramAlert(recipients, context)
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
	return sam.activeAlerts
}

// GetAlertStatus returns the status of a specific alert
func (sam *SimpleAlertManager) GetAlertStatus(alertID string) (*ActiveAlert, error) {
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

func (sam *SimpleAlertManager) sendDiscordAlert(recipients []string, context AlertContext) error {
	fmt.Printf("[ALERT] 💬 Discord alert to %v: %s (%s)\n", recipients, context.Message, context.Severity)
	return nil
}

func (sam *SimpleAlertManager) sendTelegramAlert(recipients []string, context AlertContext) error {
	fmt.Printf("[ALERT] 📱 Telegram alert to %v: %s (%s)\n", recipients, context.Message, context.Severity)
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
