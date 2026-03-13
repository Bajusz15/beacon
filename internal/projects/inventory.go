package projects

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ProjectEntry holds metadata for a single Beacon project
type ProjectEntry struct {
	Name      string `json:"name"`
	Location  string `json:"location"`
	ConfigDir string `json:"config_dir"`
}

// Inventory holds the list of known projects
type Inventory struct {
	Projects []ProjectEntry `json:"projects"`
}

// LoadInventory loads the inventory from the given path
func LoadInventory(path string) (*Inventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Inventory{Projects: []ProjectEntry{}}, nil
		}
		return nil, fmt.Errorf("read inventory: %w", err)
	}
	var inv Inventory
	if err := json.Unmarshal(data, &inv); err != nil {
		return nil, fmt.Errorf("parse inventory: %w", err)
	}
	if inv.Projects == nil {
		inv.Projects = []ProjectEntry{}
	}
	return &inv, nil
}

// SaveInventory writes the inventory to the given path
func SaveInventory(path string, inv *Inventory) error {
	if inv == nil {
		inv = &Inventory{Projects: []ProjectEntry{}}
	}
	if inv.Projects == nil {
		inv.Projects = []ProjectEntry{}
	}
	data, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal inventory: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create inventory dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write inventory: %w", err)
	}
	return nil
}

// AddProject adds or updates a project in the inventory
func AddProject(inv *Inventory, name, location, configDir string) {
	for i := range inv.Projects {
		if inv.Projects[i].Name == name {
			inv.Projects[i].Location = location
			inv.Projects[i].ConfigDir = configDir
			return
		}
	}
	inv.Projects = append(inv.Projects, ProjectEntry{
		Name:      name,
		Location:  location,
		ConfigDir: configDir,
	})
}

// RemoveProject removes a project from the inventory
func RemoveProject(inv *Inventory, name string) {
	var kept []ProjectEntry
	for _, p := range inv.Projects {
		if p.Name != name {
			kept = append(kept, p)
		}
	}
	inv.Projects = kept
}

// GetProject returns the entry for a project by name, or nil if not found
func GetProject(inv *Inventory, name string) *ProjectEntry {
	for i := range inv.Projects {
		if inv.Projects[i].Name == name {
			return &inv.Projects[i]
		}
	}
	return nil
}
