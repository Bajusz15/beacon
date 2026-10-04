package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"beacon/internal/identity"

	"github.com/spf13/cobra"
)

func createStorageCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "storage",
		Short: "Manage read-only file shares",
		Long: `Manage read-only local filesystem shares exposed by the Beacon storage gateway.

Mount SMB, NFS, WebDAV, or SFTP storage with your operating system, then add the mounted
directory as a Beacon storage share. Beacon only serves explicitly configured shares.`,
	}

	addCmd := &cobra.Command{
		Use:   "add <id> --root <path>",
		Short: "Add a read-only storage share",
		Example: `  beacon storage add media --root /mnt/nas/media
  beacon storage add docs --root /srv/documents --name Documents`,
		Args: cobra.ExactArgs(1),
		Run:  runStorageAdd,
	}
	addCmd.Flags().String("root", "", "Local directory to expose as a read-only share")
	addCmd.Flags().String("name", "", "Display name (defaults to the share id)")
	_ = addCmd.MarkFlagRequired("root")

	removeCmd := &cobra.Command{
		Use:     "remove <id>",
		Short:   "Remove a storage share",
		Aliases: []string{"rm"},
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := identity.RemoveStorageShare(args[0]); err != nil {
				logger.Fatalf("beacon storage remove: %v", err)
			}
			fmt.Printf("Removed storage share %q\n", args[0])
		},
	}

	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "List configured storage shares",
		Aliases: []string{"ls"},
		Run:     runStorageList,
	}

	enableCmd := &cobra.Command{
		Use:   "enable <id>",
		Short: "Enable a storage share",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := identity.SetStorageShareEnabled(args[0], true); err != nil {
				logger.Fatalf("beacon storage enable: %v", err)
			}
			fmt.Printf("Enabled storage share %q\n", args[0])
		},
	}

	disableCmd := &cobra.Command{
		Use:   "disable <id>",
		Short: "Disable a storage share",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := identity.SetStorageShareEnabled(args[0], false); err != nil {
				logger.Fatalf("beacon storage disable: %v", err)
			}
			fmt.Printf("Disabled storage share %q\n", args[0])
		},
	}

	root.AddCommand(addCmd, removeCmd, listCmd, enableCmd, disableCmd)
	return root
}

func runStorageAdd(cmd *cobra.Command, args []string) {
	id := args[0]
	rootPath, _ := cmd.Flags().GetString("root")
	name, _ := cmd.Flags().GetString("name")
	if strings.TrimSpace(name) == "" {
		name = id
	}
	if err := identity.UpsertStorageShare(id, name, rootPath); err != nil {
		logger.Fatalf("beacon storage add: %v", err)
	}
	fmt.Printf("Added read-only storage share %q -> %s\n", id, rootPath)
}

func runStorageList(cmd *cobra.Command, args []string) {
	cfg, err := identity.LoadUserConfig()
	if err != nil {
		logger.Fatalf("beacon storage list: %v", err)
	}
	if cfg == nil || cfg.Storage == nil || len(cfg.Storage.Shares) == 0 {
		fmt.Println("No storage shares configured.")
		fmt.Println("Add one with: beacon storage add <id> --root <path>")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tROOT\tENABLED\tREAD_ONLY")
	for _, share := range cfg.Storage.Shares {
		enabled := "true"
		if share.Enabled != nil && !*share.Enabled {
			enabled = "false"
		}
		readOnly := "true"
		if share.ReadOnly != nil && !*share.ReadOnly {
			readOnly = "false"
		}
		name := strings.TrimSpace(share.Name)
		if name == "" {
			name = share.ID
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", share.ID, name, share.Root, enabled, readOnly)
	}
	_ = w.Flush()
}
