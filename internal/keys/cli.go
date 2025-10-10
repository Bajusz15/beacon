package keys

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// KeysCmd represents the keys command
var KeysCmd = &cobra.Command{
	Use:   "keys",
	Short: "Manage API keys and tokens",
	Long:  `Manage API keys for monitoring and Git tokens for deployment.`,
}

// ListKeysCmd lists all stored keys
var ListKeysCmd = &cobra.Command{
	Use:   "list",
	Short: "List all stored keys",
	Long:  `List all stored API keys and Git tokens with their metadata.`,
	Run: func(cmd *cobra.Command, args []string) {
		configDir := getConfigDir()
		km, err := NewKeyManager(configDir)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		keys, err := km.ListKeys()
		if err != nil {
			fmt.Printf("Error listing keys: %v\n", err)
			os.Exit(1)
		}

		if len(keys) == 0 {
			fmt.Println("No keys stored.")
			return
		}

		fmt.Printf("%-20s %-15s %-10s %-20s %-10s\n", "NAME", "PROVIDER", "TYPE", "CREATED", "STATUS")
		fmt.Println(strings.Repeat("-", 80))

		for _, key := range keys {
			status := "inactive"
			if key.IsActive {
				status = "active"
			}

			fmt.Printf("%-20s %-15s %-10s %-20s %-10s\n",
				key.Name,
				key.Provider,
				"API_KEY",
				key.CreatedAt.Format("2006-01-02 15:04"),
				status,
			)
		}
	},
}

// AddKeyCmd adds a new API key
var AddKeyCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new API key",
	Long:  `Add a new API key for monitoring or Git token for deployment.`,
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")
		key, _ := cmd.Flags().GetString("key")
		provider, _ := cmd.Flags().GetString("provider")
		description, _ := cmd.Flags().GetString("description")

		if name == "" || key == "" {
			fmt.Println("Error: --name and --key are required")
			os.Exit(1)
		}

		if provider == "" {
			provider = "beaconwatch"
		}

		configDir := getConfigDir()
		km, err := NewKeyManager(configDir)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		err = km.AddKey(name, key, provider, description)
		if err != nil {
			fmt.Printf("Error adding key: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully added key '%s' for provider '%s'\n", name, provider)
	},
}

// RotateKeyCmd rotates an existing API key
var RotateKeyCmd = &cobra.Command{
	Use:   "rotate",
	Short: "Rotate an existing API key",
	Long:  `Replace an existing API key with a new one.`,
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")
		newKey, _ := cmd.Flags().GetString("new-key")

		if name == "" || newKey == "" {
			fmt.Println("Error: --name and --new-key are required")
			os.Exit(1)
		}

		configDir := getConfigDir()
		km, err := NewKeyManager(configDir)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		err = km.RotateKey(name, newKey)
		if err != nil {
			fmt.Printf("Error rotating key: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully rotated key '%s'\n", name)
	},
}

// DeleteKeyCmd deletes an API key
var DeleteKeyCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete an API key",
	Long:  `Delete a stored API key.`,
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")

		if name == "" {
			fmt.Println("Error: --name is required")
			os.Exit(1)
		}

		configDir := getConfigDir()
		km, err := NewKeyManager(configDir)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		err = km.DeleteKey(name)
		if err != nil {
			fmt.Printf("Error deleting key: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully deleted key '%s'\n", name)
	},
}
var ValidateKeyCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate an API key",
	Long:  `Test if an API key is valid by making a test request.`,
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")

		if name == "" {
			fmt.Println("Error: --name is required")
			os.Exit(1)
		}

		configDir := getConfigDir()
		km, err := NewKeyManager(configDir)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		err = km.ValidateKey(name)
		if err != nil {
			fmt.Printf("Key validation failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Key '%s' is valid\n", name)
	},
}

// GitCmd represents the git command
var GitCmd = &cobra.Command{
	Use:   "git",
	Short: "Manage Git tokens",
	Long:  `Manage Git tokens for different providers (GitHub, GitLab, Bitbucket).`,
}

// AddGitTokenCmd adds a new Git token
var AddGitTokenCmd = &cobra.Command{
	Use:   "add-token",
	Short: "Add a new Git token",
	Long:  `Add a new Git token for a specific provider.`,
	Run: func(cmd *cobra.Command, args []string) {
		provider, _ := cmd.Flags().GetString("provider")
		token, _ := cmd.Flags().GetString("token")
		name, _ := cmd.Flags().GetString("name")

		if provider == "" || token == "" {
			fmt.Println("Error: --provider and --token are required")
			os.Exit(1)
		}

		if name == "" {
			name = fmt.Sprintf("%s_token", provider)
		}

		configDir := getConfigDir()
		km, err := NewKeyManager(configDir)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		description := fmt.Sprintf("Git token for %s", provider)
		err = km.AddKey(name, token, provider, description)
		if err != nil {
			fmt.Printf("Error adding Git token: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully added Git token '%s' for provider '%s'\n", name, provider)
	},
}

// ListGitTokensCmd lists all Git tokens
var ListGitTokensCmd = &cobra.Command{
	Use:   "list-tokens",
	Short: "List all Git tokens",
	Long:  `List all stored Git tokens.`,
	Run: func(cmd *cobra.Command, args []string) {
		configDir := getConfigDir()
		km, err := NewKeyManager(configDir)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		keys, err := km.ListKeys()
		if err != nil {
			fmt.Printf("Error listing keys: %v\n", err)
			os.Exit(1)
		}

		gitKeys := filterGitTokens(keys)
		if len(gitKeys) == 0 {
			fmt.Println("No Git tokens stored.")
			return
		}

		fmt.Printf("%-20s %-15s %-20s %-10s\n", "NAME", "PROVIDER", "CREATED", "STATUS")
		fmt.Println(strings.Repeat("-", 70))

		for _, key := range gitKeys {
			status := "inactive"
			if key.IsActive {
				status = "active"
			}

			fmt.Printf("%-20s %-15s %-20s %-10s\n",
				key.Name,
				key.Provider,
				key.CreatedAt.Format("2006-01-02 15:04"),
				status,
			)
		}
	},
}

// RotateGitTokenCmd rotates a Git token
var RotateGitTokenCmd = &cobra.Command{
	Use:   "rotate-token",
	Short: "Rotate a Git token",
	Long:  `Replace an existing Git token with a new one.`,
	Run: func(cmd *cobra.Command, args []string) {
		provider, _ := cmd.Flags().GetString("provider")
		newToken, _ := cmd.Flags().GetString("new-token")
		name, _ := cmd.Flags().GetString("name")

		if provider == "" || newToken == "" {
			fmt.Println("Error: --provider and --new-token are required")
			os.Exit(1)
		}

		if name == "" {
			name = fmt.Sprintf("%s_token", provider)
		}

		configDir := getConfigDir()
		km, err := NewKeyManager(configDir)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		err = km.RotateKey(name, newToken)
		if err != nil {
			fmt.Printf("Error rotating Git token: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully rotated Git token '%s' for provider '%s'\n", name, provider)
	},
}

// TestGitTokenCmd tests a Git token
var TestGitTokenCmd = &cobra.Command{
	Use:   "test-token",
	Short: "Test a Git token",
	Long:  `Test if a Git token is valid by making a test request to the provider.`,
	Run: func(cmd *cobra.Command, args []string) {
		provider, _ := cmd.Flags().GetString("provider")
		name, _ := cmd.Flags().GetString("name")

		if provider == "" {
			fmt.Println("Error: --provider is required")
			os.Exit(1)
		}

		if name == "" {
			name = fmt.Sprintf("%s_token", provider)
		}

		configDir := getConfigDir()
		km, err := NewKeyManager(configDir)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		err = km.ValidateKey(name)
		if err != nil {
			fmt.Printf("Git token validation failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Git token '%s' for provider '%s' is valid\n", name, provider)
	},
}

func init() {
	// Keys command flags
	AddKeyCmd.Flags().String("name", "", "Name for the key")
	AddKeyCmd.Flags().String("key", "", "The API key or token")
	AddKeyCmd.Flags().String("provider", "beaconwatch", "Provider (beaconwatch, github, gitlab, etc.)")
	AddKeyCmd.Flags().String("description", "", "Description for the key")

	RotateKeyCmd.Flags().String("name", "", "Name of the key to rotate")
	RotateKeyCmd.Flags().String("new-key", "", "New API key or token")

	DeleteKeyCmd.Flags().String("name", "", "Name of the key to delete")

	ValidateKeyCmd.Flags().String("name", "", "Name of the key to validate")

	// Git command flags
	AddGitTokenCmd.Flags().String("provider", "", "Git provider (github, gitlab, bitbucket)")
	AddGitTokenCmd.Flags().String("token", "", "Git token")
	AddGitTokenCmd.Flags().String("name", "", "Name for the token (defaults to {provider}_token)")

	RotateGitTokenCmd.Flags().String("provider", "", "Git provider")
	RotateGitTokenCmd.Flags().String("new-token", "", "New Git token")
	RotateGitTokenCmd.Flags().String("name", "", "Name of the token to rotate")

	TestGitTokenCmd.Flags().String("provider", "", "Git provider")
	TestGitTokenCmd.Flags().String("name", "", "Name of the token to test")

	// Add subcommands
	KeysCmd.AddCommand(ListKeysCmd)
	KeysCmd.AddCommand(AddKeyCmd)
	KeysCmd.AddCommand(RotateKeyCmd)
	KeysCmd.AddCommand(DeleteKeyCmd)
	KeysCmd.AddCommand(ValidateKeyCmd)
	KeysCmd.AddCommand(GitCmd)

	GitCmd.AddCommand(AddGitTokenCmd)
	GitCmd.AddCommand(ListGitTokensCmd)
	GitCmd.AddCommand(RotateGitTokenCmd)
	GitCmd.AddCommand(TestGitTokenCmd)
}

// getConfigDir returns the beacon configuration directory
func getConfigDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".beacon"
	}
	return filepath.Join(homeDir, ".beacon")
}

// filterGitTokens filters keys to only include Git tokens
func filterGitTokens(keys []StoredKey) []StoredKey {
	var gitKeys []StoredKey
	gitProviders := map[string]bool{
		"github":    true,
		"gitlab":    true,
		"bitbucket": true,
	}

	for _, key := range keys {
		if gitProviders[key.Provider] {
			gitKeys = append(gitKeys, key)
		}
	}

	return gitKeys
}
