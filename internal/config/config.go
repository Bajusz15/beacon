package config

import (
	"os"
	"time"
)

type Config struct {
	RepoURL      string
	LocalPath    string
	PollInterval time.Duration
	Port         string
}

func Load() *Config {
	return &Config{
		RepoURL:      getEnv("BEACON_REPO_URL", "https://github.com/yourusername/yourrepo.git"),
		LocalPath:    getEnv("BEACON_LOCAL_PATH", "/opt/beacon/project"),
		PollInterval: getDurationEnv("BEACON_POLL_INTERVAL", 60*time.Second),
		Port:         getEnv("BEACON_PORT", "8080"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
