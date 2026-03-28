package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"beacon/internal/identity"

	"github.com/spf13/cobra"
)

func createTunnelCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "tunnel",
		Short: "Manage reverse tunnels to local services",
		Long: `Manage reverse tunnels that expose local services (e.g., Home Assistant)
through BeaconInfra cloud without opening ports on your device.

Tunnels are active when the master agent is running (beacon master).`,
	}

	addCmd := &cobra.Command{
		Use:   "add <id>",
		Short: "Add a tunnel to config",
		Long: `Add a new tunnel entry to ~/.beacon/config.yaml.
The cloud WebSocket opens only when you connect from BeaconInfra (tunnel_connect), not when the master starts.`,
		Example: `  beacon tunnel add homeassistant --port 8123
  beacon tunnel add grafana --port 3000`,
		Args: cobra.ExactArgs(1),
		Run:  runTunnelAdd,
	}
	addCmd.Flags().IntP("port", "p", 0, "Local port to tunnel (required)")
	_ = addCmd.MarkFlagRequired("port")

	removeCmd := &cobra.Command{
		Use:     "remove <id>",
		Short:   "Remove a tunnel from config",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"rm"},
		Run: func(cmd *cobra.Command, args []string) {
			if err := identity.RemoveTunnel(args[0]); err != nil {
				log.Fatalf("beacon tunnel remove: %v", err)
			}
			fmt.Printf("Removed tunnel %q\n", args[0])
		},
	}

	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "List configured tunnels",
		Aliases: []string{"ls"},
		Run:     runTunnelList,
	}

	enableCmd := &cobra.Command{
		Use:   "enable <id>",
		Short: "Enable a tunnel",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := identity.SetTunnelEnabled(args[0], true); err != nil {
				log.Fatalf("beacon tunnel enable: %v", err)
			}
			fmt.Printf("Enabled tunnel %q\n", args[0])
		},
	}

	disableCmd := &cobra.Command{
		Use:   "disable <id>",
		Short: "Disable a tunnel",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := identity.SetTunnelEnabled(args[0], false); err != nil {
				log.Fatalf("beacon tunnel disable: %v", err)
			}
			fmt.Printf("Disabled tunnel %q\n", args[0])
		},
	}

	root.AddCommand(addCmd, removeCmd, listCmd, enableCmd, disableCmd)
	return root
}

func runTunnelAdd(cmd *cobra.Command, args []string) {
	id := args[0]
	port, _ := cmd.Flags().GetInt("port")

	if err := identity.AppendTunnelIfMissing(id, port); err != nil {
		log.Fatalf("beacon tunnel add: %v", err)
	}
	fmt.Printf("Added tunnel %q -> localhost:%d\n", id, port)
}

func runTunnelList(cmd *cobra.Command, args []string) {
	cfg, err := identity.LoadUserConfig()
	if err != nil {
		log.Fatalf("beacon tunnel list: %v", err)
	}
	if cfg == nil || len(cfg.Tunnels) == 0 {
		fmt.Println("No tunnels configured.")
		fmt.Println("Add one with: beacon tunnel add <id> --port <port>")
		return
	}

	// Try to get live status from master
	liveStatus := fetchTunnelStatus()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tPORT\tENABLED\tSTATUS")
	for _, t := range cfg.Tunnels {
		enabled := "true"
		if t.Enabled != nil && !*t.Enabled {
			enabled = "false"
		}
		status := "-"
		if s, ok := liveStatus[t.ID]; ok {
			status = s
		}
		_, _ = fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", t.ID, t.LocalPort, enabled, status)
	}
	_ = w.Flush()
}

// fetchTunnelStatus tries to get tunnel status from the running master's /api/status endpoint.
func fetchTunnelStatus() map[string]string {
	result := make(map[string]string)

	cfg, _ := identity.LoadUserConfig()
	port := 9100
	if cfg != nil && cfg.MetricsPort > 0 {
		port = cfg.MetricsPort
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/api/status")
	if err != nil {
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	var snap struct {
		Tunnels []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"tunnels"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return result
	}
	for _, t := range snap.Tunnels {
		result[t.ID] = t.Status
	}
	return result
}
