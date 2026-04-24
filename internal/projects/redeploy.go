package projects

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func createRedeployCommand(pm *ProjectManager) *cobra.Command {
	return &cobra.Command{
		Use:   "redeploy <project-name>",
		Short: "Pull latest changes and re-run the deploy command",
		Long: `Fetches the latest code for a project and re-runs its deploy command.

The project must be in the inventory (beacon projects list) and have a
working directory with a git repository. The deploy command is read from
the project's env file (BEACON_DEPLOY_CMD).`,
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

// Redeploy pulls latest code and re-runs the deploy command for a project.
func (pm *ProjectManager) Redeploy(projectName string) error {
	inv, err := LoadInventory(pm.paths.GetProjectsFilePath())
	if err != nil {
		return fmt.Errorf("load inventory: %w", err)
	}
	entry := GetProject(inv, projectName)
	if entry == nil {
		return fmt.Errorf("project %q not found in inventory (run `beacon projects list` to see available projects)", projectName)
	}

	location := entry.Location
	if location == "" {
		location = pm.paths.GetProjectWorkingDir(projectName)
	}
	if _, err := os.Stat(location); err != nil {
		return fmt.Errorf("project directory %s does not exist", location)
	}

	envFile := pm.paths.GetProjectEnvFile(projectName)
	deployCmd := readEnvVar(envFile, "BEACON_DEPLOY_CMD")

	fmt.Printf("Redeploying %s in %s\n", projectName, location)

	fmt.Println("Pulling latest changes...")
	pull := exec.Command("git", "pull")
	pull.Dir = location
	pull.Stdout = os.Stdout
	pull.Stderr = os.Stderr
	if err := pull.Run(); err != nil {
		return fmt.Errorf("git pull failed: %w", err)
	}

	if deployCmd == "" {
		fmt.Println("No BEACON_DEPLOY_CMD configured — skipping deploy command.")
		fmt.Println("Done (code updated, no deploy command ran).")
		return nil
	}

	fmt.Printf("Running deploy command: %s\n", deployCmd)

	secureEnvPath := readEnvVar(envFile, "BEACON_SECURE_ENV_PATH")
	command := deployCmd
	if secureEnvPath != "" {
		if _, err := os.Stat(os.ExpandEnv(secureEnvPath)); err == nil {
			command = fmt.Sprintf("set -a && . %s && set +a && %s", secureEnvPath, deployCmd)
		}
	}

	deploy := exec.Command("sh", "-c", command)
	deploy.Dir = location
	deploy.Stdout = os.Stdout
	deploy.Stderr = os.Stderr
	if err := deploy.Run(); err != nil {
		return fmt.Errorf("deploy command failed: %w", err)
	}

	fmt.Printf("Redeploy of %s complete.\n", projectName)
	return nil
}

// readEnvVar reads a specific variable from a KEY=VALUE env file.
func readEnvVar(path, key string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	prefix := key + "="
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, prefix) {
			val := strings.TrimPrefix(line, prefix)
			val = strings.Trim(val, "\"'")
			return val
		}
	}
	return ""
}
