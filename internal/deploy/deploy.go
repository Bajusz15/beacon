package deploy

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"beacon/internal/config"
	"beacon/internal/state"
)

func CheckForNewTag(cfg *config.Config, status *state.Status) {
	// Set up SSH key if provided
	if cfg.SSHKeyPath != "" {
		os.Setenv("GIT_SSH_COMMAND", "ssh -i "+cfg.SSHKeyPath+" -o IdentitiesOnly=yes -o StrictHostKeyChecking=no")
	}

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
	Deploy(cfg, latestTag, status)
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
	var cloneCmd *exec.Cmd
	var stderr strings.Builder
	if tag == "" {
		// Clone default branch
		cloneCmd = exec.Command("git", "clone", repoURL, cfg.LocalPath)
	} else {
		// Clone specific tag
		cloneCmd = exec.Command("git", "clone", "--branch", tag, repoURL, cfg.LocalPath)
	}
	cloneCmd.Stderr = &stderr

	if err := cloneCmd.Run(); err != nil {
		log.Printf("[Beacon] Error cloning repository: %v\n", err)
		log.Printf("[Beacon] Git error output: %s\n", stderr.String())
		return err
	}

	// Execute deploy command if specified
	if cfg.DeployCommand != "" {
		log.Printf("[Beacon] Executing deploy command: %s\n", cfg.DeployCommand)

		// Change to the project directory
		originalDir, _ := os.Getwd()
		os.Chdir(cfg.LocalPath)
		defer os.Chdir(originalDir)

		// Execute the command
		cmd := exec.Command("sh", "-c", cfg.DeployCommand)
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
	// Change to the project directory
	originalDir, _ := os.Getwd()
	os.Chdir(cfg.LocalPath)
	defer os.Chdir(originalDir)

	// Fetch latest tags
	fetchCmd := exec.Command("git", "fetch", "--tags")
	if err := fetchCmd.Run(); err != nil {
		log.Printf("[Beacon] Error fetching tags: %v\n", err)
		return ""
	}

	// Get the latest tag
	// describeCmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	// output, err := describeCmd.Output()
	forEachCmd := exec.Command("sh", "-c", "git for-each-ref --sort=-creatordate --format='%(refname:short)' refs/tags | head -n 1")
	output, err := forEachCmd.Output()
	if err != nil {
		log.Printf("[Beacon] Error getting latest tag: %v\n", err)
		return ""
	}

	return strings.TrimSpace(string(output))
}
