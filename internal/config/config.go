package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"beacon/internal/util"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// DockerImageConfig holds configuration for a single Docker image to monitor
type DockerImageConfig struct {
	Image    string // Full image name (e.g., "username/app" or "ghcr.io/username/app")
	Registry string // Registry URL (e.g., "docker.io", "ghcr.io") - optional, auto-detected
	Username string // Registry username (optional)
	Password string // Registry password (optional)
	// Token is an alternative auth mechanism (e.g., GHCR token)
	Token              string   // Registry token (optional, alternative to username/password)
	DeployCommand      string   // Command to run when new tag is found
	DockerComposeFiles []string // Optional list of compose files used for this image's deploy command
}

type Config struct {
	// Deployment type: "git" or "docker"
	DeploymentType string

	// Git repository configuration
	RepoURL      string
	LocalPath    string
	SSHKeyPath   string
	GitToken     string
	GitTokenName string // Name of stored Git token

	// Docker registry configuration (array of images)
	DockerImages []DockerImageConfig

	// Common configuration
	PollInterval  time.Duration
	Port          string
	DeployCommand string // Legacy: kept for backward compatibility
	SecureEnvPath string // Path to secure environment file for deploy command
	ProjectDir    string
}

func Load() *Config {
	// First, check if BEACON_SECURE_ENV_PATH is set in environment (from systemd or bootstrap env file)
	// If it is, load the secure env file before reading other config values
	secureEnvPath := os.Getenv("BEACON_SECURE_ENV_PATH")
	if secureEnvPath != "" {
		secureEnvPath = os.ExpandEnv(secureEnvPath)
		if err := util.LoadEnvFile(secureEnvPath); err != nil {
			fmt.Fprintf(os.Stderr, "[Beacon] Warning: Failed to load secure environment file %s: %v\n", secureEnvPath, err)
		}
	}

	// Determine deployment type (default to "git" for backward compatibility)
	deploymentType := getEnvOrPrompt("BEACON_DEPLOYMENT_TYPE", "Enter deployment type (git or docker)", "git")
	if deploymentType != "git" && deploymentType != "docker" {
		deploymentType = "git" // Default to git if invalid
	}

	cfg := &Config{
		DeploymentType: deploymentType,
		PollInterval:   getDurationEnv("BEACON_POLL_INTERVAL", 60*time.Second),
		Port:           getEnvOrPrompt("BEACON_PORT", "Enter the port to run on", "8080"),
		DeployCommand:  getEnvOrPrompt("BEACON_DEPLOY_CMD", "Enter the deploy command to run after update (optional)", ""),
		SecureEnvPath:  getEnvOrPrompt("BEACON_SECURE_ENV_PATH", "Enter secure environment file path (optional)", "$HOME/beacon/project/.env"),
	}

	switch deploymentType {
	case "git":
		cfg.RepoURL = getEnvOrPrompt("BEACON_REPO_URL", "Enter the Git repo URL", "https://github.com/yourusername/yourrepo.git")
		cfg.LocalPath = os.ExpandEnv(getEnvOrPrompt("BEACON_LOCAL_PATH", "Enter the local path for the project", "$HOME/beacon/project"))
		cfg.SSHKeyPath = getEnvOrPrompt("BEACON_SSH_KEY_PATH", "Enter the SSH key path (optional)", "")
		cfg.GitToken = getEnvOrPrompt("BEACON_GIT_TOKEN", "Enter the Git token (optional)", "")
		ensureDir(cfg.LocalPath)
	case "docker":
		// Docker images / stacks are configured via bootstrap or config file
		cfg.LocalPath = os.ExpandEnv(getEnvOrPrompt("BEACON_LOCAL_PATH", "Enter the local path for the project", "$HOME/beacon/project"))
		ensureDir(cfg.LocalPath)

		projectName := os.Getenv("BEACON_PROJECT_NAME")
		if projectName == "" {
			projectName = filepath.Base(cfg.LocalPath)
		}

		// Load Docker images from docker-images.yml if it exists
		dockerImagesPath := filepath.Join(os.Getenv("HOME"), ".beacon", "config", "projects", projectName, "docker-images.yml")
		if images, err := loadDockerImagesConfig(dockerImagesPath); err == nil {
			cfg.DockerImages = images
		} else if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "[Beacon] Warning: Failed to load Docker images config: %v\n", err)
		}

	}

	cfg.ProjectDir = filepath.Base(cfg.LocalPath)

	return cfg
}

// loadDockerImagesConfig loads Docker images configuration from a YAML file
func loadDockerImagesConfig(path string) ([]DockerImageConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var images []DockerImageConfig
	if err := yaml.Unmarshal(data, &images); err != nil {
		return nil, fmt.Errorf("failed to parse Docker images config: %w", err)
	}
	return images, nil
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
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintf(os.Stderr, "[Beacon] Failed to set environment variable %s: %v\n", key, err)
			os.Exit(1)
		}
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
