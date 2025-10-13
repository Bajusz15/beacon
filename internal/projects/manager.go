package projects

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"beacon/internal/config"
)

// ProjectManager manages Beacon projects using unified configuration
type ProjectManager struct {
	paths *config.BeaconPaths
}

// NewProjectManager creates a new ProjectManager
func NewProjectManager() (*ProjectManager, error) {
	paths, err := config.NewBeaconPaths()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize paths: %v", err)
	}

	return &ProjectManager{
		paths: paths,
	}, nil
}

// CreateProjectCommand creates the project management command
func CreateProjectCommand() *cobra.Command {
	pm, err := NewProjectManager()
	if err != nil {
		fmt.Printf("❌ Failed to initialize project manager: %v\n", err)
		return nil
	}

	var projectCmd = &cobra.Command{
		Use:   "projects",
		Short: "Manage Beacon projects",
		Long: `Manage Beacon projects with unified configuration.

This command provides utilities for listing, removing, and managing
Beacon projects in a consistent way.`,
		Example: `  beacon projects list
  beacon projects remove myapp
  beacon projects info myapp`,
	}

	projectCmd.AddCommand(createListCommand(pm))
	projectCmd.AddCommand(createRemoveCommand(pm))
	projectCmd.AddCommand(createInfoCommand(pm))
	projectCmd.AddCommand(createCleanCommand(pm))

	return projectCmd
}

func createListCommand(pm *ProjectManager) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all Beacon projects",
		Long:  `List all configured Beacon projects with their status.`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := pm.ListProjects(); err != nil {
				fmt.Printf("❌ Failed to list projects: %v\n", err)
				return
			}
		},
	}
}

func createRemoveCommand(pm *ProjectManager) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "remove [project-name]",
		Short: "Remove a Beacon project",
		Long:  `Remove a Beacon project and all its associated files and configurations.`,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			projectName := args[0]

			if !force {
				fmt.Printf("⚠️  This will permanently remove project '%s' and all its files.\n", projectName)
				fmt.Print("Are you sure? (y/N): ")
				var response string
				fmt.Scanln(&response)
				if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
					fmt.Println("❌ Operation cancelled")
					return
				}
			}

			if err := pm.RemoveProject(projectName); err != nil {
				fmt.Printf("❌ Failed to remove project: %v\n", err)
				return
			}

			fmt.Printf("✅ Project '%s' removed successfully\n", projectName)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force removal without confirmation")

	return cmd
}

func createInfoCommand(pm *ProjectManager) *cobra.Command {
	return &cobra.Command{
		Use:   "info [project-name]",
		Short: "Show project information",
		Long:  `Show detailed information about a specific Beacon project.`,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			projectName := args[0]

			if err := pm.ShowProjectInfo(projectName); err != nil {
				fmt.Printf("❌ Failed to show project info: %v\n", err)
				return
			}
		},
	}
}

func createCleanCommand(pm *ProjectManager) *cobra.Command {
	return &cobra.Command{
		Use:   "clean",
		Short: "Clean up orphaned files",
		Long:  `Clean up orphaned files and directories that are no longer associated with any project.`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := pm.CleanupOrphanedFiles(); err != nil {
				fmt.Printf("❌ Failed to clean up files: %v\n", err)
				return
			}
		},
	}
}

// ListProjects lists all configured projects
func (pm *ProjectManager) ListProjects() error {
	projects, err := pm.paths.ListProjects()
	if err != nil {
		return err
	}

	if len(projects) == 0 {
		fmt.Println("📭 No projects configured")
		fmt.Println()
		fmt.Println("Create your first project with:")
		fmt.Println("  beacon bootstrap myapp")
		return nil
	}

	fmt.Printf("📁 Beacon Projects (%d)\n", len(projects))
	fmt.Println()

	for _, project := range projects {
		configDir := pm.paths.GetProjectConfigDir(project)
		workingDir := pm.paths.GetProjectWorkingDir(project)
		envFile := pm.paths.GetProjectEnvFile(project)

		fmt.Printf("🔹 %s\n", project)
		fmt.Printf("   Config: %s\n", pm.paths.GetRelativePath(configDir))
		fmt.Printf("   Working: %s\n", pm.paths.GetRelativePath(workingDir))
		
		// Check if files exist
		if _, err := os.Stat(envFile); err == nil {
			fmt.Printf("   Status: ✅ Configured\n")
		} else {
			fmt.Printf("   Status: ⚠️  Missing env file\n")
		}
		fmt.Println()
	}

	return nil
}

// RemoveProject removes a project and all its associated files
func (pm *ProjectManager) RemoveProject(projectName string) error {
	if !pm.paths.ProjectExists(projectName) {
		return fmt.Errorf("project '%s' does not exist", projectName)
	}

	return pm.paths.RemoveProject(projectName)
}

// ShowProjectInfo shows detailed information about a project
func (pm *ProjectManager) ShowProjectInfo(projectName string) error {
	if !pm.paths.ProjectExists(projectName) {
		return fmt.Errorf("project '%s' does not exist", projectName)
	}

	configDir := pm.paths.GetProjectConfigDir(projectName)
	workingDir := pm.paths.GetProjectWorkingDir(projectName)
	envFile := pm.paths.GetProjectEnvFile(projectName)
	monitorFile := pm.paths.GetProjectMonitorFile(projectName)
	alertsFile := pm.paths.GetProjectAlertsFile(projectName)

	fmt.Printf("📊 Project: %s\n", projectName)
	fmt.Println()
	fmt.Printf("📂 Configuration Directory: %s\n", configDir)
	fmt.Printf("📂 Working Directory: %s\n", workingDir)
	fmt.Println()
	fmt.Println("📄 Files:")
	fmt.Printf("   Environment: %s", envFile)
	if _, err := os.Stat(envFile); err == nil {
		fmt.Printf(" ✅\n")
	} else {
		fmt.Printf(" ❌\n")
	}

	fmt.Printf("   Monitor Config: %s", monitorFile)
	if _, err := os.Stat(monitorFile); err == nil {
		fmt.Printf(" ✅\n")
	} else {
		fmt.Printf(" ❌\n")
	}

	fmt.Printf("   Alerts Config: %s", alertsFile)
	if _, err := os.Stat(alertsFile); err == nil {
		fmt.Printf(" ✅\n")
	} else {
		fmt.Printf(" ❌\n")
	}

	fmt.Println()
	fmt.Println("🔧 Systemd Services:")
	userService := pm.paths.GetSystemdServiceFile(projectName, false)
	systemService := pm.paths.GetSystemdServiceFile(projectName, true)

	fmt.Printf("   User Service: %s", userService)
	if _, err := os.Stat(userService); err == nil {
		fmt.Printf(" ✅\n")
	} else {
		fmt.Printf(" ❌\n")
	}

	fmt.Printf("   System Service: %s", systemService)
	if _, err := os.Stat(systemService); err == nil {
		fmt.Printf(" ✅\n")
	} else {
		fmt.Printf(" ❌\n")
	}

	return nil
}

// CleanupOrphanedFiles cleans up orphaned files and directories
func (pm *ProjectManager) CleanupOrphanedFiles() error {
	fmt.Println("🧹 Cleaning up orphaned files...")

	// Get list of projects
	projects, err := pm.paths.ListProjects()
	if err != nil {
		return err
	}

	// Check working directory for orphaned projects
	workingDir := pm.paths.WorkingDir
	if entries, err := os.ReadDir(workingDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				projectName := entry.Name()
				found := false
				for _, project := range projects {
					if project == projectName {
						found = true
						break
					}
				}
				if !found {
					fmt.Printf("🗑️  Removing orphaned working directory: %s\n", projectName)
					os.RemoveAll(filepath.Join(workingDir, projectName))
				}
			}
		}
	}

	fmt.Println("✅ Cleanup completed")
	return nil
}
