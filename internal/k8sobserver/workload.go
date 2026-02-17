package k8sobserver

import "time"

// WorkloadSnapshot is the in-memory representation of a workload state (before converting to sources.Observation).
type WorkloadSnapshot struct {
	ID                 string
	ClusterID          string
	Namespace          string
	Kind               string
	Name               string
	UID                string
	DesiredReplicas    int32
	AvailableReplicas  int32
	ObservedGeneration int64
	SpecGeneration     int64
	DesiredImages      []string
	RunningImages      []string
	RunningDigests     []string
	Conditions         []string
	HealthSignals      []string
	FirstSeen          time.Time
	LastSeen           time.Time
	LastChange         time.Time
	InDrift            bool
	DriftReasons       []string
	PodRestartCount    int
	PodCountByPhase    map[string]int
}

// Drift reasons
const (
	DriftReasonImage      = "image_mismatch"
	DriftReasonGeneration = "generation_not_observed"
	DriftReasonReplicas   = "available_less_than_desired"
	DriftReasonRestarts   = "repeated_restarts"
)

// Health signal constants (from Pod status)
const (
	HealthCrashLoopBackOff = "CrashLoopBackOff"
	HealthImagePullBackOff = "ImagePullBackOff"
	HealthOOMKilled        = "OOMKilled"
	HealthError            = "Error"
)
