package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"beacon/internal/plugins"
)

// DiscordPlugin implements the Plugin interface for Discord webhooks
type DiscordPlugin struct {
	name       string
	webhookURL string
	httpClient *http.Client
}

// DiscordWebhookPayload represents the Discord webhook payload structure
type DiscordWebhookPayload struct {
	Content string                 `json:"content,omitempty"`
	Embeds  []DiscordEmbed         `json:"embeds,omitempty"`
	Username string                `json:"username,omitempty"`
	AvatarURL string               `json:"avatar_url,omitempty"`
}

// DiscordEmbed represents a Discord embed
type DiscordEmbed struct {
	Title       string                 `json:"title,omitempty"`
	Description string                 `json:"description,omitempty"`
	Color       int                    `json:"color,omitempty"`
	Fields      []DiscordField         `json:"fields,omitempty"`
	Footer      DiscordFooter          `json:"footer,omitempty"`
	Timestamp   string                 `json:"timestamp,omitempty"`
}

// DiscordField represents a Discord embed field
type DiscordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// DiscordFooter represents a Discord embed footer
type DiscordFooter struct {
	Text string `json:"text"`
}

// NewDiscordPlugin creates a new Discord plugin instance
func NewDiscordPlugin() *DiscordPlugin {
	return &DiscordPlugin{
		name: "discord",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Name returns the plugin name
func (p *DiscordPlugin) Name() string {
	return p.name
}

// Init initializes the Discord plugin with configuration
func (p *DiscordPlugin) Init(config map[string]interface{}) error {
	webhookURL, ok := config["webhook_url"].(string)
	if !ok || webhookURL == "" {
		return fmt.Errorf("webhook_url is required for Discord plugin")
	}
	
	// Expand environment variables
	p.webhookURL = os.ExpandEnv(webhookURL)
	
	// Validate webhook URL format
	if !strings.HasPrefix(p.webhookURL, "https://discord.com/api/webhooks/") && 
	   !strings.HasPrefix(p.webhookURL, "https://discordapp.com/api/webhooks/") {
		return fmt.Errorf("invalid Discord webhook URL format")
	}
	
	return nil
}

// SendAlert sends an alert to Discord
func (p *DiscordPlugin) SendAlert(alert plugins.Alert) error {
	if p.webhookURL == "" {
		return fmt.Errorf("Discord plugin not initialized")
	}
	
	payload := p.buildPayload(alert)
	
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Discord payload: %v", err)
	}
	
	req, err := http.NewRequest("POST", p.webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create Discord request: %v", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Discord webhook: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Discord webhook returned status %d", resp.StatusCode)
	}
	
	return nil
}

// buildPayload builds the Discord webhook payload from an alert
func (p *DiscordPlugin) buildPayload(alert plugins.Alert) DiscordWebhookPayload {
	// Determine color based on severity
	color := p.getSeverityColor(alert.Severity)
	
	// Build embed
	embed := DiscordEmbed{
		Title:       alert.Title,
		Description: alert.Message,
		Color:       color,
		Timestamp:   alert.Timestamp.Format(time.RFC3339),
		Footer: DiscordFooter{
			Text: "Beacon Monitoring",
		},
	}
	
	// Add device information
	embed.Fields = append(embed.Fields, DiscordField{
		Name:   "Device",
		Value:  alert.Device.Name,
		Inline: true,
	})
	
	if alert.Device.Location != "" {
		embed.Fields = append(embed.Fields, DiscordField{
			Name:   "Location",
			Value:  alert.Device.Location,
			Inline: true,
		})
	}
	
	embed.Fields = append(embed.Fields, DiscordField{
		Name:   "Severity",
		Value:  strings.ToUpper(alert.Severity),
		Inline: true,
	})
	
	// Add check details if available
	if alert.Check != nil {
		embed.Fields = append(embed.Fields, DiscordField{
			Name:   "Check",
			Value:  alert.Check.Name,
			Inline: true,
		})
		
		embed.Fields = append(embed.Fields, DiscordField{
			Name:   "Type",
			Value:  alert.Check.Type,
			Inline: true,
		})
		
		embed.Fields = append(embed.Fields, DiscordField{
			Name:   "Status",
			Value:  strings.ToUpper(alert.Check.Status),
			Inline: true,
		})
		
		if alert.Check.Duration > 0 {
			embed.Fields = append(embed.Fields, DiscordField{
				Name:   "Duration",
				Value:  alert.Check.Duration.String(),
				Inline: true,
			})
		}
		
		if alert.Check.Error != "" {
			embed.Fields = append(embed.Fields, DiscordField{
				Name:   "Error",
				Value:  fmt.Sprintf("```%s```", alert.Check.Error),
				Inline: false,
			})
		}
	}
	
	// Add tags if available
	if len(alert.Device.Tags) > 0 {
		embed.Fields = append(embed.Fields, DiscordField{
			Name:   "Tags",
			Value:  strings.Join(alert.Device.Tags, ", "),
			Inline: false,
		})
	}
	
	return DiscordWebhookPayload{
		Embeds:   []DiscordEmbed{embed},
		Username: "Beacon Monitor",
	}
}

// getSeverityColor returns the Discord embed color for a severity level
func (p *DiscordPlugin) getSeverityColor(severity string) int {
	switch strings.ToLower(severity) {
	case "critical":
		return 0xFF0000 // Red
	case "warning":
		return 0xFFA500 // Orange
	case "info":
		return 0x00FF00 // Green
	default:
		return 0x808080 // Gray
	}
}

// HealthCheck verifies the Discord plugin is working
func (p *DiscordPlugin) HealthCheck() error {
	if p.webhookURL == "" {
		return fmt.Errorf("Discord plugin not initialized")
	}
	
	// Test webhook with a simple payload
	testPayload := DiscordWebhookPayload{
		Content: "Beacon Discord plugin health check",
	}
	
	jsonData, err := json.Marshal(testPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal test payload: %v", err)
	}
	
	req, err := http.NewRequest("POST", p.webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create test request: %v", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send test webhook: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("test webhook returned status %d", resp.StatusCode)
	}
	
	return nil
}

// Close cleans up the Discord plugin
func (p *DiscordPlugin) Close() error {
	// Nothing to clean up for Discord plugin
	return nil
}
