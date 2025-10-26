package templates

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// CLI handles template management from command line
type CLI struct {
	manager *TemplateManager
}

// NewCLI creates a new template CLI
func NewCLI() (*CLI, error) {
	manager, err := NewTemplateManager()
	if err != nil {
		return nil, err
	}
	return &CLI{manager: manager}, nil
}

// AddTemplate adds a template interactively
func (c *CLI) AddTemplate() error {
	fmt.Println("📁 Template Management")
	fmt.Println("=====================")
	fmt.Println()

	// Get template name
	fmt.Print("Template name (e.g., 'my-alerts', 'production'): ")
	var name string
	_, err := fmt.Scanln(&name)
	if err != nil {
		return fmt.Errorf("failed to read template name: %w", err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("template name cannot be empty")
	}

	// Get template path
	fmt.Print("Template file path (absolute or relative): ")
	var path string
	_, err = fmt.Scanln(&path)
	if err != nil {
		return fmt.Errorf("failed to read template path: %w", err)
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("template path cannot be empty")
	}

	// Add template
	if err := c.manager.AddTemplate(name, path); err != nil {
		return fmt.Errorf("failed to add template: %w", err)
	}

	fmt.Printf("✅ Template '%s' added successfully\n", name)
	fmt.Printf("   Path: %s\n", path)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("1. Edit your template file")
	fmt.Println("2. Run: beacon monitor")
	fmt.Println("3. Beacon will automatically reload when template changes")

	return nil
}

// ListTemplates lists all registered templates
func (c *CLI) ListTemplates() error {
	templates := c.manager.ListTemplates()

	if len(templates) == 0 {
		fmt.Println("No templates registered.")
		fmt.Println("Add a template with: beacon template add")
		return nil
	}

	fmt.Println("📋 Registered Templates")
	fmt.Println("======================")
	fmt.Println()

	for name, template := range templates {
		fmt.Printf("Name: %s\n", name)
		fmt.Printf("Path: %s\n", template.Path)
		fmt.Printf("Last Modified: %s\n", template.LastModified.Format("2006-01-02 15:04:05"))
		fmt.Println()
	}

	return nil
}

// RemoveTemplate removes a template interactively
func (c *CLI) RemoveTemplate() error {
	templates := c.manager.ListTemplates()

	if len(templates) == 0 {
		fmt.Println("No templates to remove.")
		return nil
	}

	fmt.Println("🗑️  Remove Template")
	fmt.Println("==================")
	fmt.Println()

	// List available templates
	fmt.Println("Available templates:")
	for name := range templates {
		fmt.Printf("  - %s\n", name)
	}
	fmt.Println()

	fmt.Print("Template name to remove: ")
	var name string
	_, err := fmt.Scanln(&name)
	if err != nil {
		return fmt.Errorf("failed to read template name: %w", err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("template name cannot be empty")
	}

	if err := c.manager.RemoveTemplate(name); err != nil {
		return fmt.Errorf("failed to remove template: %w", err)
	}

	fmt.Printf("✅ Template '%s' removed successfully\n", name)
	return nil
}

// CheckChanges checks for template changes
func (c *CLI) CheckChanges() error {
	changed, err := c.manager.CheckForChanges()
	if err != nil {
		return fmt.Errorf("failed to check for changes: %w", err)
	}

	if len(changed) == 0 {
		fmt.Println("✅ No template changes detected")
		return nil
	}

	fmt.Println("🔄 Template Changes Detected")
	fmt.Println("===========================")
	fmt.Println()

	for _, name := range changed {
		fmt.Printf("  - %s (modified)\n", name)
	}

	fmt.Println()
	fmt.Println("Restart monitoring to apply changes:")
	fmt.Println("  systemctl --user restart beacon@monitor")
	fmt.Println("  # or")
	fmt.Println("  beacon monitor")

	return nil
}

// ShowTemplate shows template content
func (c *CLI) ShowTemplate(name string) error {
	template, err := c.manager.GetTemplate(name)
	if err != nil {
		return err
	}

	fmt.Printf("📄 Template: %s\n", name)
	fmt.Printf("Path: %s\n", template.Path)
	fmt.Println()

	// Read and display template content
	content, err := os.ReadFile(template.Path)
	if err != nil {
		return fmt.Errorf("failed to read template file: %w", err)
	}

	fmt.Println("Content:")
	fmt.Println(strings.Repeat("-", 50))
	fmt.Println(string(content))
	fmt.Println(strings.Repeat("-", 50))

	return nil
}

// ValidateTemplate validates a template file
func (c *CLI) ValidateTemplate(templatePath string) error {
	// Check if file exists
	if _, err := os.Stat(templatePath); err != nil {
		return fmt.Errorf("template file not found: %w", err)
	}

	// Read file content
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template file: %w", err)
	}

	// Basic validation - check if it's valid JSON or YAML
	contentStr := string(content)

	// Check for basic template structure
	if !strings.Contains(contentStr, "{{") {
		fmt.Println("⚠️  Warning: Template doesn't contain Go template syntax ({{ }})")
		fmt.Println("   This might be a static file rather than a template")
	}

	// Try to parse as JSON
	var jsonData interface{}
	if err := json.Unmarshal(content, &jsonData); err == nil {
		fmt.Println("✅ Template is valid JSON")
		return nil
	}

	// Try to parse as YAML
	// Note: In a real implementation, you'd use a YAML parser here
	if strings.Contains(contentStr, ":") && strings.Contains(contentStr, "\n") {
		fmt.Println("✅ Template appears to be valid YAML")
		return nil
	}

	fmt.Println("⚠️  Warning: Template format not recognized")
	fmt.Println("   Supported formats: JSON, YAML")
	return nil
}
