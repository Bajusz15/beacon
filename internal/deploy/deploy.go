package deploy

import (
	"fmt"
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
	// Determine deployment type (default to "git" for backward compatibility)
	deploymentType := cfg.DeploymentType
	if deploymentType == "" {
		deploymentType = "git"
	}

	switch deploymentType {
	case "docker":
		CheckForNewImageTag(cfg, status)
	case "git":
		fallthrough
	default:
		CheckForNewGitTag(cfg, status)
	}
}

func CheckForNewGitTag(cfg *config.Config, status *state.Status) {
	// Get Git token from key manager if token name is specified
	gitToken, err := getGitToken(cfg)
	if err != nil {
		logger.Infof("Failed to get Git token: %v", err)
		return
	}

	// Set up Git authentication
	setupGitAuth(cfg, gitToken)

	lastTag, _ := status.Get()

	// Check if we need to do initial deployment
	shouldDeploy := false
	if stat, err := os.Stat(cfg.LocalPath); os.IsNotExist(err) {
		logger.Infof("Local path does not exist. Cloning repository...")
		shouldDeploy = true
	} else if err == nil && stat.IsDir() {
		entries, _ := os.ReadDir(cfg.LocalPath)
		if len(entries) == 0 {
			logger.Infof("Local path is empty. Cloning repository...")
			shouldDeploy = true
		}
	}

	if shouldDeploy {
		latestTag := LatestGitTag(cfg)
		if latestTag == "" {
			logger.Infof("No Git tags found. Falling back to default branch for initial deployment...")
		}
		err := Deploy(cfg, latestTag, status)
		if err != nil {
			logger.Infof("Error during initial deployment: %v\n", err)
			return
		}
		logger.Infof("Repository cloned to %s.\n", cfg.LocalPath)
		return
	}

	// For existing repos, fetch latest tags and check for updates
	latestTag := getLatestTagFromRepo(cfg)
	if latestTag == "" || latestTag == lastTag {
		return
	}

	logger.Infof("New tag found: %s (prev: %s)\n", latestTag, lastTag)
	if err := Deploy(cfg, latestTag, status); err != nil {
		logger.Infof("Error deploying: %v\n", err)
	}
}

func Deploy(cfg *config.Config, tag string, status *state.Status) error {
	if tag == "" {
		logger.Infof("Deploying default branch...\n")
	} else {
		logger.Infof("Deploying tag %s...\n", tag)
	}

	if err := os.RemoveAll(cfg.LocalPath); err != nil {
		logger.Infof("Error removing local path %s: %v\n", cfg.LocalPath, err)
		return err
	}

	parentDir := filepath.Dir(cfg.LocalPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		logger.Infof("Error creating parent directory %s: %v\n", parentDir, err)
		return err
	}

	repoURL := authenticatedRepoURL(cfg.RepoURL, cfg.GitToken)

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
		errOutput := stderr.String()
		logger.Infof("Error cloning repository: %v\n", err)
		logger.Infof("Git error output: %s\n", errOutput)
		if isGitAuthFailure(errOutput) {
			authErr := &AuthError{Type: "git", Message: strings.TrimSpace(errOutput)}
			sd := stateBaseDir()
			pid := filepath.Base(cfg.LocalPath)
			_ = RecordCredentialError(sd, pid, authErr)
			return authErr
		}
		return err
	}
	// Successful clone — clear credential errors
	ClearCredentialErrors(stateBaseDir(), filepath.Base(cfg.LocalPath))

	// Execute deploy command if specified
	if cfg.DeployCommand != "" {
		logger.Infof("Executing deploy command: %s\n", cfg.DeployCommand)

		env, err := CommandEnv(cfg)
		if err != nil {
			return err
		}

		// Execute the command - set working directory to avoid CWD issues
		cmd := exec.Command("sh", "-c", cfg.DeployCommand)
		cmd.Dir = cfg.LocalPath // Set working directory to project directory
		cmd.Env = env
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			logger.Infof("Deploy command failed: %v\n", err)
			return err
		}

		logger.Infof("Deploy command completed successfully\n")
	}

	// Store the tag (or "default" for default branch)
	tagToStore := tag
	if tag == "" {
		tagToStore = "default"
	}
	status.Set(tagToStore, time.Now())

	if tag == "" {
		logger.Infof("Deployment of default branch complete.\n")
	} else {
		logger.Infof("Deployment of tag %s complete.\n", tag)
	}
	return nil
}

// DeployBranch does a shallow clone of the given branch and runs the deploy command.
// The stored status marker is the commit SHA so callers can tell exactly what was deployed.
func DeployBranch(cfg *config.Config, branch string, status *state.Status) error {
	gitToken, err := getGitToken(cfg)
	if err != nil {
		logger.Infof("Failed to get Git token: %v", err)
	}
	setupGitAuth(cfg, gitToken)

	logger.Infof("Deploying branch %s...\n", branch)

	if err := os.RemoveAll(cfg.LocalPath); err != nil {
		return fmt.Errorf("remove local path: %w", err)
	}
	parentDir := filepath.Dir(cfg.LocalPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	repoURL := authenticatedRepoURL(cfg.RepoURL, gitToken)

	var stderr strings.Builder
	cloneCmd := exec.Command("git", "clone", "--branch", branch, "--depth", "1", repoURL, cfg.LocalPath)
	cloneCmd.Dir = parentDir
	cloneCmd.Stderr = &stderr
	if err := cloneCmd.Run(); err != nil {
		errOutput := stderr.String()
		logger.Infof("Git error: %s\n", errOutput)
		if isGitAuthFailure(errOutput) {
			authErr := &AuthError{Type: "git", Message: strings.TrimSpace(errOutput)}
			_ = RecordCredentialError(stateBaseDir(), filepath.Base(cfg.LocalPath), authErr)
			return authErr
		}
		return fmt.Errorf("git clone branch %s: %w", branch, err)
	}
	ClearCredentialErrors(stateBaseDir(), filepath.Base(cfg.LocalPath))

	shaOut, _ := exec.Command("git", "-C", cfg.LocalPath, "rev-parse", "HEAD").Output()
	commitSHA := strings.TrimSpace(string(shaOut))
	if commitSHA == "" {
		commitSHA = branch
	}

	if cfg.DeployCommand != "" {
		logger.Infof("Executing deploy command: %s\n", cfg.DeployCommand)
		env, err := CommandEnv(cfg)
		if err != nil {
			return err
		}
		cmd := exec.Command("sh", "-c", cfg.DeployCommand)
		cmd.Dir = cfg.LocalPath
		cmd.Env = env
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("deploy command: %w", err)
		}
		logger.Infof("Deploy command completed successfully\n")
	}

	short := commitSHA
	if len(short) > 7 {
		short = short[:7]
	}
	status.Set(commitSHA, time.Now())
	logger.Infof("Branch %s deployed at %s\n", branch, short)
	return nil
}

func getLatestTagFromRepo(cfg *config.Config) string {
	// Check if repository exists
	if _, err := os.Stat(cfg.LocalPath); os.IsNotExist(err) {
		logger.Infof("Repository path does not exist: %s\n", cfg.LocalPath)
		return ""
	}

	// Fetch latest tags - set working directory to avoid CWD issues
	fetchCmd := exec.Command("git", "fetch", "--tags")
	fetchCmd.Dir = cfg.LocalPath
	if err := fetchCmd.Run(); err != nil {
		logger.Infof("Error fetching tags: %v\n", err)
		return ""
	}

	// Get the latest tag - set working directory to avoid CWD issues
	forEachCmd := exec.Command("sh", "-c", "git for-each-ref --sort=-creatordate --format='%(refname:short)' refs/tags | head -n 1")
	forEachCmd.Dir = cfg.LocalPath
	output, err := forEachCmd.Output()
	if err != nil {
		logger.Infof("Error getting latest tag: %v\n", err)
		return ""
	}

	return strings.TrimSpace(string(output))
}

// LatestGitTag returns the newest Git tag visible for cfg.RepoURL.
// It prefers an existing local checkout so fetch credentials/remotes behave as configured,
// then falls back to ls-remote for first deployments where LocalPath does not exist yet.
func LatestGitTag(cfg *config.Config) string {
	gitToken, err := getGitToken(cfg)
	if err != nil {
		logger.Infof("No Git token configured; trying Git operations without a token: %v\n", err)
	}
	setupGitAuth(cfg, gitToken)

	if st, err := os.Stat(cfg.LocalPath); err == nil && st.IsDir() {
		entries, _ := os.ReadDir(cfg.LocalPath)
		if len(entries) > 0 {
			if tag := getLatestTagFromRepo(cfg); tag != "" {
				return tag
			}
		}
	}

	return getLatestTagFromRemote(cfg, gitToken)
}

func getLatestTagFromRemote(cfg *config.Config, gitToken string) string {
	repoURL := authenticatedRepoURL(cfg.RepoURL, gitToken)
	cmd := exec.Command("git", "ls-remote", "--tags", "--sort=-creatordate", repoURL)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		errOutput := stderr.String()
		logger.Infof("Error listing remote tags: %v\n", err)
		if isGitAuthFailure(errOutput) {
			stateDir := stateBaseDir()
			projectID := filepath.Base(cfg.LocalPath)
			_ = RecordCredentialError(stateDir, projectID, &AuthError{Type: "git", Message: strings.TrimSpace(errOutput)})
		}
		return ""
	}
	// Successful git operation — clear any prior credential errors
	stateDir := stateBaseDir()
	projectID := filepath.Base(cfg.LocalPath)
	ClearCredentialErrors(stateDir, projectID)

	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasSuffix(line, "^{}") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		ref := strings.TrimPrefix(parts[1], "refs/tags/")
		if ref != "" {
			return ref
		}
	}
	return ""
}

func authenticatedRepoURL(repoURL, gitToken string) string {
	if gitToken != "" && strings.HasPrefix(repoURL, "https://") {
		return "https://" + gitToken + "@" + strings.TrimPrefix(repoURL, "https://")
	}
	return repoURL
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

	return "", nil
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
	base, err := config.BeaconHomeDir()
	if err != nil {
		return ".beacon"
	}
	return base
}

func stateBaseDir() string {
	base, err := config.BeaconHomeDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".beacon", "state")
	}
	return filepath.Join(base, "state")
}
