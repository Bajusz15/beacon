package master

import (
	"os"
	"sync"
	"time"

	"beacon/internal/identity"
	"beacon/internal/ipc"
	"beacon/internal/version"
)

const defaultMetricsPort = 9100

// MasterInfo describes the master process itself.
type MasterInfo struct {
	PID           int       `json:"pid"`
	UptimeSeconds int64     `json:"uptime_seconds"`
	StartedAt     time.Time `json:"started_at"`
}

// CheckDetail is one check's detail for /api/status.
type CheckDetail struct {
	Name       string `json:"name"`
	Type       string `json:"type,omitempty"`
	Status     string `json:"status"` // "passing" | "failing"
	DurationMs int64  `json:"duration_ms,omitempty"`
	Error      string `json:"error,omitempty"`
}

// CheckSummary aggregates a child's check results.
type CheckSummary struct {
	Total   int           `json:"total"`
	Passing int           `json:"passing"`
	Failing int           `json:"failing"`
	Details []CheckDetail `json:"details"`
}

// ChildStatus is one project child's view for /api/status.
type ChildStatus struct {
	Name    string     `json:"name"`
	Version string     `json:"version,omitempty"`
	Status  string     `json:"status"`
	PID     int        `json:"pid,omitempty"`
	DeployedAt *time.Time   `json:"deployed_at,omitempty"`
	Checks     CheckSummary `json:"checks"`
}

// CloudStatus describes cloud connectivity.
type CloudStatus struct {
	Connected bool      `json:"connected"`
	LastSync  time.Time `json:"last_sync,omitempty"`
	Endpoint  string    `json:"endpoint,omitempty"`
}

// StatusSnapshot is the full /api/status response body.
// Shared between the HTTP server and CLI deserialization.
type StatusSnapshot struct {
	Version  string        `json:"version"`
	Master   MasterInfo    `json:"master"`
	Device   DeviceInfo    `json:"device"`
	System   DeviceMetrics `json:"system"`
	Children []ChildStatus `json:"children"`
	Events   []Event       `json:"events"`
	Cloud    CloudStatus   `json:"cloud"`
}

// StatusCache holds the latest snapshot, refreshed every 10 seconds.
type StatusCache struct {
	mu        sync.RWMutex
	snapshot  StatusSnapshot
	pm        *ProcessManager
	eventLog  *EventLog
	startedAt time.Time
	cfg       *identity.UserConfig
}

// NewStatusCache constructs a cache. Call Refresh() once before serving.
func NewStatusCache(pm *ProcessManager, el *EventLog, cfg *identity.UserConfig) *StatusCache {
	return &StatusCache{
		pm:        pm,
		eventLog:  el,
		startedAt: time.Now(),
		cfg:       cfg,
	}
}

// Refresh rebuilds the snapshot from current IPC health files and /proc metrics.
// Thread-safe: acquires write lock.
func (sc *StatusCache) Refresh() {
	snap := StatusSnapshot{
		Version: version.GetVersion(),
		Master: MasterInfo{
			PID:           os.Getpid(),
			UptimeSeconds: int64(time.Since(sc.startedAt).Seconds()),
			StartedAt:     sc.startedAt,
		},
		Device: CollectDeviceInfo(),
		System: CollectDeviceMetrics(),
	}

	// Build children from IPC health + PID map
	if sc.pm != nil {
		snap.Children = sc.buildChildren()
	}

	// Events from ring buffer
	if sc.eventLog != nil {
		snap.Events = sc.eventLog.Recent()
	}

	// Preserve cloud sync state and apply config
	sc.mu.Lock()
	snap.Cloud = sc.snapshot.Cloud
	if sc.cfg != nil && sc.cfg.CloudURL != "" {
		snap.Cloud.Endpoint = sc.cfg.CloudURL
	}
	sc.snapshot = snap
	sc.mu.Unlock()
}

// Get returns a copy of the current snapshot.
func (sc *StatusCache) Get() StatusSnapshot {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.snapshot
}

// UpdateCloudSync marks a successful cloud heartbeat.
func (sc *StatusCache) UpdateCloudSync() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.snapshot.Cloud.Connected = true
	sc.snapshot.Cloud.LastSync = time.Now()
}

// UpdateConfig updates the config reference on hot-reload.
func (sc *StatusCache) UpdateConfig(cfg *identity.UserConfig) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.cfg = cfg
	if cfg != nil && cfg.CloudURL != "" {
		sc.snapshot.Cloud.Endpoint = cfg.CloudURL
	}
}

// buildChildren reads IPC health for all children and maps to ChildStatus.
func (sc *StatusCache) buildChildren() []ChildStatus {
	readers := sc.pm.GetIPCReaders()
	if len(readers) == 0 {
		return nil
	}

	pids := sc.pm.GetChildPIDs()

	children := make([]ChildStatus, 0, len(readers))
	for projectID, reader := range readers {
		child := sc.childFromReader(projectID, reader)
		if pid, ok := pids[projectID]; ok {
			child.PID = pid
		}
		children = append(children, child)
	}
	return children
}

// childFromReader builds a ChildStatus from an IPC reader.
func (sc *StatusCache) childFromReader(projectID string, reader *ipc.Reader) ChildStatus {
	report, err := reader.ReadHealthIfFresh(healthMaxAge)
	if err != nil || report == nil {
		return ChildStatus{
			Name:   projectID,
			Status: ipc.StatusUnknown,
			Checks: CheckSummary{},
		}
	}

	details := make([]CheckDetail, 0, len(report.Checks))
	passing := 0
	failing := 0
	for _, c := range report.Checks {
		status := "passing"
		if !c.Passed {
			status = "failing"
			failing++
		} else {
			passing++
		}
		details = append(details, CheckDetail{
			Name:       c.Name,
			Status:     status,
			DurationMs: c.LatencyMs,
			Error:      c.Error,
		})
	}

	return ChildStatus{
		Name:    report.ProjectID,
		Version: report.Version,
		Status:  report.Status,
		DeployedAt: report.DeployedAt,
		Checks: CheckSummary{
			Total:   len(report.Checks),
			Passing: passing,
			Failing: failing,
			Details: details,
		},
	}
}
