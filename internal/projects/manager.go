package projects

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"beacon/internal/config"
	"beacon/internal/state"

	"github.com/spf13/cobra"
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
  beacon projects status
  beacon projects status myapp
  beacon projects remove myapp
  beacon projects info myapp`,
	}

	projectCmd.AddCommand(createListCommand(pm))
	projectCmd.AddCommand(createStatusCommand(pm))
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
				_, err := fmt.Scanln(&response)
				if err != nil {
					fmt.Printf("❌ Failed to read response: %v\n", err)
					return
				}
				if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
					fmt.Println("❌ Operation canceled")
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

func createStatusCommand(pm *ProjectManager) *cobra.Command {
	return &cobra.Command{
		Use:   "status [project-name]",
		Short: "Show project health status",
		Long:  `Show check health status (up/down) from last monitor run. With no project name, show all projects.`,
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := pm.ShowStatus(args); err != nil {
				fmt.Printf("❌ Failed to show status: %v\n", err)
				return
			}
		},
	}
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

// readChecksState reads ~/.beacon/state/<project>/checks.json if present
func (pm *ProjectManager) readChecksState(projectName string) (*state.ChecksState, error) {
	path := filepath.Join(pm.paths.StateDir, projectName, "checks.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var st state.ChecksState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// healthSummary returns a short health summary from checks state (e.g. "3 up, 1 down", "all up", "degraded")
func healthSummary(st *state.ChecksState) string {
	if st == nil || len(st.Checks) == 0 {
		return "—"
	}
	var up, down int
	for _, c := range st.Checks {
		if c.Status == "up" {
			up++
		} else {
			down++
		}
	}
	if down == 0 {
		return "all up"
	}
	if up == 0 {
		return "degraded"
	}
	return fmt.Sprintf("%d up, %d down", up, down)
}

// ListProjects lists all configured projects with health summary when available
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

		if _, err := os.Stat(envFile); err == nil {
			fmt.Printf("   Config status: ✅ Configured\n")
		} else {
			fmt.Printf("   Config status: ⚠️  Missing env file\n")
		}

		st, err := pm.readChecksState(project)
		if err == nil {
			fmt.Printf("   Health: %s\n", healthSummary(st))
		} else {
			fmt.Printf("   Health: —\n")
		}
		fmt.Println()
	}

	return nil
}

// ShowStatus prints health status for one or all projects (per-check detail)
func (pm *ProjectManager) ShowStatus(args []string) error {
	if len(args) == 1 {
		return pm.showStatusOne(args[0])
	}
	projects, err := pm.paths.ListProjects()
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		fmt.Println("📭 No projects configured")
		return nil
	}
	for _, project := range projects {
		if err := pm.showStatusOne(project); err != nil {
			fmt.Printf("🔹 %s: no health data\n", project)
		}
		fmt.Println()
	}
	return nil
}

func (pm *ProjectManager) showStatusOne(projectName string) error {
	st, err := pm.readChecksState(projectName)
	if err != nil {
		return err
	}
	fmt.Printf("📊 %s (updated %s)\n", projectName, st.UpdatedAt.Format(time.RFC3339))
	if len(st.Checks) == 0 {
		fmt.Println("   No checks")
		return nil
	}
	for _, c := range st.Checks {
		icon := "✅"
		if c.Status != "up" {
			icon = "❌"
		}
		line := fmt.Sprintf("   %s %s %s", icon, c.Name, c.Status)
		if c.Error != "" {
			line += fmt.Sprintf(" — %s", c.Error)
		}
		fmt.Println(line)
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
					err := os.RemoveAll(filepath.Join(workingDir, projectName))
					if err != nil {
						return fmt.Errorf("failed to remove orphaned working directory: %w", err)
					}
				}
			}
		}
	}

	fmt.Println("✅ Cleanup completed")
	return nil
}
