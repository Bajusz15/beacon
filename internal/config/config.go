package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

type Config struct {
	RepoURL       string
	LocalPath     string
	PollInterval  time.Duration
	Port          string
	SSHKeyPath    string
	GitToken      string
	DeployCommand string
}

func Load() *Config {
	// apiKey := getEnvOrPrompt("BEACON_API_KEY", "Enter your Beacon API key")
	// projectName := getEnvOrPrompt("BEACON_PROJECT_NAME", "Enter your project name")

	cfg := &Config{
		RepoURL:       getEnvOrPrompt("BEACON_REPO_URL", "Enter the Git repo URL", "https://github.com/yourusername/yourrepo.git"),
		LocalPath:     os.ExpandEnv(getEnvOrPrompt("BEACON_LOCAL_PATH", "Enter the local path for the project", "$HOME/beacon/project")),
		PollInterval:  getDurationEnv("BEACON_POLL_INTERVAL", 60*time.Second),
		Port:          getEnvOrPrompt("BEACON_PORT", "Enter the port to run on", "8080"),
		SSHKeyPath:    getEnvOrPrompt("BEACON_SSH_KEY_PATH", "Enter the SSH key path (optional)", ""),
		GitToken:      getEnvOrPrompt("BEACON_GIT_TOKEN", "Enter the Git token (optional)", ""),
		DeployCommand: getEnvOrPrompt("BEACON_DEPLOY_COMMAND", "Enter the deploy command to run after update (optional)", ""),
	}

	ensureDir(cfg.LocalPath)

	return cfg
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvOrPrompt(key, prompt, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" && isInteractive() {
		if defaultValue != "" {
			fmt.Printf("%s [%s]: ", prompt, defaultValue)
		} else {
			fmt.Printf("%s: ", prompt)
		}
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			value = input
		} else {
			value = defaultValue
		}
		os.Setenv(key, value)
	}
	if value == "" {
		value = defaultValue
	}
	return value
}

func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func ensureDir(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "[Beacon] Failed to create local path %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("[Beacon] Created directory: %s\n", path)
	}
}
