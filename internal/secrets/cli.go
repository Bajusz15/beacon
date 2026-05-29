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
	opts := &commandOptions{}

	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Manage encrypted project secrets",
	}

	cmd.PersistentFlags().StringVar(&opts.project, "project", "", "Project name")
	cmd.PersistentFlags().StringVar(&opts.env, "env", "", "Deployment environment (default: project config, env file, or default)")
	cmd.AddCommand(newSetCommand(opts), newGetCommand(opts), newListCommand(opts), newExportCommand(opts), newRemoveCommand(opts))
	return cmd
}

type commandOptions struct {
	project string
	env     string
}

func newSetCommand(opts *commandOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "set KEY [VALUE]",
		Short: "Set a secret",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedEnv, err := resolveCLIEnv(opts.project, opts.env)
			if err != nil {
				return err
			}
			value := ""
			if len(args) == 2 {
				value = args[1]
			} else {
				if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "Secret value: "); err != nil {
					return err
				}
				data, err := term.ReadPassword(int(os.Stdin.Fd()))
				if _, printErr := fmt.Fprintln(cmd.ErrOrStderr()); printErr != nil && err == nil {
					return printErr
				}
				if err != nil {
					return fmt.Errorf("read secret value: %w", err)
				}
				value = string(data)
			}
			store, err := NewStore()
			if err != nil {
				return err
			}
			if err := store.Set(opts.project, resolvedEnv, args[0], value); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Secret %s set for %s/%s\n", args[0], opts.project, resolvedEnv)
			return err
		},
	}
}

func newGetCommand(opts *commandOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get KEY",
		Short: "Get a secret value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reveal, _ := cmd.Flags().GetBool("reveal")
			if !reveal {
				return fmt.Errorf("refusing to print secret without --reveal")
			}
			resolvedEnv, err := resolveCLIEnv(opts.project, opts.env)
			if err != nil {
				return err
			}
			store, err := NewStore()
			if err != nil {
				return err
			}
			value, err := store.Get(opts.project, resolvedEnv, args[0])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), value)
			return err
		},
	}
	cmd.Flags().Bool("reveal", false, "Print the secret value")
	return cmd
}

func newListCommand(opts *commandOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List secret keys",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			reveal, _ := cmd.Flags().GetBool("reveal")
			resolvedEnv, err := resolveCLIEnv(opts.project, opts.env)
			if err != nil {
				return err
			}
			store, err := NewStore()
			if err != nil {
				return err
			}
			if reveal {
				values, err := store.Load(opts.project, resolvedEnv)
				if err != nil {
					return err
				}
				return printEnvValues(cmd, values)
			}
			keys, err := store.List(opts.project, resolvedEnv)
			if err != nil {
				return err
			}
			return printLines(cmd, keys)
		},
	}
	cmd.Flags().Bool("reveal", false, "Print secret values")
	return cmd
}

func newExportCommand(opts *commandOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export secrets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			reveal, _ := cmd.Flags().GetBool("reveal")
			if !reveal {
				return fmt.Errorf("refusing to export secrets without --reveal")
			}
			format, _ := cmd.Flags().GetString("format")
			resolvedEnv, err := resolveCLIEnv(opts.project, opts.env)
			if err != nil {
				return err
			}
			store, err := NewStore()
			if err != nil {
				return err
			}
			values, err := store.Load(opts.project, resolvedEnv)
			if err != nil {
				return err
			}
			switch format {
			case "env":
				return printEnvValues(cmd, values)
			case "json":
				encoded, err := json.MarshalIndent(values, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
				return err
			default:
				return fmt.Errorf("unsupported export format %q (use env or json)", format)
			}
		},
	}
	cmd.Flags().Bool("reveal", false, "Print secret values")
	cmd.Flags().String("format", "env", "Export format: env or json")
	return cmd
}

func newRemoveCommand(opts *commandOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "remove KEY",
		Short: "Remove a secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedEnv, err := resolveCLIEnv(opts.project, opts.env)
			if err != nil {
				return err
			}
			store, err := NewStore()
			if err != nil {
				return err
			}
			if err := store.Remove(opts.project, resolvedEnv, args[0]); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Secret %s removed from %s/%s\n", args[0], opts.project, resolvedEnv)
			return err
		},
	}
}

func sortedSecretKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func printEnvValues(cmd *cobra.Command, values map[string]string) error {
	for _, key := range sortedSecretKeys(values) {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", key, shellQuote(values[key])); err != nil {
			return err
		}
	}
	return nil
}

func printLines(cmd *cobra.Command, lines []string) error {
	for _, line := range lines {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), line); err != nil {
			return err
		}
	}
	return nil
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
