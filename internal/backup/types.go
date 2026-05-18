package backup

import "time"

// Status represents the runtime state of the backup subsystem.
type Status struct {
	Enabled       bool      `json:"enabled"`
	Schedule      string    `json:"schedule,omitempty"`
	LastRunAt     time.Time `json:"last_run_at,omitempty"`
	LastSuccess   bool      `json:"last_success"`
	LastError     string    `json:"last_error,omitempty"`
	LastDurationS float64   `json:"last_duration_s,omitempty"`
	NextRunAt     time.Time `json:"next_run_at,omitempty"`
	SnapshotCount int       `json:"snapshot_count,omitempty"`
	Running       bool      `json:"running"`
}

// EventAppender allows the backup package to log events without importing master.
type EventAppender interface {
	AppendBackupEvent(message string, durationMs int64)
}
