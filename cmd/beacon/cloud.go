package main

import (
	"fmt"
	"log"
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
				log.Fatalf("beacon cloud logout: %v", err)
			}
			p, err := identity.UserConfigPath()
			if err != nil {
				log.Printf("[Beacon] Updated cloud settings")
				return
			}
			log.Printf("[Beacon] Cleared cloud credentials in %s", p)
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
				log.Fatalf("beacon cloud login: read API key: %v", err)
			}
			apiKey = strings.TrimSpace(string(b))
			fmt.Fprintln(os.Stderr)
		} else {
			log.Fatal("beacon cloud login: non-interactive terminal; use --api-key or set BEACON_API_KEY")
		}
	}

	name, _ := cmd.Flags().GetString("name")
	if name == "" {
		name, _ = cmd.Flags().GetString("device-name")
	}
	if name == "" {
		name = strings.TrimSpace(os.Getenv("BEACON_DEVICE_NAME"))
	}

	if err := identity.WriteCloudLogin(apiKey, name); err != nil {
		log.Fatalf("beacon cloud login: %v", err)
	}
	p, err := identity.UserConfigPath()
	if err != nil {
		log.Printf("[Beacon] Wrote ~/.beacon/config.yaml")
		return
	}
	log.Printf("[Beacon] Wrote %s", p)
}
