package master

import (
	"sync"
	"time"
)

const (
	maxEvents   = 50
	maxEventAge = 24 * time.Hour
)

// EventType classifies a log entry.
type EventType string

const (
	EventDeploy  EventType = "deploy"
	EventAlert   EventType = "alert"
	EventSync    EventType = "sync"
	EventRestart EventType = "restart"
	EventStart   EventType = "start"
	EventStop    EventType = "stop"
)

// Event is a single entry in the event ring buffer.
type Event struct {
	Timestamp  time.Time `json:"timestamp"`
	Type       EventType `json:"type"`
	Child      string    `json:"child,omitempty"`
	Message    string    `json:"message"`
	DurationMs int64     `json:"duration_ms,omitempty"`
}

// EventLog is a bounded, thread-safe ring buffer.
type EventLog struct {
	mu     sync.RWMutex
	events []Event
}

// NewEventLog creates an empty event log.
func NewEventLog() *EventLog {
	return &EventLog{
		events: make([]Event, 0, maxEvents),
	}
}

// Append adds an event. Evicts the oldest entry if at capacity.
// Also prunes events older than maxEventAge.
func (el *EventLog) Append(e Event) {
	el.mu.Lock()
	defer el.mu.Unlock()

	el.events = append(el.events, e)

	// Prune old events
	cutoff := time.Now().Add(-maxEventAge)
	start := 0
	for start < len(el.events) && el.events[start].Timestamp.Before(cutoff) {
		start++
	}
	if start > 0 {
		el.events = el.events[start:]
	}

	// Cap at maxEvents
	if len(el.events) > maxEvents {
		el.events = el.events[len(el.events)-maxEvents:]
	}
}

// Recent returns a copy of events within the last 24h, newest-last.
func (el *EventLog) Recent() []Event {
	el.mu.RLock()
	defer el.mu.RUnlock()

	cutoff := time.Now().Add(-maxEventAge)
	result := make([]Event, 0, len(el.events))
	for _, e := range el.events {
		if !e.Timestamp.Before(cutoff) {
			result = append(result, e)
		}
	}
	return result
}
