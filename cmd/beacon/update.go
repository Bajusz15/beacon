package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"beacon/internal/update"
	"beacon/internal/version"

	"github.com/spf13/cobra"
)

func createUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update beacon to the latest release",
		Long: `Fetch the latest Beacon release from GitHub, verify its SHA256 checksum,
and replace the current binary. Works on Linux and macOS.

By default, downloads and installs the update. Use --check to only check
if an update is available without installing it.`,
		Example: `  beacon update          # download and install latest
  beacon update --check  # just check, don't install`,
		Run: func(cmd *cobra.Command, args []string) {
			checkOnly, _ := cmd.Flags().GetBool("check")
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			fmt.Printf("Current version: %s (%s/%s)\n", version.GetVersion(), runtime.GOOS, runtime.GOARCH)
			fmt.Println("Checking for updates...")

			info, err := update.CheckLatest(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if !info.IsNewer {
				fmt.Printf("Already up to date (%s)\n", info.CurrentVer)
				return
			}

			fmt.Printf("New version available: %s (current: %s)\n", info.Tag, info.CurrentVer)
			fmt.Printf("Asset: %s\n", info.AssetName)

			if checkOnly {
				fmt.Println("\nRun `beacon update` to install.")
				return
			}

			fmt.Println("Downloading...")
			if err := update.DownloadAndReplace(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("Updated to %s\n", info.Tag)
			fmt.Println("Restart beacon start to use the new version.")
		},
	}

	cmd.Flags().Bool("check", false, "Only check for updates, don't install")
	return cmd
}
