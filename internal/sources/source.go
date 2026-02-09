package sources

import "context"

// Source is an observation source that can be started and stopped.
// Implementations (e.g. Kubernetes observer) watch external systems and emit observations via a Sink.
type Source interface {
	// Name returns the unique name of this source (e.g. "my-cluster").
	Name() string
	// Type returns the source type (e.g. "kubernetes").
	Type() string
	// Start begins watching and sending observations to the configured Sink. It blocks until ctx is done or an error occurs.
	Start(ctx context.Context) error
	// Stop stops the source and releases resources.
	Stop() error
}

// Sink receives observations and events from a Source.
// Implementations persist state and optionally trigger alerts.
type Sink interface {
	// RecordObservation stores a workload observation (current state snapshot).
	RecordObservation(obs Observation) error
	// RecordEvent appends an audit/change event (e.g. drift_detected, workload_added).
	RecordEvent(ev Event) error
}

// Observation is a point-in-time snapshot of an observed workload (e.g. a Deployment).
type Observation struct {
	ID                string
	ClusterID         string
	Namespace         string
	Kind              string
	Name              string
	UID               string
	DesiredReplicas   int32
	AvailableReplicas int32
	ObservedGeneration int64
	SpecGeneration    int64
	DesiredImages     []string
	RunningImages     []string
	RunningDigests    []string
	Conditions        []string
	HealthSignals     []string
	FirstSeen         string
	LastSeen          string
	LastChange        string
	InDrift           bool
	DriftReasons      []string
}

// Event is a single audit/change event for the event log.
type Event struct {
	Timestamp string
	Type      string // workload_added, workload_updated, workload_deleted, drift_detected, health_alert
	SourceID  string
	Reason    string
	Details   map[string]interface{}
}
