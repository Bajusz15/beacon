package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"beacon/internal/identity"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func createCloudCommand() *cobra.Command {
	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Save BeaconInfra API credentials (interactive or --api-key)",
		Long: `Writes your API key to ~/.beacon/config.yaml and enables cloud heartbeats.
The API base URL is baked into this binary at compile time (see "beacon config show").

Get an API key at https://beaconinfra.dev (Settings → API Keys).

Non-interactive: beacon cloud login --api-key usr_...
Or: BEACON_API_KEY=usr_... beacon cloud login`,
		Run: runCloudLogin,
	}
	loginCmd.Flags().String("api-key", "", "User API key (non-interactive); else BEACON_API_KEY")
	loginCmd.Flags().String("name", "", "Device name (default: hostname)")
	loginCmd.Flags().String("device-name", "", "Alias for --name")

	logoutCmd := &cobra.Command{
		Use:   "logout",
		Short: "Clear API key from config and set cloud_reporting_enabled to false",
		Run: func(cmd *cobra.Command, args []string) {
			if err := identity.WriteCloudLogout(); err != nil {
				logger.Fatalf("beacon cloud logout: %v", err)
			}
			fmt.Println()
			fmt.Println("  ✓ Logged out — API key removed, cloud reporting disabled.")
			fmt.Println()
			fmt.Println("  Beacon will continue running locally. To re-authenticate:")
			fmt.Println()
			fmt.Println("    beacon cloud login --api-key YOUR_KEY")
			fmt.Println()
		},
	}

	root := &cobra.Command{
		Use:   "cloud",
		Short: "BeaconInfra cloud credentials",
	}
	root.AddCommand(loginCmd, logoutCmd)
	return root
}

func runCloudLogin(cmd *cobra.Command, args []string) {
	apiKey, _ := cmd.Flags().GetString("api-key")
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("BEACON_API_KEY"))
	}
	if apiKey == "" {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			fmt.Fprint(os.Stderr, "BeaconInfra API key: ")
			b, err := term.ReadPassword(syscall.Stdin)
			if err != nil {
				logger.Fatalf("beacon cloud login: read API key: %v", err)
			}
			apiKey = strings.TrimSpace(string(b))
			fmt.Fprintln(os.Stderr)
		} else {
			logger.Fatalf("beacon cloud login: non-interactive terminal; use --api-key or set BEACON_API_KEY")
		}
	}
	if apiKey == "" {
		logger.Fatalf("beacon cloud login: API key cannot be empty. Get one at https://beaconinfra.dev → API Keys")
	}
	if !strings.HasPrefix(apiKey, "usr_") {
		logger.Fatalf("beacon cloud login: invalid API key format (expected usr_...). Get one at https://beaconinfra.dev → API Keys")
	}

	name, _ := cmd.Flags().GetString("name")
	if name == "" {
		name, _ = cmd.Flags().GetString("device-name")
	}
	if name == "" {
		name = strings.TrimSpace(os.Getenv("BEACON_DEVICE_NAME"))
	}

	if err := identity.WriteCloudLogin(apiKey, name); err != nil {
		logger.Fatalf("beacon cloud login: %v", err)
	}

	cfg, _ := identity.LoadUserConfig()
	deviceName := ""
	if cfg != nil {
		deviceName = cfg.DeviceName
	}

	fmt.Println()
	fmt.Println("  ✓ Authenticated successfully")
	if deviceName != "" {
		fmt.Printf("  ✓ Device name: %s\n", deviceName)
	}
	fmt.Println()
	fmt.Println("  Next step — start Beacon:")
	fmt.Println()
	fmt.Println("    beacon start")
	fmt.Println()
	fmt.Println("  Your device will appear automatically in BeaconInfra")
	fmt.Println("  after the first heartbeat (~30 seconds).")
	fmt.Println()
}
