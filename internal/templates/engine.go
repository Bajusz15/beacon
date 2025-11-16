package templates

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"beacon/internal/plugins"
)

// TemplateEngine handles template processing
type TemplateEngine struct {
	templates map[string]*template.Template
}

// NewTemplateEngine creates a new template engine
func NewTemplateEngine() *TemplateEngine {
	return &TemplateEngine{
		templates: make(map[string]*template.Template),
	}
}

// LoadTemplate loads a template from file
func (te *TemplateEngine) LoadTemplate(name, filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read template file: %w", err)
	}

	tmpl, err := template.New(name).Parse(string(content))
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	te.templates[name] = tmpl
	return nil
}

// ProcessAlert processes an alert through a template
func (te *TemplateEngine) ProcessAlert(templateName string, alert plugins.Alert) (string, error) {
	tmpl, exists := te.templates[templateName]
	if !exists {
		return "", fmt.Errorf("template '%s' not found", templateName)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, alert); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// ProcessCheckResult processes a check result through a template
func (te *TemplateEngine) ProcessCheckResult(templateName string, result *plugins.CheckResult) (string, error) {
	tmpl, exists := te.templates[templateName]
	if !exists {
		return "", fmt.Errorf("template '%s' not found", templateName)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, result); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// CreateDefaultTemplates creates default template files
func CreateDefaultTemplates(templateDir string) error {
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		return fmt.Errorf("failed to create template directory: %w", err)
	}

	// Email template
	emailTemplate := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{.Title}}</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .header { background-color: {{if eq .Severity "critical"}}#dc3545{{else if eq .Severity "warning"}}#ffc107{{else}}#28a745{{end}}; color: white; padding: 15px; border-radius: 5px; }
        .content { margin: 20px 0; }
        .field { margin: 10px 0; }
        .label { font-weight: bold; }
        .error { background-color: #f8d7da; color: #721c24; padding: 10px; border-radius: 3px; margin: 10px 0; }
    </style>
</head>
<body>
    <div class="header">
        <h1>{{.Title}}</h1>
    </div>
    
    <div class="content">
        <p>{{.Message}}</p>
        
        <div class="field">
            <span class="label">Device:</span> {{.Device.Name}}
        </div>
        <div class="field">
            <span class="label">Severity:</span> {{.Severity}}
        </div>
        <div class="field">
            <span class="label">Time:</span> {{.Timestamp.Format "2006-01-02 15:04:05 MST"}}
        </div>
        
        {{if .Check}}
        <h3>Check Details</h3>
        <div class="field">
            <span class="label">Name:</span> {{.Check.Name}}
        </div>
        <div class="field">
            <span class="label">Type:</span> {{.Check.Type}}
        </div>
        <div class="field">
            <span class="label">Status:</span> {{.Check.Status}}
        </div>
        {{if .Check.Error}}
        <div class="error">
            <strong>Error:</strong> {{.Check.Error}}
        </div>
        {{end}}
        {{end}}
    </div>
</body>
</html>`

	// Webhook template
	webhookTemplate := `{
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
    },
    "check": {{if .Check}}{
      "name": "{{.Check.Name}}",
      "type": "{{.Check.Type}}",
      "status": "{{.Check.Status}}",
      "duration": "{{.Check.Duration}}",
      "timestamp": "{{.Check.Timestamp.Format "2006-01-02T15:04:05Z07:00"}}",
      "error": "{{.Check.Error}}",
      "http_status_code": {{.Check.HTTPStatusCode}},
      "response_time": "{{.Check.ResponseTime}}",
      "command_output": "{{.Check.CommandOutput}}",
      "command_error": "{{.Check.CommandError}}"
    }{{else}}null{{end}}
  }
}`

	// Write template files
	templates := map[string]string{
		"email.html":   emailTemplate,
		"webhook.json": webhookTemplate,
	}

	for filename, content := range templates {
		filePath := filepath.Join(templateDir, filename)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write template %s: %w", filename, err)
		}
	}

	return nil
}

// GetDefaultTemplatePath returns the default template directory
func GetDefaultTemplatePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "/tmp"
	}
	return filepath.Join(homeDir, ".beacon", "templates")
}
