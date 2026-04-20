package alerting

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"beacon/internal/logging"
)

var logger = logging.New("alerting")

// AlertSeverity represents the severity level of an alert
type AlertSeverity string

const (
	SeverityCritical AlertSeverity = "critical"
	SeverityWarning  AlertSeverity = "warning"
	SeverityInfo     AlertSeverity = "info"
)

const webhookSchemaVersion = "1"

// AlertRouting defines simple alert routing rules
type AlertRouting struct {
	Severity         AlertSeverity `yaml:"severity"`
	Channels         []string      `yaml:"channels"`          // email, webhook, slack
	Recipients       []string      `yaml:"recipients"`        // To addresses (email); slack/webhook ignore
	BackupDelay      time.Duration `yaml:"backup_delay"`      // delay before notifying backup (0 = disabled)
	BackupRecipients []string      `yaml:"backup_recipients"` // backup recipients
	Enabled          bool          `yaml:"enabled"`
}

// AlertContext contains information about the alert
type AlertContext struct {
	AlertID     string            `json:"alert_id"`
	ProjectID   string            `json:"project_id"`
	DeviceName  string            `json:"device_name"`
	Service     string            `json:"service"`
	Severity    AlertSeverity     `json:"severity"`
	Message     string            `json:"message"`
	Timestamp   time.Time         `json:"timestamp"`
	Tags        map[string]string `json:"tags"`
	Source      string            `json:"source"`      // beacon agent, manual, etc.
	Environment string            `json:"environment"` // prod, staging, dev
}

// WebhookPayloadV1 is the JSON body for POST alert_channels.webhook.url
type WebhookPayloadV1 struct {
	SchemaVersion string    `json:"schema_version"`
	Summary       string    `json:"summary"`
	AlertID       string    `json:"alert_id"`
	ProjectID     string    `json:"project_id"`
	DeviceName    string    `json:"device_name"`
	Severity      string    `json:"severity"`
	Message       string    `json:"message"`
	Service       string    `json:"service,omitempty"`
	Source        string    `json:"source,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

// SimpleAlertManager manages simple alert routing (concurrency-safe)
type SimpleAlertManager struct {
	mu           sync.RWMutex
	routing      map[AlertSeverity]*AlertRouting
	activeAlerts map[string]*ActiveAlert
	cooldowns    map[string]time.Time
	channels     alertChannelSettings
	httpClient   *http.Client
}

type alertChannelSettings struct {
	email   emailSettings
	webhook webhookSettings
}

type emailSettings struct {
	Enabled  bool
	SMTPHost string
	SMTPPort int
	User     string
	Password string
	From     string
}

type webhookSettings struct {
	Enabled bool
	URL     string
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
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// LoadChannels parses alert_channels from YAML (map) into the manager.
func (sam *SimpleAlertManager) LoadChannels(raw map[string]interface{}) {
	sam.mu.Lock()
	defer sam.mu.Unlock()
	sam.channels = parseAlertChannels(raw)
}

// LoadRouting loads alert routing configuration
func (sam *SimpleAlertManager) LoadRouting(routing []AlertRouting) error {
	sam.mu.Lock()
	defer sam.mu.Unlock()

	for _, route := range routing {
		if route.Severity == "" {
			return fmt.Errorf("alert routing severity cannot be empty")
		}

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

		r := route
		sam.routing[r.Severity] = &r
	}
	return nil
}

// ProcessAlert processes a new alert and routes it appropriately
func (sam *SimpleAlertManager) ProcessAlert(ctx AlertContext) error {
	sam.mu.RLock()
	routing, exists := sam.routing[ctx.Severity]
	sam.mu.RUnlock()
	if !exists || !routing.Enabled {
		return fmt.Errorf("no routing configured for severity: %s", ctx.Severity)
	}

	cooldownKey := fmt.Sprintf("%s:%s", ctx.Service, ctx.Severity)
	sam.mu.RLock()
	lastAlert, ok := sam.cooldowns[cooldownKey]
	sam.mu.RUnlock()
	if ok {
		if time.Since(lastAlert) < 5*time.Minute {
			logger.Infof("Alert in cooldown for %s", cooldownKey)
			return nil
		}
	}

	activeAlert := &ActiveAlert{
		AlertID:      ctx.AlertID,
		Context:      ctx,
		Routing:      routing,
		SentAt:       time.Now(),
		Acknowledged: false,
		Resolved:     false,
	}

	sam.mu.Lock()
	sam.activeAlerts[ctx.AlertID] = activeAlert
	sam.cooldowns[cooldownKey] = time.Now()
	sam.mu.Unlock()

	return sam.sendAlert(routing, ctx)
}

func (sam *SimpleAlertManager) sendAlert(routing *AlertRouting, ctx AlertContext) error {
	var sendErr error
	for _, channel := range routing.Channels {
		err := sam.sendToChannel(channel, routing.Recipients, ctx)
		if err != nil {
			logger.Infof("Failed to send via %s: %v", channel, err)
			sendErr = err
		}
	}
	return sendErr
}

func (sam *SimpleAlertManager) sendToChannel(channel string, recipients []string, ctx AlertContext) error {
	switch channel {
	case "email":
		return sam.sendEmailAlert(recipients, ctx)
	case "slack":
		return sam.sendSlackAlert(recipients, ctx)
	case "webhook":
		return sam.sendWebhookAlert(ctx)
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

func buildSummary(ctx AlertContext) string {
	p := ctx.ProjectID
	if p == "" {
		p = "(unknown-project)"
	}
	d := ctx.DeviceName
	if d == "" {
		d = "(unknown-device)"
	}
	return fmt.Sprintf("Problem on project %s on device %s: %s", p, d, ctx.Message)
}

func (sam *SimpleAlertManager) sendEmailAlert(recipients []string, ctx AlertContext) error {
	sam.mu.RLock()
	ch := sam.channels.email
	sam.mu.RUnlock()

	if !ch.Enabled {
		return nil
	}
	if ch.SMTPHost == "" {
		logger.Infof("Email channel enabled but smtp_host is empty; skipping")
		return nil
	}
	if len(recipients) == 0 {
		return fmt.Errorf("email channel: no recipients")
	}
	port := ch.SMTPPort
	if port <= 0 {
		port = 587
	}

	from := os.ExpandEnv(ch.From)
	if from == "" {
		from = "Beacon Alerts <beacon@localhost>"
	}

	user := os.ExpandEnv(ch.User)
	pass := os.ExpandEnv(ch.Password)
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, ch.SMTPHost)
	}

	subject := buildSummary(ctx)
	body := subject + "\n\n" +
		"Alert ID: " + ctx.AlertID + "\n" +
		"Service: " + ctx.Service + "\n" +
		"Severity: " + string(ctx.Severity) + "\n" +
		"Time: " + ctx.Timestamp.UTC().Format(time.RFC3339) + "\n"

	msg := bytes.NewBuffer(nil)
	fmt.Fprintf(msg, "From: %s\r\n", from)
	fmt.Fprintf(msg, "To: %s\r\n", strings.Join(recipients, ", "))
	fmt.Fprintf(msg, "Subject: %s\r\n", subject)
	fmt.Fprintf(msg, "Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprintf(msg, "\r\n")
	msg.WriteString(body)

	addr := net.JoinHostPort(ch.SMTPHost, strconv.Itoa(port))
	return sendSMTP(addr, auth, from, recipients, msg.Bytes())
}

func sendSMTP(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("smtp address %q: %w", addr, err)
	}
	if portStr == "465" {
		tlsCfg := &tls.Config{ServerName: host}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("smtp tls dial: %w", err)
		}
		defer conn.Close()
		c, err := smtp.NewClient(conn, host)
		if err != nil {
			return fmt.Errorf("smtp client: %w", err)
		}
		defer c.Close()
		if auth != nil {
			if ok, _ := c.Extension("AUTH"); ok {
				if err := c.Auth(auth); err != nil {
					return fmt.Errorf("smtp auth: %w", err)
				}
			}
		}
		if err := c.Mail(from); err != nil {
			return fmt.Errorf("smtp mail: %w", err)
		}
		for _, r := range to {
			if err := c.Rcpt(r); err != nil {
				return fmt.Errorf("smtp rcpt %s: %w", r, err)
			}
		}
		w, err := c.Data()
		if err != nil {
			return fmt.Errorf("smtp data: %w", err)
		}
		if _, err := w.Write(msg); err != nil {
			return err
		}
		return w.Close()
	}

	return smtp.SendMail(addr, auth, from, to, msg)
}

func (sam *SimpleAlertManager) sendSlackAlert(recipients []string, ctx AlertContext) error {
	logger.Infof("Slack alert (mock) channels=%v alert=%s severity=%s", recipients, ctx.AlertID, ctx.Severity)
	return nil
}

func (sam *SimpleAlertManager) sendWebhookAlert(ctx AlertContext) error {
	sam.mu.RLock()
	ch := sam.channels.webhook
	client := sam.httpClient
	sam.mu.RUnlock()

	if !ch.Enabled {
		return nil
	}
	url := strings.TrimSpace(os.ExpandEnv(ch.URL))
	if url == "" {
		logger.Infof("Webhook channel enabled but url is empty; skipping")
		return nil
	}

	payload := WebhookPayloadV1{
		SchemaVersion: webhookSchemaVersion,
		Summary:       buildSummary(ctx),
		AlertID:       ctx.AlertID,
		ProjectID:     ctx.ProjectID,
		DeviceName:    ctx.DeviceName,
		Severity:      string(ctx.Severity),
		Message:       ctx.Message,
		Service:       ctx.Service,
		Source:        ctx.Source,
		Timestamp:     ctx.Timestamp,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Beacon-Agent/Alerts")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func parseAlertChannels(raw map[string]interface{}) alertChannelSettings {
	var out alertChannelSettings
	if raw == nil {
		return out
	}
	if m, ok := raw["email"].(map[string]interface{}); ok {
		out.email = parseEmailSettings(m)
	}
	if m, ok := raw["webhook"].(map[string]interface{}); ok {
		out.webhook = parseWebhookSettings(m)
	}
	return out
}

func parseEmailSettings(m map[string]interface{}) emailSettings {
	var e emailSettings
	if v, ok := m["smtp_host"].(string); ok {
		e.SMTPHost = os.ExpandEnv(v)
	}
	e.SMTPPort = intFromYAML(m["smtp_port"], 587)
	if v, ok := m["smtp_user"].(string); ok {
		e.User = v
	}
	if v, ok := m["smtp_password"].(string); ok {
		e.Password = v
	}
	if v, ok := m["from"].(string); ok {
		e.From = v
	}
	if v, ok := m["enabled"].(bool); ok {
		e.Enabled = v
	}
	return e
}

func parseWebhookSettings(m map[string]interface{}) webhookSettings {
	var w webhookSettings
	if v, ok := m["url"].(string); ok {
		w.URL = os.ExpandEnv(v)
	}
	if v, ok := m["enabled"].(bool); ok {
		w.Enabled = v
	}
	return w
}

func intFromYAML(v interface{}, def int) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return def
	}
}
