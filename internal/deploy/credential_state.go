package deploy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// CredentialErrorRecord is persisted to disk so the master can include it in heartbeats.
type CredentialErrorRecord struct {
	Type       string    `json:"type"`
	ErrorCode  int       `json:"error_code,omitempty"`
	Message    string    `json:"message"`
	DetectedAt time.Time `json:"detected_at"`
}

const credentialErrorsFile = "credential_errors.json"
const credentialErrorMaxAge = 24 * time.Hour

var credentialProjectIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func credentialErrorPath(stateDir, projectID string) (string, error) {
	if err := validateCredentialProjectID(projectID); err != nil {
		return "", err
	}
	return filepath.Join(stateDir, projectID, credentialErrorsFile), nil
}

func validateCredentialProjectID(projectID string) error {
	if projectID == "" {
		return fmt.Errorf("project id cannot be empty")
	}
	if projectID == "." || projectID == ".." || filepath.IsAbs(projectID) || filepath.Base(projectID) != projectID {
		return fmt.Errorf("invalid project id %q", projectID)
	}
	if !credentialProjectIDPattern.MatchString(projectID) {
		return fmt.Errorf("invalid project id %q", projectID)
	}
	return nil
}

// RecordCredentialError persists a credential error for the given project.
func RecordCredentialError(stateDir, projectID string, authErr *AuthError) error {
	p, err := credentialErrorPath(stateDir, projectID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}

	existing := readRawErrors(p)

	rec := CredentialErrorRecord{
		Type:       authErr.Type,
		ErrorCode:  authErr.Code,
		Message:    authErr.Message,
		DetectedAt: time.Now(),
	}

	// Replace existing error of the same type, or append
	replaced := false
	for i, e := range existing {
		if e.Type == rec.Type {
			existing[i] = rec
			replaced = true
			break
		}
	}
	if !replaced {
		existing = append(existing, rec)
	}

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}

	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// ReadCredentialErrors returns unexpired credential errors for a project.
func ReadCredentialErrors(stateDir, projectID string) []CredentialErrorRecord {
	p, err := credentialErrorPath(stateDir, projectID)
	if err != nil {
		return nil
	}
	all := readRawErrors(p)
	cutoff := time.Now().Add(-credentialErrorMaxAge)
	fresh := make([]CredentialErrorRecord, 0, len(all))
	for _, e := range all {
		if e.DetectedAt.After(cutoff) {
			fresh = append(fresh, e)
		}
	}
	return fresh
}

// ClearCredentialErrors removes the credential error file for a project.
func ClearCredentialErrors(stateDir, projectID string) {
	p, err := credentialErrorPath(stateDir, projectID)
	if err != nil {
		logger.Infof("Refusing to clear credential errors for invalid project id %q: %v", projectID, err)
		return
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		logger.Infof("Failed to clear credential errors for %s: %v", projectID, err)
	}
}

func readRawErrors(path string) []CredentialErrorRecord {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var records []CredentialErrorRecord
	if json.Unmarshal(data, &records) != nil {
		return nil
	}
	return records
}
