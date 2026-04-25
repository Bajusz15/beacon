package projects

import (
	"fmt"
	"os"
	"path/filepath"

	"beacon/internal/config"
	"beacon/internal/deploy"
	"beacon/internal/state"
	"beacon/internal/util"

	"github.com/spf13/cobra"
)

func createRedeployCommand(pm *ProjectManager) *cobra.Command {
	return &cobra.Command{
		Use:   "redeploy <project-name>",
		Short: "Run a full deploy cycle for a project",
		Long: `Triggers a full deploy for a project using its existing configuration.

Loads the project's env file, builds the deploy config (same as beacon deploy),
and runs the full deploy cycle (clone + deploy command for git projects,
or the configured deploy flow for docker projects).`,
		Example: `  beacon projects redeploy myapp`,
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := pm.Redeploy(args[0]); err != nil {
				fmt.Printf("Redeploy failed: %v\n", err)
				os.Exit(1)
			}
		},
	}
}

// Redeploy runs a full deploy cycle for a project.
func (pm *ProjectManager) Redeploy(projectName string) error {
	if !pm.paths.ProjectExists(projectName) {
		return fmt.Errorf("project %q not found (run `beacon projects list` to see available projects)", projectName)
	}

	envFile := pm.paths.GetProjectEnvFile(projectName)
	if _, err := os.Stat(envFile); err != nil {
		return fmt.Errorf("env file not found: %s", envFile)
	}

	if err := util.LoadEnvFile(envFile); err != nil {
		return fmt.Errorf("load env file: %w", err)
	}

	cfg := config.Load()

	statusDir := filepath.Join(os.Getenv("HOME"), ".beacon", cfg.ProjectDir)
	status := state.NewStatus(statusDir)

	fmt.Printf("Deploying %s (%s) from %s\n", projectName, cfg.DeploymentType, cfg.LocalPath)

	if err := deploy.Deploy(cfg, "", status); err != nil {
		return err
	}

	fmt.Printf("Redeploy of %s complete.\n", projectName)
	return nil
}
