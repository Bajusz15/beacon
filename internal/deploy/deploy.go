package deploy

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"beacon/internal/config"
	"beacon/internal/keys"
	"beacon/internal/state"
	"beacon/internal/util"
)

func CheckForNewTag(cfg *config.Config, status *state.Status) {
	// Get Git token from key manager if token name is specified
	gitToken, err := getGitToken(cfg)
	if err != nil {
		log.Printf("[Beacon] Failed to get Git token: %v", err)
		return
	}

	// Set up Git authentication
	setupGitAuth(cfg, gitToken)

	lastTag, _ := status.Get()

	// Check if we need to do initial deployment
	shouldDeploy := false
	if stat, err := os.Stat(cfg.LocalPath); os.IsNotExist(err) {
		log.Println("[Beacon] Local path does not exist. Cloning repository...")
		shouldDeploy = true
	} else if err == nil && stat.IsDir() {
		entries, _ := os.ReadDir(cfg.LocalPath)
		if len(entries) == 0 {
			log.Println("[Beacon] Local path is empty. Cloning repository...")
			shouldDeploy = true
		}
	}

	if shouldDeploy {
		// Clone default branch for initial deployment
		err := Deploy(cfg, "", status)
		if err != nil {
			log.Printf("[Beacon] Error during initial deployment: %v\n", err)
			return
		}
		log.Printf("[Beacon] Repository cloned to %s.\n", cfg.LocalPath)
		return
	}

	// For existing repos, fetch latest tags and check for updates
	latestTag := getLatestTagFromRepo(cfg)
	if latestTag == "" || latestTag == lastTag {
		return
	}

	log.Printf("[Beacon] New tag found: %s (prev: %s)\n", latestTag, lastTag)
	if err := Deploy(cfg, latestTag, status); err != nil {
		log.Printf("[Beacon] Error deploying: %v\n", err)
	}
}

func Deploy(cfg *config.Config, tag string, status *state.Status) error {
	if tag == "" {
		log.Printf("[Beacon] Deploying default branch...\n")
	} else {
		log.Printf("[Beacon] Deploying tag %s...\n", tag)
	}

	if err := os.RemoveAll(cfg.LocalPath); err != nil {
		log.Printf("[Beacon] Error removing local path %s: %v\n", cfg.LocalPath, err)
		return err
	}

	parentDir := filepath.Dir(cfg.LocalPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		log.Printf("[Beacon] Error creating parent directory %s: %v\n", parentDir, err)
		return err
	}

	repoURL := cfg.RepoURL
	if cfg.GitToken != "" && len(repoURL) > 8 && repoURL[:8] == "https://" {
		repoURL = "https://" + cfg.GitToken + "@" + repoURL[8:]
	}

	// Clone the repository
	// Set working directory to parentDir to avoid "Unable to read current working directory" errors
	var cloneCmd *exec.Cmd
	var stderr strings.Builder
	if tag == "" {
		// Clone default branch
		cloneCmd = exec.Command("git", "clone", repoURL, cfg.LocalPath)
	} else {
		// Clone specific tag
		cloneCmd = exec.Command("git", "clone", "--branch", tag, repoURL, cfg.LocalPath)
	}
	cloneCmd.Dir = parentDir // Set working directory to parent to avoid CWD issues
	cloneCmd.Stderr = &stderr

	if err := cloneCmd.Run(); err != nil {
		log.Printf("[Beacon] Error cloning repository: %v\n", err)
		log.Printf("[Beacon] Git error output: %s\n", stderr.String())
		return err
	}

	// Execute deploy command if specified
	if cfg.DeployCommand != "" {
		log.Printf("[Beacon] Executing deploy command: %s\n", cfg.DeployCommand)

		// Build the command with secure environment file sourcing
		var command string
		if cfg.SecureEnvPath != "" {
			// Check if secure env file exists
			if _, err := os.Stat(cfg.SecureEnvPath); err == nil {
				log.Printf("[Beacon] Sourcing secure environment file: %s\n", cfg.SecureEnvPath)
				command = fmt.Sprintf("set -a && . %s && set +a && %s", cfg.SecureEnvPath, cfg.DeployCommand)
			} else {
				log.Printf("[Beacon] Warning: Secure environment file not found: %s\n", cfg.SecureEnvPath)
				log.Printf("[Beacon] Running deploy command without secure environment\n")
				command = cfg.DeployCommand
			}
		} else {
			command = cfg.DeployCommand
		}

		// Execute the command - set working directory to avoid CWD issues
		cmd := exec.Command("sh", "-c", command)
		cmd.Dir = cfg.LocalPath // Set working directory to project directory
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			log.Printf("[Beacon] Deploy command failed: %v\n", err)
			return err
		}

		log.Printf("[Beacon] Deploy command completed successfully\n")
	}

	// Store the tag (or "default" for default branch)
	tagToStore := tag
	if tag == "" {
		tagToStore = "default"
	}
	status.Set(tagToStore, time.Now())

	if tag == "" {
		log.Printf("[Beacon] Deployment of default branch complete.\n")
	} else {
		log.Printf("[Beacon] Deployment of tag %s complete.\n", tag)
	}
	return nil
}

func getLatestTagFromRepo(cfg *config.Config) string {
	// Check if repository exists
	if _, err := os.Stat(cfg.LocalPath); os.IsNotExist(err) {
		log.Printf("[Beacon] Repository path does not exist: %s\n", cfg.LocalPath)
		return ""
	}

	// Fetch latest tags - set working directory to avoid CWD issues
	fetchCmd := exec.Command("git", "fetch", "--tags")
	fetchCmd.Dir = cfg.LocalPath
	if err := fetchCmd.Run(); err != nil {
		log.Printf("[Beacon] Error fetching tags: %v\n", err)
		return ""
	}

	// Get the latest tag - set working directory to avoid CWD issues
	forEachCmd := exec.Command("sh", "-c", "git for-each-ref --sort=-creatordate --format='%(refname:short)' refs/tags | head -n 1")
	forEachCmd.Dir = cfg.LocalPath
	output, err := forEachCmd.Output()
	if err != nil {
		log.Printf("[Beacon] Error getting latest tag: %v\n", err)
		return ""
	}

	return strings.TrimSpace(string(output))
}

// getGitToken retrieves the Git token from config or key manager
func getGitToken(cfg *config.Config) (string, error) {
	// If token is directly specified in config, use it
	if cfg.GitToken != "" {
		return cfg.GitToken, nil
	}

	// If token_name is specified, get it from key manager
	if cfg.GitTokenName != "" {
		configDir := getConfigDir()
		keyManager, err := keys.NewKeyManager(configDir)
		if err != nil {
			return "", fmt.Errorf("failed to initialize key manager: %w", err)
		}

		storedKey, err := keyManager.GetKey(cfg.GitTokenName)
		if err != nil {
			return "", fmt.Errorf("failed to get Git token '%s': %w", cfg.GitTokenName, err)
		}

		return storedKey.Key, nil
	}

	return "", fmt.Errorf("no Git token or token_name specified in configuration")
}

// setupGitAuth configures Git authentication based on the provided token
func setupGitAuth(cfg *config.Config, gitToken string) {
	// Set up SSH key if provided
	if cfg.SSHKeyPath != "" {
		util.LogError(os.Setenv("GIT_SSH_COMMAND", "ssh -i "+cfg.SSHKeyPath+" -o IdentitiesOnly=yes -o StrictHostKeyChecking=no"), "set up SSH key")
		return
	}

	// Set up Git token authentication
	if gitToken != "" {
		// Configure Git to use the token for HTTPS authentication
		util.LogError(os.Setenv("GIT_ASKPASS", "echo"), "set up Git token authentication")
		util.LogError(os.Setenv("GIT_USERNAME", "token"), "set up Git token authentication")
		util.LogError(os.Setenv("GIT_PASSWORD", gitToken), "set up Git token authentication")

		// For GitHub, GitLab, etc., we need to modify the URL to include the token
		if strings.Contains(cfg.RepoURL, "github.com") {
			// GitHub: https://token@github.com/user/repo.git
			modifiedURL := strings.Replace(cfg.RepoURL, "https://", fmt.Sprintf("https://%s@", gitToken), 1)
			util.LogError(os.Setenv("BEACON_REPO_URL", modifiedURL), "set up github repository URL")
		} else if strings.Contains(cfg.RepoURL, "gitlab.com") {
			// GitLab: https://oauth2:token@gitlab.com/user/repo.git
			modifiedURL := strings.Replace(cfg.RepoURL, "https://", fmt.Sprintf("https://oauth2:%s@", gitToken), 1)
			util.LogError(os.Setenv("BEACON_REPO_URL", modifiedURL), "set up gitlab repository URL")
		}
	}
}

// getConfigDir returns the beacon configuration directory
func getConfigDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".beacon"
	}
	return filepath.Join(homeDir, ".beacon")
}
