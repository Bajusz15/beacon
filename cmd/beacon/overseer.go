package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"beacon/internal/overseer"
	"beacon/internal/util"

	"github.com/spf13/cobra"
)

func createProxmoxCommand() *cobra.Command {
	root := &cobra.Command{
		Use:     "proxmox",
		Aliases: []string{"overseer"},
		Short:   "Oversee the VMs and containers this Proxmox host runs",
		Long: `When Beacon runs on a Proxmox VE host it acts as an "overseer" for the host: it sees
every VM and container the host runs — including their up/down state — through the
host's own pvesh CLI. This is the one vantage point a crashed guest cannot report from.

Run on the Proxmox host (as root, or a user with pvesh access). With no subcommand,
the guest inventory is listed. ("overseer" is kept as an alias.)`,
		RunE: runOverseerList,
	}
	root.Flags().Bool("json", false, "Output raw JSON")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List the VMs and containers this host runs",
		RunE:  runOverseerList,
	}
	listCmd.Flags().Bool("json", false, "Output raw JSON")

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show a one-line summary of this host's guests",
		RunE:  runOverseerStatus,
	}

	root.AddCommand(listCmd, statusCmd)
	return root
}

func newOverseerOrExit(cmd *cobra.Command) (*overseer.Overseer, context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
	o := overseer.New()
	if !o.Available(ctx) {
		cancel()
		fmt.Fprintln(os.Stderr, "overseer: pvesh not available — run this on a Proxmox VE host with pvesh in PATH")
		os.Exit(1)
	}
	return o, ctx, cancel
}

func runOverseerList(cmd *cobra.Command, _ []string) error {
	o, ctx, cancel := newOverseerOrExit(cmd)
	defer cancel()

	guests, err := o.ListGuests(ctx)
	if err != nil {
		return err
	}

	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(guests)
	}

	if len(guests) == 0 {
		fmt.Println("No VMs or containers found on this host.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "VMID\tNAME\tTYPE\tSTATUS")
	for _, g := range guests {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", g.VMID, g.Name, g.Type, g.Status)
	}
	util.LogError(w.Flush(), "flush overseer table")
	return nil
}

func runOverseerStatus(cmd *cobra.Command, _ []string) error {
	o, ctx, cancel := newOverseerOrExit(cmd)
	defer cancel()

	guests, err := o.ListGuests(ctx)
	if err != nil {
		return err
	}
	running := 0
	for _, g := range guests {
		if g.Running() {
			running++
		}
	}
	fmt.Printf("%d/%d guests running on this host.\n", running, len(guests))
	return nil
}
