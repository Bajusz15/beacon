package email

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"os"
	"strings"

	"beacon/internal/plugins"
	"beacon/internal/util"
)

// EmailPlugin implements the Plugin interface for SMTP email
type EmailPlugin struct {
	name     string
	smtpHost string
	smtpPort string
	smtpUser string
	smtpPass string
	from     string
	to       []string
	useTLS   bool
	auth     smtp.Auth
}

// NewEmailPlugin creates a new email plugin instance
func NewEmailPlugin() *EmailPlugin {
	return &EmailPlugin{
		name: "email",
	}
}

// Name returns the plugin name
func (p *EmailPlugin) Name() string {
	return p.name
}

// Init initializes the email plugin with configuration
func (p *EmailPlugin) Init(config map[string]interface{}) error {
	// Required fields
	smtpHost, ok := config["smtp_host"].(string)
	if !ok || smtpHost == "" {
		return fmt.Errorf("smtp_host is required for email plugin")
	}

	smtpPort, ok := config["smtp_port"].(string)
	if !ok || smtpPort == "" {
		return fmt.Errorf("smtp_port is required for email plugin")
	}

	smtpUser, ok := config["smtp_user"].(string)
	if !ok || smtpUser == "" {
		return fmt.Errorf("smtp_user is required for email plugin")
	}

	smtpPass, ok := config["smtp_pass"].(string)
	if !ok || smtpPass == "" {
		return fmt.Errorf("smtp_pass is required for email plugin")
	}

	from, ok := config["from"].(string)
	if !ok || from == "" {
		return fmt.Errorf("from is required for email plugin")
	}

	toInterface, ok := config["to"]
	if !ok {
		return fmt.Errorf("to is required for email plugin")
	}

	// Handle different types for 'to' field
	var to []string
	switch v := toInterface.(type) {
	case string:
		to = []string{v}
	case []interface{}:
		for _, item := range v {
			if str, ok := item.(string); ok {
				to = append(to, str)
			}
		}
	case []string:
		to = v
	default:
		return fmt.Errorf("invalid 'to' field format")
	}

	if len(to) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}

	// Expand environment variables
	p.smtpHost = os.ExpandEnv(smtpHost)
	p.smtpPort = os.ExpandEnv(smtpPort)
	p.smtpUser = os.ExpandEnv(smtpUser)
	p.smtpPass = os.ExpandEnv(smtpPass)
	p.from = os.ExpandEnv(from)
	p.to = make([]string, len(to))
	for i, addr := range to {
		p.to[i] = os.ExpandEnv(addr)
	}

	// Optional TLS setting
	if useTLS, ok := config["use_tls"].(bool); ok {
		p.useTLS = useTLS
	} else {
		// Default to TLS for common ports
		p.useTLS = p.smtpPort == "587" || p.smtpPort == "465"
	}

	// Create SMTP auth
	p.auth = smtp.PlainAuth("", p.smtpUser, p.smtpPass, p.smtpHost)

	return nil
}

// SendAlert sends an alert via email
func (p *EmailPlugin) SendAlert(alert plugins.Alert) error {
	if p.smtpHost == "" || p.smtpUser == "" || len(p.to) == 0 {
		return fmt.Errorf("email plugin not initialized")
	}

	subject, body := p.buildEmail(alert)

	// Build email message
	message := p.buildMessage(subject, body)

	// Send email
	addr := fmt.Sprintf("%s:%s", p.smtpHost, p.smtpPort)

	if p.useTLS {
		return p.sendTLS(addr, []byte(message))
	} else {
		return smtp.SendMail(addr, p.auth, p.from, p.to, []byte(message))
	}
}

// buildEmail builds the email subject and body from an alert
func (p *EmailPlugin) buildEmail(alert plugins.Alert) (string, string) {
	// Build subject
	subject := fmt.Sprintf("[%s] %s - %s", strings.ToUpper(alert.Severity), alert.Device.Name, alert.Title)

	// Build body
	var body strings.Builder

	body.WriteString("Beacon Alert\n")
	body.WriteString("============\n\n")
	body.WriteString(fmt.Sprintf("Title: %s\n", alert.Title))
	body.WriteString(fmt.Sprintf("Message: %s\n", alert.Message))
	body.WriteString(fmt.Sprintf("Severity: %s\n", strings.ToUpper(alert.Severity)))
	body.WriteString(fmt.Sprintf("Time: %s\n\n", alert.Timestamp.Format("2006-01-02 15:04:05 MST")))

	body.WriteString("Device Information:\n")
	body.WriteString("-------------------\n")
	body.WriteString(fmt.Sprintf("Name: %s\n", alert.Device.Name))
	if alert.Device.Location != "" {
		body.WriteString(fmt.Sprintf("Location: %s\n", alert.Device.Location))
	}
	if alert.Device.Environment != "" {
		body.WriteString(fmt.Sprintf("Environment: %s\n", alert.Device.Environment))
	}
	if len(alert.Device.Tags) > 0 {
		body.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(alert.Device.Tags, ", ")))
	}

	// Add check details if available
	if alert.Check != nil {
		body.WriteString("\nCheck Details:\n")
		body.WriteString("--------------\n")
		body.WriteString(fmt.Sprintf("Name: %s\n", alert.Check.Name))
		body.WriteString(fmt.Sprintf("Type: %s\n", alert.Check.Type))
		body.WriteString(fmt.Sprintf("Status: %s\n", strings.ToUpper(alert.Check.Status)))
		body.WriteString(fmt.Sprintf("Duration: %s\n", alert.Check.Duration.String()))

		if alert.Check.Error != "" {
			body.WriteString(fmt.Sprintf("Error: %s\n", alert.Check.Error))
		}

		if alert.Check.HTTPStatusCode > 0 {
			body.WriteString(fmt.Sprintf("HTTP Status Code: %d\n", alert.Check.HTTPStatusCode))
		}

		if alert.Check.ResponseTime > 0 {
			body.WriteString(fmt.Sprintf("Response Time: %s\n", alert.Check.ResponseTime.String()))
		}

		if alert.Check.CommandOutput != "" {
			body.WriteString(fmt.Sprintf("Command Output: %s\n", alert.Check.CommandOutput))
		}
	}

	body.WriteString("\n---\n")
	body.WriteString("This alert was sent by Beacon monitoring system.\n")

	return subject, body.String()
}

// buildMessage builds the complete email message
func (p *EmailPlugin) buildMessage(subject, body string) string {
	var message strings.Builder

	message.WriteString(fmt.Sprintf("From: %s\r\n", p.from))
	message.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(p.to, ", ")))
	message.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	message.WriteString("MIME-Version: 1.0\r\n")
	message.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	message.WriteString("\r\n")
	message.WriteString(body)

	return message.String()
}

// sendTLS sends email using TLS connection
func (p *EmailPlugin) sendTLS(addr string, message []byte) error {
	// Create TLS connection
	conn, err := tls.Dial("tcp", addr, &tls.Config{
		ServerName: p.smtpHost,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %v", err)
	}
	defer util.Close(conn, "SMTP connection")

	// Create SMTP client
	client, err := smtp.NewClient(conn, p.smtpHost)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %v", err)
	}
	defer client.Quit()

	// Authenticate
	if err := client.Auth(p.auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %v", err)
	}

	// Set sender
	if err := client.Mail(p.from); err != nil {
		return fmt.Errorf("failed to set sender: %v", err)
	}

	// Set recipients
	for _, to := range p.to {
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("failed to set recipient %s: %v", to, err)
		}
	}

	// Send message
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to get data writer: %v", err)
	}

	if _, err := writer.Write(message); err != nil {
		return fmt.Errorf("failed to write message: %v", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close data writer: %v", err)
	}

	return nil
}

// HealthCheck verifies the email plugin is working
func (p *EmailPlugin) HealthCheck() error {
	if p.smtpHost == "" || p.smtpUser == "" || len(p.to) == 0 {
		return fmt.Errorf("email plugin not initialized")
	}

	// Test SMTP connection
	addr := fmt.Sprintf("%s:%s", p.smtpHost, p.smtpPort)

	if p.useTLS {
		conn, err := tls.Dial("tcp", addr, &tls.Config{
			ServerName: p.smtpHost,
		})
		if err != nil {
			return fmt.Errorf("failed to connect to SMTP server: %v", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, p.smtpHost)
		if err != nil {
			return fmt.Errorf("failed to create SMTP client: %v", err)
		}
		defer client.Quit()

		if err := client.Auth(p.auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %v", err)
		}
	} else {
		// Test basic SMTP connection
		conn, err := smtp.Dial(addr)
		if err != nil {
			return fmt.Errorf("failed to connect to SMTP server: %v", err)
		}
		defer util.Close(conn, "smtp connection")

		if err := conn.Auth(p.auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %v", err)
		}
	}

	return nil
}

// Close cleans up the email plugin
func (p *EmailPlugin) Close() error {
	// Nothing to clean up for email plugin
	return nil
}
