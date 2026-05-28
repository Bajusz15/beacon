package deploy

import (
	"errors"
	"fmt"
	"strings"
)

// AuthError is returned when a git or docker operation fails due to authentication.
type AuthError struct {
	Type    string // "git" or "docker"
	Code    int    // HTTP status code (401, 403) or 0 if unknown
	Message string
}

func (e *AuthError) Error() string {
	if e.Code > 0 {
		return fmt.Sprintf("%s auth error (%d): %s", e.Type, e.Code, e.Message)
	}
	return fmt.Sprintf("%s auth error: %s", e.Type, e.Message)
}

func IsAuthError(err error) (*AuthError, bool) {
	var ae *AuthError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}

func isGitAuthFailure(stderr string) bool {
	lower := strings.ToLower(stderr)
	patterns := []string{
		"authentication failed",
		"could not read username",
		"could not read password",
		"invalid credentials",
		"401",
		"403",
		"terminal prompts disabled",
		"could not resolve host",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func isDockerAuthFailure(output string) bool {
	lower := strings.ToLower(output)
	patterns := []string{
		"unauthorized",
		"denied",
		"authentication required",
		"401",
		"403",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}
