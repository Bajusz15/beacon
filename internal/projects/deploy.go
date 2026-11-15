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

func createDeployCommand(pm *ProjectManager) *cobra.Command {
	return &cobra.Command{
		Use:   "deploy <project-name> <git-ref>",
		Short: "Deploy a project at a specific Git ref",
		Long: `Deploys a project at the supplied Git ref.

The ref can be a tag, branch, or commit SHA. This is useful for testing a
specific commit without moving the stable release tag. Use "projects redeploy"
to return to the latest tag-based deployment.`,
		Example: `  beacon projects deploy beaconinfra 8f3c2a1
  beacon projects deploy beaconinfra feature/storage-test
  beacon projects redeploy beaconinfra`,
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if err := pm.DeployRef(args[0], args[1]); err != nil {
				fmt.Printf("Deploy failed: %v\n", err)
				os.Exit(1)
			}
		},
	}
}

// DeployRef runs a full deploy cycle for a project at a specific Git ref.
func (pm *ProjectManager) DeployRef(projectName, ref string) error {
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
	if cfg.DeploymentType != "" && cfg.DeploymentType != "git" {
		return fmt.Errorf("specific Git refs are only supported for git projects, not %q", cfg.DeploymentType)
	}

	statusDir := filepath.Join(os.Getenv("HOME"), ".beacon", cfg.ProjectDir)
	status := state.NewStatus(statusDir)

	fmt.Printf("Deploying %s at %s from %s\n", projectName, ref, cfg.LocalPath)
	if err := deploy.DeployRef(cfg, ref, status); err != nil {
		return err
	}

	fmt.Printf("Deploy of %s at %s complete.\n", projectName, ref)
	return nil
}
