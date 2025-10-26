package util

import "log"

// LogError logs an error if it's not nil, with a descriptive message
func LogError(err error, operation string) {
	if err != nil {
		log.Printf("[Beacon] Failed to %s: %v", operation, err)
	}
}
