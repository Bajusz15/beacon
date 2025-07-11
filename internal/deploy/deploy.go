package deploy

import (
	"log"
	"os"
	"os/exec"
	"time"

	"beacon/internal/config"
	"beacon/internal/state"
	"beacon/internal/util"
)

func CheckForNewTag(cfg *config.Config, status *state.Status) {
	// Set up SSH key if provided
	if cfg.SSHKeyPath != "" {
		os.Setenv("GIT_SSH_COMMAND", "ssh -i "+cfg.SSHKeyPath+" -o IdentitiesOnly=yes -o StrictHostKeyChecking=no")
	}

	repoURL := cfg.RepoURL
	// Inject token for HTTPS if provided
	if cfg.GitToken != "" && len(repoURL) > 8 && repoURL[:8] == "https://" {
		repoURL = "https://" + cfg.GitToken + "@" + repoURL[8:]
	}

	cmd := exec.Command("git", "ls-remote", "--tags", repoURL)
	output, err := cmd.Output()
	if err != nil {
		log.Println("[Beacon] Error checking tags:", err)
		return
	}

	latestTag := parseLatestTag(string(output))
	lastTag, _ := status.Get()

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

	// Deploy if first run or new tag
	if shouldDeploy {
		Deploy(cfg, latestTag, status)
		log.Printf("[Beacon] Repository cloned to %s at tag %s.\n", cfg.LocalPath, latestTag)
		return
	}

	if latestTag == "" || latestTag == lastTag {
		return
	}

	log.Printf("[Beacon] New tag found: %s (prev: %s)\n", latestTag, lastTag)
	Deploy(cfg, latestTag, status)
}

func Deploy(cfg *config.Config, tag string, status *state.Status) {
	log.Printf("[Beacon] Deploying tag %s...\n", tag)

	os.RemoveAll(cfg.LocalPath)

	repoURL := cfg.RepoURL
	if cfg.GitToken != "" && len(repoURL) > 8 && repoURL[:8] == "https://" {
		repoURL = "https://" + cfg.GitToken + "@" + repoURL[8:]
	}

	exec.Command("git", "clone", "--branch", tag, repoURL, cfg.LocalPath).Run()

	// You can add shell commands, Docker run, etc. here
	status.Set(tag, time.Now())
	log.Printf("[Beacon] Deployment of tag %s complete.\n", tag)
}

func parseLatestTag(output string) string {
	lines := util.SplitLines(output)
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if line == "" {
			continue
		}
		parts := util.SplitFields(line)
		if len(parts) == 2 {
			ref := parts[1]
			if len(ref) > 10 && ref[:10] == "refs/tags/" {
				return ref[10:]
			}
		}
	}
	return ""
}
