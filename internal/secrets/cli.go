package secrets

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"beacon/internal/config"
	"beacon/internal/identity"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func Command() *cobra.Command {
	var project string
	var env string

	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Manage encrypted project secrets",
	}

	cmd.PersistentFlags().StringVar(&project, "project", "", "Project name")
	cmd.PersistentFlags().StringVar(&env, "env", "", "Deployment environment (default: project config, env file, or default)")

	setCmd := &cobra.Command{
		Use:   "set KEY [VALUE]",
		Short: "Set a secret",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedEnv, err := resolveCLIEnv(project, env)
			if err != nil {
				return err
			}
			value := ""
			if len(args) == 2 {
				value = args[1]
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "Secret value: ")
				data, err := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Fprintln(cmd.ErrOrStderr())
				if err != nil {
					return fmt.Errorf("read secret value: %w", err)
				}
				value = string(data)
			}
			store, err := NewStore()
			if err != nil {
				return err
			}
			if err := store.Set(project, resolvedEnv, args[0], value); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Secret %s set for %s/%s\n", args[0], project, resolvedEnv)
			return nil
		},
	}

	getCmd := &cobra.Command{
		Use:   "get KEY",
		Short: "Get a secret value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reveal, _ := cmd.Flags().GetBool("reveal")
			if !reveal {
				return fmt.Errorf("refusing to print secret without --reveal")
			}
			resolvedEnv, err := resolveCLIEnv(project, env)
			if err != nil {
				return err
			}
			store, err := NewStore()
			if err != nil {
				return err
			}
			value, err := store.Get(project, resolvedEnv, args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), value)
			return nil
		},
	}
	getCmd.Flags().Bool("reveal", false, "Print the secret value")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List secret keys",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			reveal, _ := cmd.Flags().GetBool("reveal")
			resolvedEnv, err := resolveCLIEnv(project, env)
			if err != nil {
				return err
			}
			store, err := NewStore()
			if err != nil {
				return err
			}
			if reveal {
				values, err := store.Load(project, resolvedEnv)
				if err != nil {
					return err
				}
				for _, key := range sortedSecretKeys(values) {
					fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", key, shellQuote(values[key]))
				}
			} else {
				keys, err := store.List(project, resolvedEnv)
				if err != nil {
					return err
				}
				for _, key := range keys {
					fmt.Fprintln(cmd.OutOrStdout(), key)
				}
			}
			return nil
		},
	}
	listCmd.Flags().Bool("reveal", false, "Print secret values")

	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Export secrets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			reveal, _ := cmd.Flags().GetBool("reveal")
			if !reveal {
				return fmt.Errorf("refusing to export secrets without --reveal")
			}
			format, _ := cmd.Flags().GetString("format")
			resolvedEnv, err := resolveCLIEnv(project, env)
			if err != nil {
				return err
			}
			store, err := NewStore()
			if err != nil {
				return err
			}
			values, err := store.Load(project, resolvedEnv)
			if err != nil {
				return err
			}
			switch format {
			case "env":
				for _, key := range sortedSecretKeys(values) {
					fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", key, shellQuote(values[key]))
				}
			case "json":
				encoded, err := json.MarshalIndent(values, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
			default:
				return fmt.Errorf("unsupported export format %q (use env or json)", format)
			}
			return nil
		},
	}
	exportCmd.Flags().Bool("reveal", false, "Print secret values")
	exportCmd.Flags().String("format", "env", "Export format: env or json")

	removeCmd := &cobra.Command{
		Use:   "remove KEY",
		Short: "Remove a secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedEnv, err := resolveCLIEnv(project, env)
			if err != nil {
				return err
			}
			store, err := NewStore()
			if err != nil {
				return err
			}
			if err := store.Remove(project, resolvedEnv, args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Secret %s removed from %s/%s\n", args[0], project, resolvedEnv)
			return nil
		},
	}

	cmd.AddCommand(setCmd, getCmd, listCmd, exportCmd, removeCmd)
	return cmd
}

func sortedSecretKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func resolveCLIEnv(project, flagEnv string) (string, error) {
	if err := ValidateProject(project); err != nil {
		return "", err
	}
	if flagEnv != "" {
		if err := ValidateEnv(flagEnv); err != nil {
			return "", err
		}
		return flagEnv, nil
	}
	if env, err := projectEnvFromUserConfig(project); err != nil {
		return "", err
	} else if env != "" {
		return env, nil
	}
	if env, err := projectEnvFromEnvFile(project); err != nil {
		return "", err
	} else if env != "" {
		return env, nil
	}
	return "default", nil
}

func projectEnvFromUserConfig(project string) (string, error) {
	uc, err := identity.LoadUserConfig()
	if err != nil {
		return "", err
	}
	if uc == nil {
		return "", nil
	}
	for _, p := range uc.Projects {
		if p.ID == project && p.Env != "" {
			if err := ValidateEnv(p.Env); err != nil {
				return "", err
			}
			return p.Env, nil
		}
	}
	return "", nil
}

func projectEnvFromEnvFile(project string) (string, error) {
	paths, err := config.NewBeaconPaths()
	if err != nil {
		return "", err
	}
	envPath := paths.GetProjectEnvFile(project)
	values, err := readEnvFile(envPath, os.Environ())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	env := strings.TrimSpace(values["BEACON_PROJECT_ENV"])
	if env == "" {
		return "", nil
	}
	if err := ValidateEnv(env); err != nil {
		return "", err
	}
	return env, nil
}
