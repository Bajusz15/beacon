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
	cmd := exec.Command("git", "ls-remote", "--tags", cfg.RepoURL)
	output, err := cmd.Output()
	if err != nil {
		log.Println("[Beacon] Error checking tags:", err)
		return
	}

	latestTag := parseLatestTag(string(output))
	lastTag, _ := status.Get()
	if latestTag == "" || latestTag == lastTag {
		return
	}

	log.Printf("[Beacon] New tag found: %s (prev: %s)\n", latestTag, lastTag)
	Deploy(cfg, latestTag, status)
}

func Deploy(cfg *config.Config, tag string, status *state.Status) {
	log.Printf("[Beacon] Deploying tag %s...\n", tag)

	os.RemoveAll(cfg.LocalPath)
	exec.Command("git", "clone", "--branch", tag, cfg.RepoURL, cfg.LocalPath).Run()

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
