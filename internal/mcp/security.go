package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"sync"
	"time"
)

const (
	gitRefPattern     = `^[a-zA-Z0-9/_.\-]+$`
	grepPatternMaxLen = 200
	confirmationTTL   = 60 * time.Second
)

var gitRefRe = regexp.MustCompile(gitRefPattern)

// ValidateProjectName ensures project is in inventory and safe
func ValidateProjectName(project string, knownProjects []string) error {
	if project == "" {
		return fmt.Errorf("project name cannot be empty")
	}
	for _, p := range knownProjects {
		if p == project {
			return nil
		}
	}
	return fmt.Errorf("project %q not in inventory", project)
}

// ValidateGitRef ensures ref is safe for git commands
func ValidateGitRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("ref cannot be empty")
	}
	if len(ref) > 256 {
		return fmt.Errorf("ref too long")
	}
	if !gitRefRe.MatchString(ref) {
		return fmt.Errorf("ref contains invalid characters: %q", ref)
	}
	return nil
}

// ValidateGrepPattern sanitizes grep pattern - allow alphanumeric, spaces, common regex
func ValidateGrepPattern(pattern string) (string, error) {
	if len(pattern) > grepPatternMaxLen {
		return "", fmt.Errorf("grep pattern too long (max %d)", grepPatternMaxLen)
	}
	for _, r := range pattern {
		if r < 32 || r > 126 {
			return "", fmt.Errorf("grep pattern contains invalid character")
		}
	}
	return pattern, nil
}

// ConfirmationTokenStore holds pending confirmations for write tools
type ConfirmationTokenStore struct {
	mu    sync.Mutex
	store map[string]confirmationEntry
}

type confirmationEntry struct {
	Tool      string
	Args      map[string]any
	ExpiresAt time.Time
}

// NewConfirmationTokenStore creates a new store
func NewConfirmationTokenStore() *ConfirmationTokenStore {
	return &ConfirmationTokenStore{
		store: make(map[string]confirmationEntry),
	}
}

// CreateToken creates a confirmation token for a pending action
func (s *ConfirmationTokenStore) CreateToken(tool string, args map[string]any) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	s.mu.Lock()
	s.store[token] = confirmationEntry{
		Tool:      tool,
		Args:      args,
		ExpiresAt: time.Now().Add(confirmationTTL),
	}
	s.mu.Unlock()
	return token, nil
}

// Confirm consumes the token and returns the stored args if valid
func (s *ConfirmationTokenStore) Confirm(token string) (tool string, args map[string]any, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.store[token]
	if !ok {
		return "", nil, fmt.Errorf("invalid or expired confirmation token")
	}
	if time.Now().After(e.ExpiresAt) {
		delete(s.store, token)
		return "", nil, fmt.Errorf("confirmation token expired")
	}
	delete(s.store, token)
	return e.Tool, e.Args, nil
}

// ToolRateLimiter provides simple per-tool rate limiting
type ToolRateLimiter struct {
	mu       sync.Mutex
	lastCall map[string]time.Time
	interval time.Duration
}

// NewToolRateLimiter creates a rate limiter with min interval between calls per tool
func NewToolRateLimiter(interval time.Duration) *ToolRateLimiter {
	return &ToolRateLimiter{
		lastCall: make(map[string]time.Time),
		interval: interval,
	}
}

// Allow returns true if the call is allowed (not rate limited)
func (r *ToolRateLimiter) Allow(tool string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	last, ok := r.lastCall[tool]
	if !ok || now.Sub(last) >= r.interval {
		r.lastCall[tool] = now
		return true
	}
	return false
}
