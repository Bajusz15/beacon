package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"beacon/internal/plugins"
	"beacon/internal/util"
)

// TelegramPlugin implements the Plugin interface for Telegram bot API
type TelegramPlugin struct {
	name       string
	botToken   string
	chatID     string
	httpClient *http.Client
	apiURL     string
}

// TelegramMessage represents a Telegram message payload
type TelegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

// TelegramResponse represents the response from Telegram API
type TelegramResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
	ErrorCode   int    `json:"error_code,omitempty"`
}

// NewTelegramPlugin creates a new Telegram plugin instance
func NewTelegramPlugin() *TelegramPlugin {
	return &TelegramPlugin{
		name: "telegram",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		apiURL: "https://api.telegram.org/bot",
	}
}

// Name returns the plugin name
func (p *TelegramPlugin) Name() string {
	return p.name
}

// Init initializes the Telegram plugin with configuration
func (p *TelegramPlugin) Init(config map[string]interface{}) error {
	botToken, ok := config["bot_token"].(string)
	if !ok || botToken == "" {
		return fmt.Errorf("bot_token is required for Telegram plugin")
	}

	chatID, ok := config["chat_id"].(string)
	if !ok || chatID == "" {
		return fmt.Errorf("chat_id is required for Telegram plugin")
	}

	// Expand environment variables
	p.botToken = os.ExpandEnv(botToken)
	p.chatID = os.ExpandEnv(chatID)

	// Validate bot token format (should be numeric)
	if !isNumeric(p.botToken) {
		return fmt.Errorf("invalid bot token format")
	}

	// Validate chat ID format (should be numeric or start with @)
	if !isNumeric(p.chatID) && !strings.HasPrefix(p.chatID, "@") {
		return fmt.Errorf("invalid chat ID format")
	}

	return nil
}

// SendAlert sends an alert to Telegram
func (p *TelegramPlugin) SendAlert(alert plugins.Alert) error {
	if p.botToken == "" || p.chatID == "" {
		return fmt.Errorf("Telegram plugin not initialized")
	}

	message := p.buildMessage(alert)

	payload := TelegramMessage{
		ChatID:    p.chatID,
		Text:      message,
		ParseMode: "Markdown",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Telegram payload: %v", err)
	}

	url := fmt.Sprintf("%s%s/sendMessage", p.apiURL, p.botToken)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create Telegram request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Telegram message: %v", err)
	}
	defer util.DeferClose(resp.Body, "HTTP response body")()

	var telegramResp TelegramResponse
	if err := json.NewDecoder(resp.Body).Decode(&telegramResp); err != nil {
		return fmt.Errorf("failed to decode Telegram response: %v", err)
	}

	if !telegramResp.OK {
		return fmt.Errorf("Telegram API error: %s (code: %d)", telegramResp.Description, telegramResp.ErrorCode)
	}

	return nil
}

// buildMessage builds the Telegram message from an alert
func (p *TelegramPlugin) buildMessage(alert plugins.Alert) string {
	var message strings.Builder

	// Add emoji based on severity
	emoji := p.getSeverityEmoji(alert.Severity)
	message.WriteString(fmt.Sprintf("%s *%s*\n\n", emoji, alert.Title))

	// Add main message
	message.WriteString(fmt.Sprintf("%s\n\n", alert.Message))

	// Add device information
	message.WriteString("*Device Information:*\n")
	message.WriteString(fmt.Sprintf("• Name: %s\n", escapeMarkdown(alert.Device.Name)))

	if alert.Device.Location != "" {
		message.WriteString(fmt.Sprintf("• Location: %s\n", escapeMarkdown(alert.Device.Location)))
	}

	message.WriteString(fmt.Sprintf("• Severity: %s\n", strings.ToUpper(alert.Severity)))

	// Add check details if available
	if alert.Check != nil {
		message.WriteString("\n*Check Details:*\n")
		message.WriteString(fmt.Sprintf("• Name: %s\n", escapeMarkdown(alert.Check.Name)))
		message.WriteString(fmt.Sprintf("• Type: %s\n", escapeMarkdown(alert.Check.Type)))
		message.WriteString(fmt.Sprintf("• Status: %s\n", strings.ToUpper(alert.Check.Status)))

		if alert.Check.Duration > 0 {
			message.WriteString(fmt.Sprintf("• Duration: %s\n", alert.Check.Duration.String()))
		}

		if alert.Check.Error != "" {
			message.WriteString(fmt.Sprintf("• Error: `%s`\n", escapeMarkdown(alert.Check.Error)))
		}
	}

	// Add tags if available
	if len(alert.Device.Tags) > 0 {
		message.WriteString(fmt.Sprintf("\n*Tags:* %s\n", strings.Join(alert.Device.Tags, ", ")))
	}

	// Add timestamp
	message.WriteString(fmt.Sprintf("\n*Time:* %s", alert.Timestamp.Format("2006-01-02 15:04:05 MST")))

	return message.String()
}

// getSeverityEmoji returns the emoji for a severity level
func (p *TelegramPlugin) getSeverityEmoji(severity string) string {
	switch strings.ToLower(severity) {
	case "critical":
		return "🚨"
	case "warning":
		return "⚠️"
	case "info":
		return "ℹ️"
	default:
		return "📢"
	}
}

// escapeMarkdown escapes special Markdown characters
func escapeMarkdown(text string) string {
	// Escape special Markdown characters
	text = strings.ReplaceAll(text, "_", "\\_")
	text = strings.ReplaceAll(text, "*", "\\*")
	text = strings.ReplaceAll(text, "[", "\\[")
	text = strings.ReplaceAll(text, "]", "\\]")
	text = strings.ReplaceAll(text, "(", "\\(")
	text = strings.ReplaceAll(text, ")", "\\)")
	text = strings.ReplaceAll(text, "~", "\\~")
	text = strings.ReplaceAll(text, "`", "\\`")
	text = strings.ReplaceAll(text, ">", "\\>")
	text = strings.ReplaceAll(text, "#", "\\#")
	text = strings.ReplaceAll(text, "+", "\\+")
	text = strings.ReplaceAll(text, "-", "\\-")
	text = strings.ReplaceAll(text, "=", "\\=")
	text = strings.ReplaceAll(text, "|", "\\|")
	text = strings.ReplaceAll(text, "{", "\\{")
	text = strings.ReplaceAll(text, "}", "\\}")
	text = strings.ReplaceAll(text, ".", "\\.")
	text = strings.ReplaceAll(text, "!", "\\!")
	return text
}

// isNumeric checks if a string contains only numeric characters
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// HealthCheck verifies the Telegram plugin is working
func (p *TelegramPlugin) HealthCheck() error {
	if p.botToken == "" || p.chatID == "" {
		return fmt.Errorf("Telegram plugin not initialized")
	}

	// Test bot by sending a simple message
	testMessage := TelegramMessage{
		ChatID:    p.chatID,
		Text:      "Beacon Telegram plugin health check",
		ParseMode: "Markdown",
	}

	jsonData, err := json.Marshal(testMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal test message: %v", err)
	}

	url := fmt.Sprintf("%s%s/sendMessage", p.apiURL, p.botToken)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create test request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send test message: %v", err)
	}
	defer util.DeferClose(resp.Body, "HTTP response body")()

	var telegramResp TelegramResponse
	if err := json.NewDecoder(resp.Body).Decode(&telegramResp); err != nil {
		return fmt.Errorf("failed to decode test response: %v", err)
	}

	if !telegramResp.OK {
		return fmt.Errorf("test message failed: %s (code: %d)", telegramResp.Description, telegramResp.ErrorCode)
	}

	return nil
}

// Close cleans up the Telegram plugin
func (p *TelegramPlugin) Close() error {
	// Nothing to clean up for Telegram plugin
	return nil
}
