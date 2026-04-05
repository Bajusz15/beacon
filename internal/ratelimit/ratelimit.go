package ratelimit

import (
	"beacon/internal/logging"
	"beacon/internal/util"
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

var logger = logging.New("ratelimit")

// RateLimiter handles rate limiting and backoff for external API calls
type RateLimiter struct {
	mu                sync.Mutex
	requestsPerMinute int
	requestsPerHour   int
	minInterval       time.Duration
	maxInterval       time.Duration
	backoffMultiplier float64
	maxRetries        int

	// Internal state
	lastRequest    time.Time
	requestCount   int
	windowStart    time.Time
	currentBackoff time.Duration
	retryCount     int
}

// Config holds rate limiting configuration
type Config struct {
	RequestsPerMinute int           `yaml:"requests_per_minute"`
	RequestsPerHour   int           `yaml:"requests_per_hour"`
	MinInterval       time.Duration `yaml:"min_interval"`
	MaxInterval       time.Duration `yaml:"max_interval"`
	BackoffMultiplier float64       `yaml:"backoff_multiplier"`
	MaxRetries        int           `yaml:"max_retries"`
}

// DefaultConfig returns sensible defaults for rate limiting
func DefaultConfig() *Config {
	return &Config{
		RequestsPerMinute: 15,  // 1 request per second
		RequestsPerHour:   900, // 1 request per second
		MinInterval:       3 * time.Second,
		MaxInterval:       60 * time.Second,
		BackoffMultiplier: 2.0,
		MaxRetries:        5,
	}
}

// NewRateLimiter creates a new rate limiter with the given configuration
func NewRateLimiter(config *Config) *RateLimiter {
	if config == nil {
		config = DefaultConfig()
	}

	return &RateLimiter{
		requestsPerMinute: config.RequestsPerMinute,
		requestsPerHour:   config.RequestsPerHour,
		minInterval:       config.MinInterval,
		maxInterval:       config.MaxInterval,
		backoffMultiplier: config.BackoffMultiplier,
		maxRetries:        config.MaxRetries,
		currentBackoff:    config.MinInterval,
		windowStart:       time.Now(),
	}
}

// Wait blocks until it's safe to make the next request
func (rl *RateLimiter) Wait(ctx context.Context) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Reset counters if window has passed
	if now.Sub(rl.windowStart) >= time.Minute {
		rl.requestCount = 0
		rl.windowStart = now
	}

	// Check if we need to wait due to rate limiting
	if rl.requestCount >= rl.requestsPerMinute {
		waitTime := time.Minute - now.Sub(rl.windowStart)
		if waitTime > 0 {
			logger.Infof("Rate limit reached, waiting %v", waitTime)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitTime):
				// Reset counters after waiting
				rl.requestCount = 0
				rl.windowStart = time.Now()
			}
		}
	}

	// Check minimum interval between requests
	if !rl.lastRequest.IsZero() {
		timeSinceLastRequest := now.Sub(rl.lastRequest)
		if timeSinceLastRequest < rl.minInterval {
			waitTime := rl.minInterval - timeSinceLastRequest
			logger.Infof("Minimum interval not met, waiting %v", waitTime)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitTime):
			}
		}
	}

	// Apply current backoff if we're in retry mode
	if rl.retryCount > 0 {
		logger.Infof("Applying backoff: %v (retry %d/%d)", rl.currentBackoff, rl.retryCount, rl.maxRetries)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(rl.currentBackoff):
		}
	}

	// Update state
	rl.lastRequest = time.Now()
	rl.requestCount++

	return nil
}

// RecordSuccess resets the backoff counter on successful request
func (rl *RateLimiter) RecordSuccess() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.retryCount = 0
	rl.currentBackoff = rl.minInterval
}

// RecordFailure increases the backoff for the next request
func (rl *RateLimiter) RecordFailure() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.retryCount++
	if rl.retryCount <= rl.maxRetries {
		// Exponential backoff
		rl.currentBackoff = time.Duration(float64(rl.currentBackoff) * rl.backoffMultiplier)
		if rl.currentBackoff > rl.maxInterval {
			rl.currentBackoff = rl.maxInterval
		}
		logger.Infof("Failure recorded, new backoff: %v", rl.currentBackoff)
	} else {
		logger.Infof("Max retries (%d) exceeded", rl.maxRetries)
	}
}

// ShouldRetry returns true if we should retry after a failure
func (rl *RateLimiter) ShouldRetry() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.retryCount <= rl.maxRetries
}

// GetStats returns current rate limiter statistics
func (rl *RateLimiter) GetStats() map[string]interface{} {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	return map[string]interface{}{
		"requests_per_minute": rl.requestsPerMinute,
		"requests_per_hour":   rl.requestsPerHour,
		"min_interval":        rl.minInterval.String(),
		"max_interval":        rl.maxInterval.String(),
		"current_backoff":     rl.currentBackoff.String(),
		"retry_count":         rl.retryCount,
		"max_retries":         rl.maxRetries,
		"request_count":       rl.requestCount,
		"window_start":        rl.windowStart.Format(time.RFC3339),
		"last_request":        rl.lastRequest.Format(time.RFC3339),
	}
}

// HTTPClient wraps an http.Client with rate limiting
type HTTPClient struct {
	Client      *http.Client
	rateLimiter *RateLimiter
}

// NewHTTPClient creates a new rate-limited HTTP client
func NewHTTPClient(config *Config) *HTTPClient {
	return &HTTPClient{
		Client:      &http.Client{Timeout: 30 * time.Second},
		rateLimiter: NewRateLimiter(config),
	}
}

// Do performs an HTTP request with rate limiting and retry logic
func (c *HTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	for {
		// Wait for rate limiter
		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter wait failed: %w", err)
		}

		// Make the request
		resp, err := c.Client.Do(req)

		// Handle the response
		if err != nil {
			c.rateLimiter.RecordFailure()
			if !c.rateLimiter.ShouldRetry() {
				return nil, fmt.Errorf("max retries exceeded, last error: %w", err)
			}
			continue
		}

		// Check HTTP status codes
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			c.rateLimiter.RecordSuccess()
			return resp, nil
		}

		// Handle rate limiting responses
		if resp.StatusCode == 429 {
			c.rateLimiter.RecordFailure()
			logger.Infof("HTTP 429 received, applying backoff")
			util.LogError(resp.Body.Close(), "HTTP response body close")

			if !c.rateLimiter.ShouldRetry() {
				return nil, fmt.Errorf("max retries exceeded after HTTP 429")
			}
			continue
		}

		// Handle other error status codes
		if resp.StatusCode >= 400 {
			c.rateLimiter.RecordFailure()
			util.LogError(resp.Body.Close(), "HTTP response body close")

			if !c.rateLimiter.ShouldRetry() {
				return nil, fmt.Errorf("max retries exceeded, last status: %d", resp.StatusCode)
			}
			continue
		}

		// Success
		c.rateLimiter.RecordSuccess()
		return resp, nil
	}
}

// GetStats returns the rate limiter statistics
func (c *HTTPClient) GetStats() map[string]interface{} {
	return c.rateLimiter.GetStats()
}
