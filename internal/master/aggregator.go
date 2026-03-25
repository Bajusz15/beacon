package master

import (
	"time"

	"beacon/internal/ipc"
)

const (
	// healthMaxAge is the maximum age of a health report before it's considered stale
	healthMaxAge = 60 * time.Second
)

// ProjectHealth represents aggregated health data for a project in the heartbeat.
type ProjectHealth struct {
	ProjectID     string         `json:"project_id"`
	Status        string         `json:"status"`
	UptimeSeconds int64          `json:"uptime_seconds,omitempty"`
	Version       string         `json:"version,omitempty"`
	Metrics       map[string]any `json:"metrics,omitempty"`
	LogsTail      []string       `json:"logs_tail,omitempty"`
	Checks        []CheckHealth  `json:"checks,omitempty"`
}

// CheckHealth represents a single health check result in the heartbeat.
type CheckHealth struct {
	Name      string `json:"name"`
	Passed    bool   `json:"passed"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
}

// AggregateProjectHealth reads health reports from all children via IPC and returns
// a slice of ProjectHealth suitable for inclusion in the heartbeat payload.
func AggregateProjectHealth(pm *ProcessManager) []ProjectHealth {
	if pm == nil {
		return nil
	}

	readers := pm.GetIPCReaders()
	if len(readers) == 0 {
		return nil
	}

	projects := make([]ProjectHealth, 0, len(readers))

	for projectID, reader := range readers {
		health := aggregateFromReader(projectID, reader)
		projects = append(projects, health)
	}

	return projects
}

// aggregateFromReader reads a single project's health from IPC.
func aggregateFromReader(projectID string, reader *ipc.Reader) ProjectHealth {
	report, err := reader.ReadHealthIfFresh(healthMaxAge)
	if err != nil || report == nil {
		// Health file missing or stale - report as unknown
		return ProjectHealth{
			ProjectID: projectID,
			Status:    ipc.StatusUnknown,
		}
	}

	// Convert IPC checks to heartbeat format
	checks := make([]CheckHealth, 0, len(report.Checks))
	for _, c := range report.Checks {
		checks = append(checks, CheckHealth{
			Name:      c.Name,
			Passed:    c.Passed,
			LatencyMs: c.LatencyMs,
		})
	}

	return ProjectHealth{
		ProjectID:     report.ProjectID,
		Status:        report.Status,
		UptimeSeconds: report.UptimeSeconds,
		Version:       report.Version,
		Metrics:       report.Metrics,
		LogsTail:      report.LogsTail,
		Checks:        checks,
	}
}
