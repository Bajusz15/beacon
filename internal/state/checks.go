package state

import "time"

// CheckState represents a single check's persisted state for CLI display
type CheckState struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"` // "up", "down", "error"
	Timestamp time.Time `json:"timestamp"`
	Error     string    `json:"error,omitempty"`
}

// ChecksState is written to ~/.beacon/state/<project>/checks.json
type ChecksState struct {
	UpdatedAt time.Time    `json:"updated_at"`
	Checks    []CheckState `json:"checks"`
}
