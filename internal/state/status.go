package state

import (
	"sync"
	"time"
)

type Status struct {
	mu           sync.RWMutex
	LastTag      string
	LastDeployed time.Time
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
}
