package util

import (
	"io"

	"beacon/internal/logging"
)

// Closer provides safe resource closing with error logging
type Closer struct {
	log *logging.Logger
}

// NewCloser creates a new Closer. An optional prefix becomes the logger component name.
func NewCloser(prefix string) *Closer {
	name := prefix
	if name == "" {
		name = "util"
	}
	return &Closer{log: logging.New(name)}
}

// DefaultCloser is a package-level closer with the default prefix
var DefaultCloser = NewCloser("")

// Close safely closes any io.Closer and logs errors
func (c *Closer) Close(closer io.Closer, resource string) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil {
		c.log.Errorf("Failed to close %s: %v", resource, err)
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
