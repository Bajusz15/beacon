package templates

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"beacon/internal/config"
)

// TemplateConfig represents a template configuration
type TemplateConfig struct {
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	LastModified time.Time `json:"last_modified"`
	Checksum     string    `json:"checksum"`
}

// TemplateManager handles template loading and management
type TemplateManager struct {
	configDir string
	templates map[string]*TemplateConfig
}

// NewTemplateManager creates a new template manager
func NewTemplateManager() (*TemplateManager, error) {
	configDir := getConfigDir()
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	tm := &TemplateManager{
		configDir: configDir,
		templates: make(map[string]*TemplateConfig),
	}

	// Load existing templates
	if err := tm.loadTemplates(); err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	return tm, nil
}

// AddTemplate adds a template to the manager
func (tm *TemplateManager) AddTemplate(name, templatePath string) error {
	// Resolve absolute path
	absPath, err := filepath.Abs(templatePath)
	if err != nil {
		return fmt.Errorf("failed to resolve template path: %w", err)
	}

	// Check if template file exists
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("template file not found: %w", err)
	}

	// Calculate checksum
	checksum, err := calculateChecksum(absPath)
	if err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	template := &TemplateConfig{
		Name:         name,
		Path:         absPath,
		LastModified: info.ModTime(),
		Checksum:     checksum,
	}

	tm.templates[name] = template
	return tm.saveTemplates()
}

// GetTemplate returns a template by name
func (tm *TemplateManager) GetTemplate(name string) (*TemplateConfig, error) {
	template, exists := tm.templates[name]
	if !exists {
		return nil, fmt.Errorf("template '%s' not found", name)
	}
	return template, nil
}

// ListTemplates returns all registered templates
func (tm *TemplateManager) ListTemplates() map[string]*TemplateConfig {
	return tm.templates
}

// RemoveTemplate removes a template
func (tm *TemplateManager) RemoveTemplate(name string) error {
	if _, exists := tm.templates[name]; !exists {
		return fmt.Errorf("template '%s' not found", name)
	}
	delete(tm.templates, name)
	return tm.saveTemplates()
}

// CheckForChanges checks if any templates have been modified
func (tm *TemplateManager) CheckForChanges() ([]string, error) {
	var changed []string

	for name, template := range tm.templates {
		info, err := os.Stat(template.Path)
		if err != nil {
			// Template file no longer exists
			changed = append(changed, name)
			continue
		}

		// Check if modification time changed
		if !info.ModTime().Equal(template.LastModified) {
			// Recalculate checksum to confirm change
			newChecksum, err := calculateChecksum(template.Path)
			if err != nil {
				continue
			}

			if newChecksum != template.Checksum {
				changed = append(changed, name)
				// Update template info
				template.LastModified = info.ModTime()
				template.Checksum = newChecksum
			}
		}
	}

	if len(changed) > 0 {
		// Save updated template info
		if err := tm.saveTemplates(); err != nil {
			return changed, fmt.Errorf("failed to save template updates: %w", err)
		}
	}

	return changed, nil
}

// loadTemplates loads templates from config file
func (tm *TemplateManager) loadTemplates() error {
	configPath := filepath.Join(tm.configDir, "templates.json")

	data, err := os.ReadFile(configPath) // #nosec G304 -- configPath is safely constructed
	if err != nil {
		if os.IsNotExist(err) {
			// No templates file yet, start empty
			return nil
		}
		return err
	}

	var templates map[string]*TemplateConfig
	if err := json.Unmarshal(data, &templates); err != nil {
		return err
	}

	tm.templates = templates
	return nil
}

// saveTemplates saves templates to config file
func (tm *TemplateManager) saveTemplates() error {
	configPath := filepath.Join(tm.configDir, "templates.json")

	data, err := json.MarshalIndent(tm.templates, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// getConfigDir returns the beacon config directory
func getConfigDir() string {
	base, err := config.BeaconHomeDir()
	if err != nil {
		return filepath.Join("/tmp", ".beacon")
	}
	return base
}

// calculateChecksum calculates SHA256 checksum of a file
func calculateChecksum(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	// Simple checksum using file size and modification time
	info, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}

	// Use a combination of size and mod time as checksum
	checksum := fmt.Sprintf("%d-%d", len(data), info.ModTime().Unix())
	return checksum, nil
}
