package util

import "beacon/internal/logging"

var logger = logging.New("util")

// LogError logs an error if it's not nil, with a descriptive message
func LogError(err error, operation string) {
	if err != nil {
		logger.Errorf("Failed to %s: %v", operation, err)
	}
}
