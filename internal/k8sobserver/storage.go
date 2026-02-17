package k8sobserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"beacon/internal/sources"
)

const (
	workloadsFile = "workloads.json"
	eventsFile    = "events.jsonl"
	maxEvents     = 10000
)

// NoopSink discards observations and events. Use for read-only status (no state written).
type NoopSink struct{}

func (NoopSink) RecordObservation(obs sources.Observation) error { return nil }
func (NoopSink) RecordEvent(ev sources.Event) error              { return nil }

// FileSink persists observations and events to the state directory.
type FileSink struct {
	stateDir string
	mu       sync.Mutex
	events   []sources.Event
}

// NewFileSink creates a sink that writes to stateDir/k8s/.
func NewFileSink(stateDir string) (*FileSink, error) {
	dir := filepath.Join(stateDir, "k8s")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	s := &FileSink{stateDir: dir, events: make([]sources.Event, 0, 256)}
	return s, nil
}

// RecordObservation writes the current workload snapshot to workloads.json (merged with existing).
func (s *FileSink) RecordObservation(obs sources.Observation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.stateDir, workloadsFile)
	var workloads map[string]sources.Observation
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		if err := json.Unmarshal(data, &workloads); err != nil {
			workloads = make(map[string]sources.Observation)
		}
	} else {
		workloads = make(map[string]sources.Observation)
	}
	workloads[obs.ID] = obs
	newData, err := json.MarshalIndent(workloads, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, newData, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// RecordEvent appends an event to the event log and enforces retention.
func (s *FileSink) RecordEvent(ev sources.Event) error {
	s.mu.Lock()
	s.events = append(s.events, ev)
	if len(s.events) > maxEvents {
		s.events = s.events[len(s.events)-maxEvents:]
	}
	evCopy := ev
	s.mu.Unlock()

	f, err := os.OpenFile(filepath.Join(s.stateDir, eventsFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	return enc.Encode(evCopy)
}

// RecordObservationsBatch writes multiple workload snapshots at once (e.g. after full sync).
func (s *FileSink) RecordObservationsBatch(observations []sources.Observation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	workloads := make(map[string]sources.Observation)
	for _, obs := range observations {
		workloads[obs.ID] = obs
	}
	path := filepath.Join(s.stateDir, workloadsFile)
	newData, err := json.MarshalIndent(workloads, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, newData, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// observationFromSnapshot converts internal snapshot to sources.Observation.
func observationFromSnapshot(w WorkloadSnapshot) sources.Observation {
	formatTime := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.UTC().Format(time.RFC3339)
	}
	return sources.Observation{
		ID:                 w.ID,
		ClusterID:          w.ClusterID,
		Namespace:          w.Namespace,
		Kind:               w.Kind,
		Name:               w.Name,
		UID:                w.UID,
		DesiredReplicas:    w.DesiredReplicas,
		AvailableReplicas:  w.AvailableReplicas,
		ObservedGeneration: w.ObservedGeneration,
		SpecGeneration:     w.SpecGeneration,
		DesiredImages:      w.DesiredImages,
		RunningImages:      w.RunningImages,
		RunningDigests:     w.RunningDigests,
		Conditions:         w.Conditions,
		HealthSignals:      w.HealthSignals,
		FirstSeen:          formatTime(w.FirstSeen),
		LastSeen:           formatTime(w.LastSeen),
		LastChange:         formatTime(w.LastChange),
		InDrift:            w.InDrift,
		DriftReasons:       w.DriftReasons,
	}
}

func (w WorkloadSnapshot) toObservation() sources.Observation {
	return observationFromSnapshot(w)
}

// EventFromSnapshot creates a sources.Event for the event log.
func EventFromSnapshot(evType, reason string, w WorkloadSnapshot, details map[string]interface{}) sources.Event {
	if details == nil {
		details = make(map[string]interface{})
	}
	details["workload_id"] = w.ID
	details["namespace"] = w.Namespace
	details["kind"] = w.Kind
	details["name"] = w.Name
	return sources.Event{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Type:      evType,
		SourceID:  w.ClusterID,
		Reason:    reason,
		Details:   details,
	}
}

// LoadWorkloads reads the current workloads from the sink directory (for status CLI).
func LoadWorkloads(stateDir string) ([]sources.Observation, error) {
	path := filepath.Join(stateDir, "k8s", workloadsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var workloads map[string]sources.Observation
	if err := json.Unmarshal(data, &workloads); err != nil {
		return nil, fmt.Errorf("parse workloads: %w", err)
	}
	out := make([]sources.Observation, 0, len(workloads))
	for _, w := range workloads {
		out = append(out, w)
	}
	return out, nil
}
