// Package ipc provides file-based inter-process communication between master and child agents.
package ipc

import "time"

// HealthReport is written by the child agent to {ipc-dir}/health.json every 10 seconds.
// The master reads these to aggregate project health into heartbeat payloads.
type HealthReport struct {
	ProjectID     string            `json:"project_id"`
	Timestamp     time.Time         `json:"timestamp"`
	Status        string            `json:"status"` // "healthy", "degraded", "down", "unknown"
	UptimeSeconds int64             `json:"uptime_seconds"`
	Version       string            `json:"version,omitempty"`
	Metrics       map[string]any    `json:"metrics,omitempty"`
	LogsTail      []string          `json:"logs_tail,omitempty"`
	Checks        []CheckResult     `json:"checks"`
}

// CheckResult represents the result of a single health check.
type CheckResult struct {
	Name      string `json:"name"`
	Passed    bool   `json:"passed"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Command is written by the master to {ipc-dir}/command.json for the child to execute.
type Command struct {
	ID        string         `json:"id"`
	Action    string         `json:"action"` // "restart", "stop", "health_check", "fetch_logs"
	Payload   map[string]any `json:"payload,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// CommandResult is written by the child to {ipc-dir}/command_result.json after executing a command.
type CommandResult struct {
	CommandID string    `json:"command_id"`
	Status    string    `json:"status"` // "success", "failed"
	Message   string    `json:"message,omitempty"`
	Data      any       `json:"data,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Status constants for HealthReport.Status
const (
	StatusHealthy  = "healthy"
	StatusDegraded = "degraded"
	StatusDown     = "down"
	StatusUnknown  = "unknown"
)

// Action constants for Command.Action
const (
	ActionRestart     = "restart"
	ActionStop        = "stop"
	ActionHealthCheck = "health_check"
	ActionFetchLogs   = "fetch_logs"
)

// Result status constants for CommandResult.Status
const (
	ResultSuccess = "success"
	ResultFailed  = "failed"
)
