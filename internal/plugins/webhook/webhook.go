package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"

	"beacon/internal/plugins"
	"beacon/internal/util"
)

// WebhookPlugin implements the Plugin interface for generic webhooks
type WebhookPlugin struct {
	name        string
	url         string
	method      string
	headers     map[string]string
	template    string
	httpClient  *http.Client
	contentType string
}

// NewWebhookPlugin creates a new webhook plugin instance
func NewWebhookPlugin() *WebhookPlugin {
	return &WebhookPlugin{
		name: "webhook",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		method:      "POST",
		contentType: "application/json",
		headers:     make(map[string]string),
	}
}

// Name returns the plugin name
func (p *WebhookPlugin) Name() string {
	return p.name
}

// Init initializes the webhook plugin with configuration
func (p *WebhookPlugin) Init(config map[string]interface{}) error {
	url, ok := config["url"].(string)
	if !ok || url == "" {
		return fmt.Errorf("url is required for webhook plugin")
	}

	// Expand environment variables
	p.url = os.ExpandEnv(url)

	// Optional method
	if method, ok := config["method"].(string); ok && method != "" {
		p.method = strings.ToUpper(method)
	}

	// Optional content type
	if contentType, ok := config["content_type"].(string); ok && contentType != "" {
		p.contentType = contentType
	}

	// Optional template
	if template, ok := config["template"].(string); ok && template != "" {
		p.template = template
	} else {
		// Use default template
		p.template = p.getDefaultTemplate()
	}

	// Optional headers
	if headers, ok := config["headers"].(map[string]interface{}); ok {
		p.headers = make(map[string]string)
		for key, value := range headers {
			if strValue, ok := value.(string); ok {
				p.headers[key] = os.ExpandEnv(strValue)
			}
		}
	}

	return nil
}

// SendAlert sends an alert via webhook
func (p *WebhookPlugin) SendAlert(alert plugins.Alert) error {
	if p.url == "" {
		return fmt.Errorf("webhook plugin not initialized")
	}

	// Generate payload based on template
	payload, err := p.generatePayload(alert)
	if err != nil {
		return fmt.Errorf("failed to generate payload: %v", err)
	}

	// Create request
	req, err := http.NewRequest(p.method, p.url, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %v", err)
	}

	// Set content type
	req.Header.Set("Content-Type", p.contentType)

	// Set custom headers
	for key, value := range p.headers {
		req.Header.Set(key, value)
	}

	// Send request
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %v", err)
	}
	defer util.DeferClose(resp.Body, "HTTP response body")()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// generatePayload generates the webhook payload using the template
func (p *WebhookPlugin) generatePayload(alert plugins.Alert) ([]byte, error) {
	// Parse template with custom functions
	tmpl, err := template.New("webhook").Funcs(templateFuncs).Parse(p.template)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %v", err)
	}

	// Create template data
	data := map[string]interface{}{
		"Title":     alert.Title,
		"Message":   alert.Message,
		"Severity":  alert.Severity,
		"Timestamp": alert.Timestamp,
		"Device":    alert.Device,
		"Check":     alert.Check,
		"Metadata":  alert.Metadata,
	}

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %v", err)
	}

	// Return as bytes
	return buf.Bytes(), nil
}

// getDefaultTemplate returns the default webhook template
func (p *WebhookPlugin) getDefaultTemplate() string {
	return `{
  "alert": {
    "title": "{{.Title}}",
    "message": "{{.Message}}",
    "severity": "{{.Severity}}",
    "timestamp": "{{.Timestamp.Format "2006-01-02T15:04:05Z07:00"}}",
    "device": {
      "name": "{{.Device.Name}}",
      "location": "{{.Device.Location}}",
      "environment": "{{.Device.Environment}}",
      "tags": [{{range $i, $tag := .Device.Tags}}{{if $i}},{{end}}"{{$tag}}"{{end}}]
    }{{if .Check}},
    "check": {
      "name": "{{.Check.Name}}",
      "type": "{{.Check.Type}}",
      "status": "{{.Check.Status}}",
      "duration": "{{.Check.Duration}}",
      "error": "{{.Check.Error}}",
      "http_status_code": {{.Check.HTTPStatusCode}},
      "response_time": "{{.Check.ResponseTime}}",
      "command_output": "{{.Check.CommandOutput}}",
      "command_error": "{{.Check.CommandError}}"
    }{{end}}{{if .Metadata}},
    "metadata": {{.Metadata | toJson}}{{end}}
  }
}`
}

// HealthCheck verifies the webhook plugin is working
func (p *WebhookPlugin) HealthCheck() error {
	if p.url == "" {
		return fmt.Errorf("webhook plugin not initialized")
	}

	// Test webhook with a simple payload
	testPayload := map[string]interface{}{
		"test":      true,
		"message":   "Beacon webhook plugin health check",
		"timestamp": time.Now().Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(testPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal test payload: %v", err)
	}

	req, err := http.NewRequest(p.method, p.url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create test request: %v", err)
	}

	req.Header.Set("Content-Type", p.contentType)

	// Set custom headers
	for key, value := range p.headers {
		req.Header.Set(key, value)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send test webhook: %v", err)
	}
	defer util.DeferClose(resp.Body, "HTTP response body")()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("test webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// Close cleans up the webhook plugin
func (p *WebhookPlugin) Close() error {
	// Nothing to clean up for webhook plugin
	return nil
}

// Template functions for Go templates
var templateFuncs = template.FuncMap{
	"toJson": func(v interface{}) string {
		b, err := json.Marshal(v)
		if err != nil {
			return "null"
		}
		return string(b)
	},
}
