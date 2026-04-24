package bootstrap

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"text/template"

	"beacon/internal/config"
	"beacon/internal/identity"
	"beacon/internal/projects"
	"beacon/internal/systemd"
	"beacon/internal/util"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// DockerImageBootstrapConfig holds bootstrap configuration for a single Docker image
type DockerImageBootstrapConfig struct {
	Image              string   `yaml:"image"`
	Registry           string   `yaml:"registry"`
	Username           string   `yaml:"username"`
	Password           string   `yaml:"password"`
	Token              string   `yaml:"token"`
	DeployCommand      string   `yaml:"deploy_command"`
	DockerComposeFiles []string `yaml:"docker_compose_files"` // Array of compose files
}

// BootstrapConfig holds configuration for bootstrapping a new Beacon project
type BootstrapConfig struct {
	ProjectName    string `yaml:"project_name"`
	DeploymentType string `yaml:"deployment_type"` // "git" or "docker"

	// Git repository configuration
	RepoURL    string `yaml:"repo_url"`
	SSHKeyPath string `yaml:"ssh_key_path"`
	GitToken   string `yaml:"git_token"`

	// Docker registry configuration (array of images)
	DockerImages []DockerImageBootstrapConfig `yaml:"docker_images"`

	// Common configuration
	LocalPath        string `yaml:"local_path"`
	DeployCommand    string `yaml:"deploy_command"`
	PollInterval     string `yaml:"poll_interval"`
	Port             string `yaml:"port"`
	SecureEnvPath    string `yaml:"secure_env_path"`
	User             string `yaml:"user"`
	WorkingDir       string `yaml:"working_dir"`
	UseSystemService bool   `yaml:"use_system_service"`

	// Machine-wide master agent (~/.beacon/config.yaml). Written on first bootstrap if missing.
	CloudReportingEnabled *bool `yaml:"cloud_reporting_enabled,omitempty"`
}

// BootstrapManager manages the bootstrap process using unified configuration
type BootstrapManager struct {
	paths          *config.BeaconPaths
	serviceManager *systemd.ServiceManager
}

// NewBootstrapManager creates a new BootstrapManager
func NewBootstrapManager(useSystemService bool) (*BootstrapManager, error) {
	paths, err := config.NewBeaconPaths()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize paths: %v", err)
	}

	var serviceType systemd.ServiceType
	if useSystemService {
		serviceType = systemd.SystemService
	} else {
		serviceType = systemd.UserService
	}

	serviceManager := systemd.NewServiceManager(serviceType)

	return &BootstrapManager{
		paths:          paths,
		serviceManager: serviceManager,
	}, nil
}

// BootstrapProject bootstraps a new Beacon project
func (bm *BootstrapManager) BootstrapProject(projectName string, skipSystemd bool) error {
	// Validate project name
	if err := bm.paths.ValidateProjectName(projectName); err != nil {
		return fmt.Errorf("invalid project name: %v", err)
	}

	// Check if project already exists
	if bm.paths.ProjectExists(projectName) {
		return fmt.Errorf("project '%s' already exists", projectName)
	}

	// Ensure all directories exist
	if err := bm.paths.EnsureDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %v", err)
	}

	if err := bm.ensureGlobalCloudSettings(true, nil); err != nil {
		fmt.Printf("Warning: global cloud settings: %v\n", err)
	}

	// Create project structure
	if err := bm.paths.CreateProjectStructure(projectName); err != nil {
		return fmt.Errorf("failed to create project structure: %v", err)
	}

	// Collect configuration (needed for inventory location)
	config, err := bm.collectConfiguration(projectName)
	if err != nil {
		return fmt.Errorf("failed to collect configuration: %v", err)
	}

	// Create environment file
	if err := bm.createEnvironmentFile(config); err != nil {
		return fmt.Errorf("failed to create environment file: %v", err)
	}

	// Create Docker deployment configs if needed
	if config.DeploymentType == "docker" {
		if len(config.DockerImages) > 0 {
			if err := bm.createDockerImagesConfig(config); err != nil {
				return fmt.Errorf("failed to create Docker images config: %v", err)
			}
		}
	}

	// Create systemd service if requested and available
	systemdCreated := false
	if !skipSystemd && bm.serviceManager.IsAvailable() {
		if err := bm.createSystemdService(config); err != nil {
			fmt.Printf("Warning: Failed to create systemd service: %v\n", err)
		} else {
			systemdCreated = true
		}
	}

	// Register project in inventory
	bm.addProjectToInventory(config)

	if err := identity.AppendProjectIfMissing(config.ProjectName, bm.paths.GetProjectMonitorFile(config.ProjectName)); err != nil {
		fmt.Printf("Warning: could not add project to master config: %v\n", err)
	}

	// Display success message
	bm.displaySuccessMessage(config, systemdCreated)

	bm.tryInstallMasterSystemd(skipSystemd)

	return nil
}

// BootstrapProjectFromConfig bootstraps a project using a configuration file
func (bm *BootstrapManager) BootstrapProjectFromConfig(projectName, configFile string, skipSystemd bool) error {
	// Validate project name
	if err := bm.paths.ValidateProjectName(projectName); err != nil {
		return fmt.Errorf("invalid project name: %v", err)
	}

	// Load configuration from file
	config, err := bm.loadConfigFromFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config file: %v", err)
	}

	// Override project name with the one provided as argument
	config.ProjectName = projectName

	// Set working directory if not specified
	if config.WorkingDir == "" {
		config.WorkingDir = bm.paths.GetProjectWorkingDir(projectName)
	}

	if err := bm.paths.EnsureDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %v", err)
	}
	if err := bm.ensureGlobalCloudSettings(false, config); err != nil {
		fmt.Printf("Warning: global cloud settings: %v\n", err)
	}

	// Create directory structure
	if err := bm.paths.CreateProjectStructure(projectName); err != nil {
		return fmt.Errorf("failed to create project structure: %v", err)
	}

	// Create environment file
	if err := bm.createEnvironmentFile(config); err != nil {
		return fmt.Errorf("failed to create environment file: %v", err)
	}

	// Create Docker deployment configs if needed
	if config.DeploymentType == "docker" {
		if len(config.DockerImages) > 0 {
			if err := bm.createDockerImagesConfig(config); err != nil {
				return fmt.Errorf("failed to create Docker images config: %v", err)
			}
		}
	}

	// Create systemd service if requested and available
	systemdCreated := false
	if !skipSystemd && bm.serviceManager.IsAvailable() {
		if err := bm.createSystemdService(config); err != nil {
			fmt.Printf("Warning: Failed to create systemd service: %v\n", err)
		} else {
			systemdCreated = true
		}
	}

	// Register project in inventory
	bm.addProjectToInventory(config)

	if err := identity.AppendProjectIfMissing(config.ProjectName, bm.paths.GetProjectMonitorFile(config.ProjectName)); err != nil {
		fmt.Printf("Warning: could not add project to master config: %v\n", err)
	}

	// Display success message
	bm.displaySuccessMessage(config, systemdCreated)

	bm.tryInstallMasterSystemd(skipSystemd)

	return nil
}

// addProjectToInventory adds or updates the project in projects.json
func (bm *BootstrapManager) addProjectToInventory(config *BootstrapConfig) {
	invPath := bm.paths.GetProjectsFilePath()
	inv, err := projects.LoadInventory(invPath)
	if err != nil {
		fmt.Printf("Warning: Failed to load project inventory: %v\n", err)
		return
	}
	location := filepath.Clean(os.ExpandEnv(config.LocalPath))
	if abs, err := filepath.Abs(location); err == nil {
		location = abs
	}
	configDir := bm.paths.GetProjectConfigDir(config.ProjectName)
	projects.AddProject(inv, config.ProjectName, location, configDir)
	if err := projects.SaveInventory(invPath, inv); err != nil {
		fmt.Printf("Warning: Failed to save project inventory: %v\n", err)
	}
}

// loadConfigFromFile loads bootstrap configuration from a YAML file
func (bm *BootstrapManager) loadConfigFromFile(configFile string) (*BootstrapConfig, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var config BootstrapConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	return &config, nil
}

// collectConfiguration collects configuration from user input
func (bm *BootstrapManager) collectConfiguration(projectName string) (*BootstrapConfig, error) {
	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %v", err)
	}

	// Ask for deployment type first
	deploymentType := promptForInput("Enter deployment type (git or docker)", "git")
	if deploymentType != "git" && deploymentType != "docker" {
		deploymentType = "git" // Default to git if invalid
	}

	config := &BootstrapConfig{
		ProjectName:      projectName,
		DeploymentType:   deploymentType,
		LocalPath:        promptForInput("Enter local deployment path", bm.paths.GetProjectWorkingDir(projectName)),
		DeployCommand:    promptForInput("Enter deploy command (optional)", ""),
		PollInterval:     promptForInput("Enter polling interval", "60s"),
		Port:             promptForInput("Enter HTTP server port", "8080"),
		SecureEnvPath:    promptForInput("Enter secure environment file path (optional)", fmt.Sprintf("/etc/beacon/%s.env", projectName)),
		User:             currentUser.Username,
		WorkingDir:       bm.paths.GetProjectWorkingDir(projectName),
		UseSystemService: bm.serviceManager != nil && bm.serviceManager.IsAvailable(),
	}

	// Collect deployment-specific configuration
	switch deploymentType {
	case "git":
		config.RepoURL = promptForInput("Enter Git repository URL", "https://github.com/yourusername/yourrepo.git")
		config.SSHKeyPath = promptForInput("Enter SSH key path (optional)", "")
		config.GitToken = promptForInput("Enter Git token (optional)", "")
	case "docker":
		// For interactive mode, collect at least one image
		// Users can add more images by editing the config file later
		fmt.Println("\n📦 Docker Image Configuration")
		fmt.Println("You can add more images later by editing the configuration file.")

		imgCfg := DockerImageBootstrapConfig{}
		imgCfg.Image = promptForInput("Enter Docker image name (e.g., username/app or ghcr.io/username/app)", "")
		imgCfg.Registry = promptForInput("Enter Docker registry (e.g., docker.io, ghcr.io) [optional, auto-detected]", "")
		imgCfg.Username = promptForInput("Enter Docker registry username (optional)", "")
		imgCfg.Password = promptForInput("Enter Docker registry password (optional)", "")
		imgCfg.Token = promptForInput("Enter Docker registry token (optional)", "")

		// Support multiple compose files (comma-separated or space-separated)
		composeFilesInput := promptForInput("Enter docker-compose file(s) (optional, comma or space separated, relative to local_path)", "")
		if composeFilesInput != "" {
			// Split by comma or space
			composeFiles := strings.Fields(strings.ReplaceAll(composeFilesInput, ",", " "))
			imgCfg.DockerComposeFiles = composeFiles
		}

		imgCfg.DeployCommand = promptForInput("Enter deploy command for this image (e.g., 'docker compose up -d' or 'docker run ...')", "")

		config.DockerImages = []DockerImageBootstrapConfig{imgCfg}
	}

	return config, nil
}

// createEnvironmentFile creates the environment file for the project
func (bm *BootstrapManager) createEnvironmentFile(config *BootstrapConfig) error {
	// Default to "git" when DeploymentType is unset so env file contains BEACON_REPO_URL etc.
	if config.DeploymentType == "" {
		config.DeploymentType = "git"
	}

	envPath := bm.paths.GetProjectEnvFile(config.ProjectName)

	file, err := os.Create(envPath)
	if err != nil {
		return fmt.Errorf("failed to create environment file: %v", err)
	}
	defer util.Close(file, "environment file")

	tmpl, err := template.New("env").Parse(envTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %v", err)
	}

	if err := tmpl.Execute(file, config); err != nil {
		return fmt.Errorf("failed to execute template: %v", err)
	}

	fmt.Printf("✅ Created environment file: %s\n", envPath)
	if config.GitToken != "" {
		fmt.Printf("✅ Git token configured in environment file\n")
	} else {
		fmt.Printf("⚠️  Warning: No Git token configured (will use SSH key if provided)\n")
	}
	return nil
}

// createDockerImagesConfig creates a YAML file with Docker images configuration
func (bm *BootstrapManager) createDockerImagesConfig(config *BootstrapConfig) error {
	dockerImagesPath := filepath.Join(bm.paths.GetProjectConfigDir(config.ProjectName), "docker-images.yml")

	file, err := os.Create(dockerImagesPath)
	if err != nil {
		return fmt.Errorf("failed to create Docker images config file: %v", err)
	}
	defer util.Close(file, "Docker images config file")

	data, err := yaml.Marshal(config.DockerImages)
	if err != nil {
		return fmt.Errorf("failed to marshal Docker images config: %v", err)
	}

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write Docker images config: %v", err)
	}

	fmt.Printf("✅ Created Docker images configuration: %s\n", dockerImagesPath)
	return nil
}

// createSystemdService creates a systemd service for the project
func (bm *BootstrapManager) createSystemdService(config *BootstrapConfig) error {
	serviceConfig := systemd.GetDefaultServiceConfig(
		config.ProjectName,
		bm.paths.GetProjectEnvFile(config.ProjectName),
		config.WorkingDir,
	)

	if err := bm.serviceManager.CreateService(serviceConfig); err != nil {
		return fmt.Errorf("failed to create service: %v", err)
	}

	if err := bm.serviceManager.ReloadDaemon(); err != nil {
		return fmt.Errorf("failed to reload daemon: %v", err)
	}

	// Get service path for display
	servicePath := bm.paths.GetSystemdServiceFile(config.ProjectName, config.UseSystemService)
	fmt.Printf("✅ Created systemd service: %s\n", servicePath)
	return nil
}

// displaySuccessMessage displays a success message with next steps
func (bm *BootstrapManager) displaySuccessMessage(config *BootstrapConfig, systemdCreated bool) {
	fmt.Println("\n🎉 Project bootstrapped successfully!")
	fmt.Println()
	fmt.Printf("📁 Project: %s\n", config.ProjectName)
	fmt.Printf("📂 Config: %s\n", bm.paths.GetProjectConfigDir(config.ProjectName))
	fmt.Printf("📂 Working: %s\n", config.WorkingDir)
	fmt.Printf("📄 Environment: %s\n", bm.paths.GetProjectEnvFile(config.ProjectName))
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("1. Review and edit the environment file if needed")
	if systemdCreated {
		fmt.Println("2. Start the deployment service (this will clone the repo and run your deploy command):")
		fmt.Printf("   systemctl --user start beacon@%s\n", config.ProjectName)
		fmt.Println("3. Check if deployment succeeded:")
		fmt.Printf("   # Verify the repository was cloned to: %s\n", config.WorkingDir)
		if config.DeployCommand != "" {
			fmt.Printf("   # Check if deploy command ran: %s\n", config.DeployCommand)
		}
		fmt.Printf("   systemctl --user status beacon@%s\n", config.ProjectName)
		fmt.Println("4. Add health checks (monitor config) for this project:")
		fmt.Printf("   cp beacon.monitor.example.yml %s\n", bm.paths.GetProjectMonitorFile(config.ProjectName))
		fmt.Println("   # Optional: beacon setup-wizard generates a monitor YAML elsewhere — copy or merge checks into the path above.")
		fmt.Println("5. Run the master agent (spawns child agents per project; local dashboard):")
		fmt.Println("   beacon master")
		fmt.Println("   # In another terminal: beacon status")
		fmt.Println("6. Debug a single project’s monitor without the master (optional):")
		fmt.Printf("   beacon monitor -f %s\n", bm.paths.GetProjectMonitorFile(config.ProjectName))
	} else {
		fmt.Println("2. Verify deployment succeeded:")
		fmt.Printf("   # Check if repository was cloned to: %s\n", config.WorkingDir)
		if config.DeployCommand != "" {
			fmt.Printf("   # Verify deploy command completed: %s\n", config.DeployCommand)
		}
		fmt.Println("3. Add health checks (monitor config):")
		fmt.Printf("   cp beacon.monitor.example.yml %s\n", bm.paths.GetProjectMonitorFile(config.ProjectName))
		fmt.Println("4. Run the master agent:")
		fmt.Println("   beacon master")
		fmt.Println("5. Optional — debug monitor only:")
		fmt.Printf("   beacon monitor -f %s\n", bm.paths.GetProjectMonitorFile(config.ProjectName))
	}
	fmt.Println()
	fmt.Println("💡 Tip: Use 'beacon --help' to see all available commands")
	uc, _ := identity.LoadUserConfig()
	if uc != nil && uc.CloudReportingEnabled {
		fmt.Println()
		fmt.Println("Cloud (optional): add BeaconInfra API key, then restart the master:")
		fmt.Println("  beacon cloud login              # or: beacon init first, then cloud later")
		fmt.Println("  systemctl --user restart beacon-master.service")
	}
}

func (bm *BootstrapManager) ensureGlobalCloudSettings(interactive bool, bc *BootstrapConfig) error {
	gp, err := identity.UserConfigPath()
	if err != nil {
		return err
	}
	_, statErr := os.Stat(gp)
	if statErr == nil {
		return nil
	}
	if !os.IsNotExist(statErr) {
		return statErr
	}
	enabled := true
	if bc != nil && bc.CloudReportingEnabled != nil {
		enabled = *bc.CloudReportingEnabled
	} else if interactive && term.IsTerminal(int(os.Stdin.Fd())) {
		enabled = promptYesNo("Send this host's health status to Beacon cloud?", true)
	}
	if err := identity.MergeBootstrapCloudOnly(enabled); err != nil {
		return err
	}
	fmt.Printf("✅ Created global settings: %s (cloud_reporting=%v)\n", gp, enabled)
	return nil
}

func promptYesNo(prompt string, defaultYes bool) bool {
	defShow := "Y/n"
	if !defaultYes {
		defShow = "y/N"
	}
	fmt.Printf("%s [%s]: ", prompt, defShow)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultYes
	}
	return line == "y" || line == "yes"
}

func (bm *BootstrapManager) tryInstallMasterSystemd(skipSystemd bool) {
	if skipSystemd || bm.serviceManager == nil || !bm.serviceManager.IsAvailable() {
		return
	}
	uc, err := identity.LoadUserConfig()
	if err != nil || uc == nil || !uc.CloudReportingEnabled {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		fmt.Printf("Warning: cannot install beacon-master.service: no home directory\n")
		return
	}
	exe := resolveBeaconExecutable()
	execLine := fmt.Sprintf("%s master --foreground", exe)
	if err := bm.serviceManager.CreateMasterService(execLine, home); err != nil {
		fmt.Printf("Warning: beacon-master.service: %v\n", err)
		return
	}
	if err := bm.serviceManager.ReloadDaemon(); err != nil {
		fmt.Printf("Warning: systemd daemon-reload: %v\n", err)
		return
	}
	if err := bm.serviceManager.EnableMasterService(); err != nil {
		fmt.Printf("Warning: systemctl enable beacon-master: %v\n", err)
		return
	}
	if err := bm.serviceManager.StartMasterService(); err != nil {
		fmt.Printf("Warning: systemctl start beacon-master: %v (run `beacon init`, then: systemctl --user restart beacon-master.service)\n", err)
		return
	}
	fmt.Printf("✅ Cloud master agent service installed: systemctl --user status beacon-master.service\n")
}

func resolveBeaconExecutable() string {
	if p, err := os.Executable(); err == nil && p != "" {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
		return p
	}
	return "/usr/local/bin/beacon"
}

// promptForInput roadmap the user for input with a default value
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

// Environment file template
const envTemplate = `# Beacon project environment file for {{.ProjectName}}

# Deployment type: "git" or "docker"
BEACON_DEPLOYMENT_TYPE={{.DeploymentType}}

{{- if eq .DeploymentType "git"}}
# Git repository configuration
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
{{- else if eq .DeploymentType "docker"}}
# Docker registry configuration
# Docker images are configured in docker-images.yml file in the project config directory
# See: ~/.beacon/config/projects/{{.ProjectName}}/docker-images.yml
{{- end}}

# Local deployment path
BEACON_LOCAL_PATH={{.LocalPath}}

# Deploy command (optional)
{{- if .DeployCommand}}
BEACON_DEPLOY_COMMAND={{.DeployCommand}}
{{- end}}

# Polling interval
BEACON_POLL_INTERVAL={{.PollInterval}}

# HTTP server port
BEACON_PORT={{.Port}}

# Secure environment file path (optional)
{{- if .SecureEnvPath}}
BEACON_SECURE_ENV_PATH={{.SecureEnvPath}}
{{- end}}

# Project metadata
BEACON_PROJECT_NAME={{.ProjectName}}
BEACON_USER={{.User}}
BEACON_WORKING_DIR={{.WorkingDir}}
`

// BootstrapCommand creates the bootstrap command
func BootstrapCommand() *cobra.Command {
	var skipSystemd bool
	var useSystemService bool
	var configFile string

	cmd := &cobra.Command{
		Use:   "bootstrap [project-name]",
		Short: "Bootstrap a new Beacon project",
		Long: `Bootstrap a new Beacon project with unified configuration management.

This command will:
- Create project directories in ~/.beacon/config/projects/
- Set up environment configuration
- Optionally create systemd services
- Handle all path management automatically

You can specify a configuration file using -f flag for non-interactive setup:
  beacon bootstrap myapp                    # Interactive setup
  beacon bootstrap myapp -f config.yml     # Use config file
  beacon bootstrap myapp --skip-systemd    # Skip systemd service creation
  beacon bootstrap myapp --system-service # Create system service instead of user service`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			projectName := args[0]

			bm, err := NewBootstrapManager(useSystemService)
			if err != nil {
				fmt.Printf("❌ Failed to initialize bootstrap manager: %v\n", err)
				return
			}

			if configFile != "" {
				// Use config file for non-interactive setup
				if err := bm.BootstrapProjectFromConfig(projectName, configFile, skipSystemd); err != nil {
					fmt.Printf("❌ Bootstrap failed: %v\n", err)
					return
				}
			} else {
				// Use interactive setup
				if err := bm.BootstrapProject(projectName, skipSystemd); err != nil {
					fmt.Printf("❌ Bootstrap failed: %v\n", err)
					return
				}
			}
		},
	}

	cmd.Flags().BoolVar(&skipSystemd, "skip-systemd", false, "Skip systemd service creation")
	cmd.Flags().BoolVar(&useSystemService, "system-service", false, "Create system service instead of user service")
	cmd.Flags().StringVarP(&configFile, "config", "f", "", "Path to bootstrap configuration file")

	return cmd
}
