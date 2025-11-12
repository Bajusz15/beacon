package bootstrap

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"strings"
	"text/template"

	"beacon/internal/config"
	"beacon/internal/systemd"
	"beacon/internal/util"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// BootstrapConfig holds configuration for bootstrapping a new Beacon project
type BootstrapConfig struct {
	ProjectName      string `yaml:"project_name"`
	RepoURL          string `yaml:"repo_url"`
	LocalPath        string `yaml:"local_path"`
	DeployCommand    string `yaml:"deploy_command"`
	PollInterval     string `yaml:"poll_interval"`
	Port             string `yaml:"port"`
	SSHKeyPath       string `yaml:"ssh_key_path"`
	GitToken         string `yaml:"git_token"`
	SecureEnvPath    string `yaml:"secure_env_path"`
	User             string `yaml:"user"`
	WorkingDir       string `yaml:"working_dir"`
	UseSystemService bool   `yaml:"use_system_service"`
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

	// Create project structure
	if err := bm.paths.CreateProjectStructure(projectName); err != nil {
		return fmt.Errorf("failed to create project structure: %v", err)
	}

	// Collect configuration
	config, err := bm.collectConfiguration(projectName)
	if err != nil {
		return fmt.Errorf("failed to collect configuration: %v", err)
	}

	// Create environment file
	if err := bm.createEnvironmentFile(config); err != nil {
		return fmt.Errorf("failed to create environment file: %v", err)
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

	// Display success message
	bm.displaySuccessMessage(config, systemdCreated)

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

	// Create directory structure
	if err := bm.paths.CreateProjectStructure(projectName); err != nil {
		return fmt.Errorf("failed to create project structure: %v", err)
	}

	// Create environment file
	if err := bm.createEnvironmentFile(config); err != nil {
		return fmt.Errorf("failed to create environment file: %v", err)
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

	// Display success message
	bm.displaySuccessMessage(config, systemdCreated)

	return nil
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

	config := &BootstrapConfig{
		ProjectName:      projectName,
		RepoURL:          promptForInput("Enter Git repository URL", "https://github.com/yourusername/yourrepo.git"),
		LocalPath:        promptForInput("Enter local deployment path", bm.paths.GetProjectWorkingDir(projectName)),
		DeployCommand:    promptForInput("Enter deploy command (optional)", ""),
		PollInterval:     promptForInput("Enter polling interval", "60s"),
		Port:             promptForInput("Enter HTTP server port", "8080"),
		SSHKeyPath:       promptForInput("Enter SSH key path (optional)", ""),
		GitToken:         promptForInput("Enter Git token (optional)", ""),
		SecureEnvPath:    promptForInput("Enter secure environment file path (optional)", fmt.Sprintf("/etc/beacon/%s.env", projectName)),
		User:             currentUser.Username,
		WorkingDir:       bm.paths.GetProjectWorkingDir(projectName),
		UseSystemService: bm.serviceManager != nil && bm.serviceManager.IsAvailable(),
	}

	return config, nil
}

// createEnvironmentFile creates the environment file for the project
func (bm *BootstrapManager) createEnvironmentFile(config *BootstrapConfig) error {
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
		fmt.Println("4. Set up monitoring configuration:")
		fmt.Printf("   cp beacon.monitor.example.yml %s\n", bm.paths.GetProjectMonitorFile(config.ProjectName))
		fmt.Println("   # Or use the wizard BEFORE bootstrap: beacon setup-wizard")
		fmt.Println("5. Start monitoring:")
		fmt.Printf("   beacon monitor -f %s\n", bm.paths.GetProjectMonitorFile(config.ProjectName))
	} else {
		fmt.Println("2. Verify deployment succeeded:")
		fmt.Printf("   # Check if repository was cloned to: %s\n", config.WorkingDir)
		if config.DeployCommand != "" {
			fmt.Printf("   # Verify deploy command completed: %s\n", config.DeployCommand)
		}
		fmt.Println("3. Set up monitoring configuration:")
		fmt.Printf("   cp beacon.monitor.example.yml %s\n", bm.paths.GetProjectMonitorFile(config.ProjectName))
		fmt.Println("   # Or use the wizard BEFORE bootstrap: beacon setup-wizard")
		fmt.Println("4. Start monitoring:")
		fmt.Printf("   beacon monitor -f %s\n", bm.paths.GetProjectMonitorFile(config.ProjectName))
	}
	fmt.Println()
	fmt.Println("💡 Tip: Use 'beacon --help' to see all available commands")
}

// promptForInput prompts the user for input with a default value
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
