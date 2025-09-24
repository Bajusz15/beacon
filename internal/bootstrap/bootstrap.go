package bootstrap

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/cobra"
)

type BootstrapConfig struct {
	ProjectName      string
	RepoURL          string
	LocalPath        string
	DeployCommand    string
	PollInterval     string
	Port             string
	SSHKeyPath       string
	GitToken         string
	SecureEnvPath    string // Path to secure environment file for deploy command
	User             string
	WorkingDir       string
	ProjectConfigDir string
}

const envTemplate = `# Beacon project environment file for {{.ProjectName}}

# Repository URL (supports both HTTPS and SSH)
BEACON_REPO_URL={{.RepoURL}}

# Path to SSH private key (optional, for SSH URLs)
{{- if .SSHKeyPath}}
BEACON_SSH_KEY_PATH={{.SSHKeyPath}}
{{- end}}

# Personal access token (optional, for HTTPS URLs)
{{- if .GitToken}}
BEACON_GIT_TOKEN={{.GitToken}}
{{- end}}

# Local deployment path
BEACON_LOCAL_PATH={{.LocalPath}}

# Deploy command to run after update (optional)
{{- if .DeployCommand}}
BEACON_DEPLOY_CMD={{.DeployCommand}}
{{- end}}

# Polling interval (e.g., 30s, 1m, 5m)
BEACON_POLL_INTERVAL={{.PollInterval}}

# HTTP server port
BEACON_PORT={{.Port}}

# Secure environment file path for deploy command (optional)
{{- if .SecureEnvPath}}
BEACON_SECURE_ENV_PATH={{.SecureEnvPath}}
{{- end}}
`

const systemdTemplate = `[Unit]
Description=Beacon Agent for {{.ProjectName}} - Lightweight deployment and reporting for IoT
After=network.target

[Service]
EnvironmentFile={{.ProjectConfigDir}}/env
{{- if .SecureEnvPath}}
# Secure environment file for deploy command
EnvironmentFile={{.SecureEnvPath}}
{{- end}}
Type=simple
ExecStart=/usr/local/bin/beacon deploy
Restart=always
RestartSec=5

# Logging
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=default.target
`

func Run(cmd *cobra.Command, args []string) {
	cmd.Println("[Beacon] Starting project bootstrap...")
	cmd.Println("[Beacon] This will create the necessary directory structure, configuration files,")
	cmd.Println("[Beacon] and optionally set up systemd services for your beacon project.")
	cmd.Println("")

	// Get flags
	force, _ := cmd.Flags().GetBool("force")
	skipSystemd, _ := cmd.Flags().GetBool("skip-systemd")

	if force {
		cmd.Println("[Beacon] Using --force flag: will overwrite existing components")
	}
	if skipSystemd {
		cmd.Println("[Beacon] Using --skip-systemd flag: will skip systemd service setup")
	}

	// Get project name from args or prompt
	projectName := ""
	if len(args) > 0 {
		projectName = args[0]
	} else {
		projectName = promptForInput("Enter project name", "")
		if projectName == "" {
			cmd.Println("[Beacon] Project name is required")
			return
		}
	}

	// Validate project name (no spaces, special chars)
	if !isValidProjectName(projectName) {
		cmd.Println("[Beacon] Invalid project name. Use only letters, numbers, hyphens, and underscores.")
		return
	}

	// Get current user
	currentUser, err := user.Current()
	if err != nil {
		cmd.Printf("[Beacon] Error getting current user: %v\n", err)
		return
	}

	// Check what already exists
	existingComponents := checkExistingComponents(projectName)
	if len(existingComponents) > 0 {
		cmd.Println("[Beacon] Found existing components:")
		for _, component := range existingComponents {
			cmd.Printf("  - %s\n", component)
		}

		if force {
			cmd.Println("[Beacon] Using --force flag, overwriting existing components")
		} else {
			overwrite := promptForInput("Overwrite existing components? (y/N)", "N")
			if strings.ToLower(overwrite) != "y" && strings.ToLower(overwrite) != "yes" {
				cmd.Println("[Beacon] Bootstrap cancelled")
				return
			}
		}
	}

	// Collect configuration
	config := &BootstrapConfig{
		ProjectName:   projectName,
		RepoURL:       promptForInput("Enter Git repository URL", "https://github.com/yourusername/yourrepo.git"),
		LocalPath:     promptForInput("Enter local deployment path", fmt.Sprintf("%s/beacon/%s", currentUser.HomeDir, projectName)),
		DeployCommand: promptForInput("Enter deploy command (optional)", ""),
		PollInterval:  promptForInput("Enter polling interval", "60s"),
		Port:          promptForInput("Enter HTTP server port", "8080"),
		SSHKeyPath:    promptForInput("Enter SSH key path (optional)", ""),
		GitToken:      promptForInput("Enter Git token (optional)", ""),
		SecureEnvPath: promptForInput("Enter secure environment file path (optional)", fmt.Sprintf("/etc/beacon/%s.env", projectName)),
		User:          currentUser.Username,
		WorkingDir:    fmt.Sprintf("%s/beacon/%s", currentUser.HomeDir, projectName),
	}

	// Validate configuration
	if err := validateConfiguration(config); err != nil {
		cmd.Printf("[Beacon] Configuration error: %v\n", err)
		return
	}

	// Create directory structure
	if err := createDirectoryStructure(config); err != nil {
		cmd.Printf("[Beacon] Error creating directories: %v\n", err)
		return
	}

	// Create environment file
	if err := createEnvironmentFile(config); err != nil {
		cmd.Printf("[Beacon] Error creating environment file: %v\n", err)
		return
	}

	// Check if systemd is available and ask user preference
	systemdAvailable := checkSystemdAvailable()
	if systemdAvailable && !skipSystemd {
		setupSystemd := promptForInput("Set up systemd service? (Y/n)", "Y")
		if strings.ToLower(setupSystemd) != "n" && strings.ToLower(setupSystemd) != "no" {
			if err := createSystemdService(config); err != nil {
				cmd.Printf("[Beacon] Error creating systemd service: %v\n", err)
				// Continue without systemd
			}
		} else {
			cmd.Println("[Beacon] Skipping systemd service setup")
		}
	} else if skipSystemd {
		cmd.Println("[Beacon] Skipping systemd service setup (--skip-systemd flag)")
	} else {
		cmd.Println("[Beacon] Systemd not available, skipping service setup")
	}

	// Set permissions
	if err := setPermissions(config); err != nil {
		cmd.Printf("[Beacon] Error setting permissions: %v\n", err)
		return
	}

	// Check if beacon binary exists
	if !checkBeaconBinary() {
		cmd.Println("[Beacon] Warning: Beacon binary not found at /usr/local/bin/beacon")
		cmd.Println("[Beacon] Please install beacon first or update the ExecStart path in the service file")
	}

	// Provide next steps
	cmd.Println("[Beacon] Bootstrap completed successfully!")
	cmd.Println("")
	cmd.Println("Next steps:")
	cmd.Printf("1. Review configuration: %s/env\n", config.ProjectConfigDir)
	cmd.Printf("2. Edit configuration if needed\n")

	if config.SecureEnvPath != "" {
		cmd.Printf("3. Set up secure environment file: %s\n", config.SecureEnvPath)
		cmd.Printf("   Add your deployment environment variables (API keys, database URLs, etc.)\n")
		cmd.Printf("   Example: sudo nano %s\n", config.SecureEnvPath)
		cmd.Printf("   Set permissions: sudo chmod 600 %s\n", config.SecureEnvPath)
	}

	// Check if systemd service was created
	homeDir := os.Getenv("HOME")
	servicePath := filepath.Join(homeDir, ".config", "systemd", "user", fmt.Sprintf("beacon@%s.service", projectName))
	if _, err := os.Stat(servicePath); err == nil {
		cmd.Println("3. Enable and start the user systemd service:")
		cmd.Printf("   systemctl --user daemon-reload\n")
		cmd.Printf("   systemctl --user enable beacon@%s\n", projectName)
		cmd.Printf("   systemctl --user start beacon@%s\n", projectName)
		cmd.Printf("   systemctl --user status beacon@%s\n", projectName)
		cmd.Printf("   journalctl --user -u beacon@%s -f\n", projectName)
	} else {
		cmd.Println("3. Run beacon manually:")
		cmd.Printf("   beacon deploy\n")
		cmd.Printf("   # Or run in background: nohup beacon deploy > beacon.log 2>&1 &\n")
	}

	cmd.Println("")
	cmd.Printf("4. Test deployment by checking the status endpoint: http://localhost:%s/status\n", config.Port)

	// Summary of what was created
	cmd.Println("")
	cmd.Println("Summary of created components:")
	cmd.Printf("  ✓ Project configuration: %s/\n", config.ProjectConfigDir)
	cmd.Printf("  ✓ Environment file: %s/env\n", config.ProjectConfigDir)
	cmd.Printf("  ✓ Working directory: %s\n", config.WorkingDir)

	if config.SecureEnvPath != "" {
		if _, err := os.Stat(config.SecureEnvPath); err == nil {
			cmd.Printf("  ✓ Secure environment file: %s\n", config.SecureEnvPath)
		} else {
			cmd.Printf("  ⚠ Secure environment file: %s (needs to be created)\n", config.SecureEnvPath)
		}
	}

	if _, err := os.Stat(servicePath); err == nil {
		cmd.Printf("  ✓ User systemd service: beacon@%s.service\n", projectName)
	} else {
		cmd.Printf("  ⚠ Systemd service: Not created\n")
	}
}

func promptForInput(prompt, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", prompt, defaultValue)
	} else {
		fmt.Printf("%s: ", prompt)
	}

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultValue
	}
	return input
}

func isValidProjectName(name string) bool {
	if name == "" {
		return false
	}

	// Check for invalid characters
	for _, char := range name {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_') {
			return false
		}
	}

	return true
}

func createDirectoryStructure(config *BootstrapConfig) error {
	// Create user-specific config directory
	homeDir := os.Getenv("HOME")
	projectConfigDir := filepath.Join(homeDir, ".beacon", "config", "projects", config.ProjectName)

	if err := os.MkdirAll(projectConfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create project config directory: %v", err)
	}

	// Check if working directory already exists
	if _, err := os.Stat(config.WorkingDir); err == nil {
		fmt.Printf("[Beacon] Working directory already exists: %s\n", config.WorkingDir)
	} else {
		// Create working directory
		if err := os.MkdirAll(config.WorkingDir, 0755); err != nil {
			return fmt.Errorf("failed to create working directory: %v", err)
		}
		fmt.Printf("[Beacon] Created working directory: %s\n", config.WorkingDir)
	}

	// Create the LocalPath directory (where git repo will be cloned)
	if _, err := os.Stat(config.LocalPath); err == nil {
		fmt.Printf("[Beacon] Local path already exists: %s\n", config.LocalPath)
	} else {
		// Create local path directory
		if err := os.MkdirAll(config.LocalPath, 0755); err != nil {
			return fmt.Errorf("failed to create local path directory: %v", err)
		}
		fmt.Printf("[Beacon] Created local path directory: %s\n", config.LocalPath)
	}

	// Create ~/.beacon for status storage
	beaconDataDir := filepath.Join(os.Getenv("HOME"), ".beacon")
	if err := os.MkdirAll(beaconDataDir, 0755); err != nil {
		return fmt.Errorf("failed to create beacon data directory: %v", err)
	}

	fmt.Printf("[Beacon] Created directories:\n")
	fmt.Printf("  - %s\n", projectConfigDir)
	fmt.Printf("  - %s\n", config.LocalPath)
	fmt.Printf("  - %s\n", beaconDataDir)

	// Store the config path for later use
	config.ProjectConfigDir = projectConfigDir

	return nil
}

func createEnvironmentFile(config *BootstrapConfig) error {
	envPath := filepath.Join(config.ProjectConfigDir, "env")

	file, err := os.Create(envPath)
	if err != nil {
		return fmt.Errorf("failed to create environment file: %v", err)
	}
	defer file.Close()

	tmpl, err := template.New("env").Parse(envTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %v", err)
	}

	if err := tmpl.Execute(file, config); err != nil {
		return fmt.Errorf("failed to execute template: %v", err)
	}

	fmt.Printf("[Beacon] Created environment file: %s\n", envPath)
	return nil
}

func createSystemdService(config *BootstrapConfig) error {
	// Create user systemd service directory
	homeDir := os.Getenv("HOME")
	userServiceDir := filepath.Join(homeDir, ".config", "systemd", "user")
	if err := os.MkdirAll(userServiceDir, 0755); err != nil {
		return fmt.Errorf("failed to create user systemd directory: %v", err)
	}

	servicePath := filepath.Join(userServiceDir, fmt.Sprintf("beacon@%s.service", config.ProjectName))

	file, err := os.Create(servicePath)
	if err != nil {
		return fmt.Errorf("failed to create systemd service file: %v", err)
	}
	defer file.Close()

	tmpl, err := template.New("systemd").Parse(systemdTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %v", err)
	}

	if err := tmpl.Execute(file, config); err != nil {
		return fmt.Errorf("failed to execute template: %v", err)
	}

	fmt.Printf("[Beacon] Created user systemd service: %s\n", servicePath)
	return nil
}

func setPermissions(config *BootstrapConfig) error {
	// Set ownership of working directory to current user
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %v", err)
	}

	// Parse user and group IDs
	uid, err := strconv.Atoi(currentUser.Uid)
	if err != nil {
		return fmt.Errorf("failed to parse user ID: %v", err)
	}

	gid, err := strconv.Atoi(currentUser.Gid)
	if err != nil {
		return fmt.Errorf("failed to parse group ID: %v", err)
	}

	// Change ownership of working directory
	if err := os.Chown(config.WorkingDir, uid, gid); err != nil {
		return fmt.Errorf("failed to change ownership of working directory: %v", err)
	}

	// Set permissions on environment file
	envPath := filepath.Join(config.ProjectConfigDir, "env")
	if err := os.Chmod(envPath, 0644); err != nil {
		return fmt.Errorf("failed to set permissions on environment file: %v", err)
	}

	fmt.Printf("[Beacon] Set permissions:\n")
	fmt.Printf("  - Working directory owned by %s\n", currentUser.Username)
	fmt.Printf("  - Environment file readable by all users\n")

	return nil
}

func checkBeaconBinary() bool {
	// Check if beacon binary exists at the expected location
	if _, err := os.Stat("/usr/local/bin/beacon"); err == nil {
		return true
	}

	// Also check if it's in PATH
	if _, err := exec.LookPath("beacon"); err == nil {
		return true
	}

	return false
}

func checkExistingComponents(projectName string) []string {
	var existing []string

	// Check project config directory
	homeDir := os.Getenv("HOME")
	projectConfigDir := filepath.Join(homeDir, ".beacon", "config", "projects", projectName)
	if _, err := os.Stat(projectConfigDir); err == nil {
		existing = append(existing, "Project config directory")
	}

	// Check environment file
	envPath := filepath.Join(projectConfigDir, "env")
	if _, err := os.Stat(envPath); err == nil {
		existing = append(existing, "Environment file")
	}

	// Check user systemd service
	servicePath := filepath.Join(homeDir, ".config", "systemd", "user", fmt.Sprintf("beacon@%s.service", projectName))
	if _, err := os.Stat(servicePath); err == nil {
		existing = append(existing, "User systemd service")
	}

	return existing
}

func checkSystemdAvailable() bool {
	// Check if systemd is available
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return true
	}

	// Check if systemctl command exists
	if _, err := exec.LookPath("systemctl"); err == nil {
		return true
	}

	return false
}

func validateConfiguration(config *BootstrapConfig) error {
	// Validate required fields
	if config.RepoURL == "" {
		return fmt.Errorf("repository URL is required")
	}

	if config.LocalPath == "" {
		return fmt.Errorf("local deployment path is required")
	}

	// Validate polling interval format
	if _, err := time.ParseDuration(config.PollInterval); err != nil {
		return fmt.Errorf("invalid polling interval format: %v", err)
	}

	// Validate port format
	if config.Port == "" {
		return fmt.Errorf("port is required")
	}

	// Basic port number validation
	if config.Port != "0" {
		if portNum, err := strconv.Atoi(config.Port); err != nil || portNum < 1 || portNum > 65535 {
			return fmt.Errorf("invalid port number: %s", config.Port)
		}
	}

	return nil
}
