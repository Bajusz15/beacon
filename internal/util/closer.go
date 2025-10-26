package util

import (
	"io"
	"log"
)

// Closer provides safe resource closing with error logging
type Closer struct {
	prefix string
}

// NewCloser creates a new Closer with an optional log prefix
func NewCloser(prefix string) *Closer {
	if prefix == "" {
		prefix = "[Beacon]"
	}
	return &Closer{prefix: prefix}
}

// DefaultCloser is a package-level closer with the default prefix
var DefaultCloser = NewCloser("[Beacon]")

// Close safely closes any io.Closer and logs errors
func (c *Closer) Close(closer io.Closer, resource string) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil {
		log.Printf("%s Failed to close %s: %v", c.prefix, resource, err)
	}
}

// Package-level convenience functions using the default closer

// Close safely closes any io.Closer and logs errors
func Close(closer io.Closer, resource string) {
	DefaultCloser.Close(closer, resource)
}

// DeferClose returns a function suitable for defer that closes the resource
func DeferClose(closer io.Closer, resource string) func() {
	return func() {
		Close(closer, resource)
	}
}
