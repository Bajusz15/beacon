package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
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
The tunnel connects automatically when the master agent starts.

Use --host (and optional --protocol) to forward to a LAN or Docker hostname instead of loopback
(e.g. Home Assistant OS add-on: --host homeassistant --port 8123).`,
		Example: `  beacon tunnel add homeassistant --port 8123
  beacon tunnel add homeassistant --port 8123 --host homeassistant
  beacon tunnel add jellyfin --port 8096 --host 192.168.1.50`,
		Args: cobra.ExactArgs(1),
		Run:  runTunnelAdd,
	}
	addCmd.Flags().IntP("port", "p", 0, "TCP port on the upstream host (required)")
	addCmd.Flags().String("host", "", "Upstream hostname or IP (omit for 127.0.0.1-only; use e.g. homeassistant on Home Assistant OS)")
	addCmd.Flags().String("protocol", "", "http or https (default http; use with --host or for HTTPS to loopback)")
	_ = addCmd.MarkFlagRequired("port")

	removeCmd := &cobra.Command{
		Use:     "remove <id>",
		Short:   "Remove a tunnel from config",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"rm"},
		Run: func(cmd *cobra.Command, args []string) {
			if err := identity.RemoveTunnel(args[0]); err != nil {
				logger.Fatalf("beacon tunnel remove: %v", err)
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
				logger.Fatalf("beacon tunnel enable: %v", err)
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
				logger.Fatalf("beacon tunnel disable: %v", err)
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
	host, _ := cmd.Flags().GetString("host")
	protocol, _ := cmd.Flags().GetString("protocol")

	var err error
	if strings.TrimSpace(host) != "" || strings.TrimSpace(protocol) != "" {
		err = identity.UpsertTunnelUpstream(id, protocol, host, port)
	} else {
		err = identity.AppendTunnelIfMissing(id, port)
	}
	if err != nil {
		logger.Fatalf("beacon tunnel add: %v", err)
	}

	cfg, _ := identity.LoadUserConfig()
	var tc *identity.TunnelConfig
	if cfg != nil {
		for i := range cfg.Tunnels {
			if cfg.Tunnels[i].ID == id {
				tc = &cfg.Tunnels[i]
				break
			}
		}
	}
	if tc != nil {
		proto, h, p, euErr := tc.EffectiveUpstream()
		if euErr == nil {
			fmt.Printf("Added tunnel %q -> %s://%s:%d\n", id, proto, h, p)
		} else {
			fmt.Printf("Added tunnel %q\n", id)
		}
	} else {
		fmt.Printf("Added tunnel %q\n", id)
	}

	if cfg != nil {
		enabled := 0
		for _, t := range cfg.Tunnels {
			if t.Enabled == nil || *t.Enabled {
				enabled++
			}
		}
		if enabled > 2 {
			fmt.Printf("Warning: only %d tunnels can be active at once. The rest will stay dormant.\n", 2)
			fmt.Println("Use 'beacon tunnel disable <id>' to choose which tunnels are dormant.")
		}
	}
}

func runTunnelList(cmd *cobra.Command, args []string) {
	cfg, err := identity.LoadUserConfig()
	if err != nil {
		logger.Fatalf("beacon tunnel list: %v", err)
	}
	if cfg == nil || len(cfg.Tunnels) == 0 {
		fmt.Println("No tunnels configured.")
		fmt.Println("Add one with: beacon tunnel add <id> --port <port>")
		return
	}

	liveStatus := fetchTunnelStatus()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tUPSTREAM\tENABLED\tSTATUS")
	for _, t := range cfg.Tunnels {
		enabled := "true"
		if t.Enabled != nil && !*t.Enabled {
			enabled = "false"
		}
		status := "-"
		if s, ok := liveStatus[t.ID]; ok {
			status = s
		}
		proto, host, port, euErr := t.EffectiveUpstream()
		up := "—"
		if euErr == nil {
			up = fmt.Sprintf("%s://%s:%d", proto, host, port)
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.ID, up, enabled, status)
	}
	_ = w.Flush()
}

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
