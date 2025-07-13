package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Status struct {
	mu           sync.RWMutex
	LastTag      string    `json:"last_tag"`
	LastDeployed time.Time `json:"last_deployed"`
	filepath     string
}

// NewStatus creates a new Status instance with persistence
func NewStatus(storageDir string) *Status {
	// Ensure data directory exists
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[Beacon] Failed to create storage directory %s: %v\n", storageDir, err)
		// Return empty status without persistence if directory creation fails
		return &Status{}
	}

	statusFile := filepath.Join(storageDir, "status.json")

	status := &Status{
		filepath: statusFile,
	}

	// Load existing status if available
	if err := status.Load(); err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "[Beacon] Failed to load status: %v\n", err)
		}
	}

	return status
}

func (s *Status) Get() (string, time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LastTag, s.LastDeployed
}

func (s *Status) Set(tag string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastTag = tag
	s.LastDeployed = t

	// Save to disk
	if err := s.save(); err != nil {
		fmt.Fprintf(os.Stderr, "[Beacon] Failed to save status: %v\n", err)
	}
}

// Load reads the status from the JSON file
func (s *Status) Load() error {
	data, err := os.ReadFile(s.filepath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, s)
}

// save writes the status to the JSON file
func (s *Status) save() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filepath, data, 0644)
}
