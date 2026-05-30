package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"beacon/internal/audit"

	"github.com/spf13/cobra"
)

func createAuditCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "audit",
		Short: "Inspect the local audit trail of remote and consequential actions",
		Long: `Beacon records consequential actions — especially those triggered remotely by
BeaconInfra (remote terminal, tunnels, deploys, credential and VPN changes) — to
a tamper-evident, append-only log at ~/.beacon/logs/audit.jsonl.

Each record is chained with the SHA-256 hash of the previous record, so any
insertion, deletion, or edit of past entries is detectable with:

  beacon audit verify

With no subcommand, the most recent entries are shown.`,
		RunE: runAuditList,
	}
	root.Flags().Int("limit", 20, "Maximum number of entries to show")
	root.Flags().Bool("json", false, "Output raw JSON entries")
	root.Flags().String("action", "", "Filter by action (e.g. terminal_open, tunnel_connect)")
	root.Flags().String("status", "", "Filter by status (received, executed, failed, denied)")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List recent audit entries",
		RunE:  runAuditList,
	}
	listCmd.Flags().Int("limit", 20, "Maximum number of entries to show")
	listCmd.Flags().Bool("json", false, "Output raw JSON entries")
	listCmd.Flags().String("action", "", "Filter by action")
	listCmd.Flags().String("status", "", "Filter by status")

	showCmd := &cobra.Command{
		Use:   "show <command-id|seq>",
		Short: "Show full details for entries matching a command id or sequence number",
		Args:  cobra.ExactArgs(1),
		RunE:  runAuditShow,
	}

	tailCmd := &cobra.Command{
		Use:   "tail",
		Short: "Print recent entries and optionally follow new ones",
		RunE:  runAuditTail,
	}
	tailCmd.Flags().Int("limit", 10, "Number of existing entries to show before following")
	tailCmd.Flags().BoolP("follow", "f", false, "Keep running and print new entries as they arrive")

	verifyCmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify the integrity of the audit log hash chain",
		RunE:  runAuditVerify,
	}

	root.AddCommand(listCmd, showCmd, tailCmd, verifyCmd)
	return root
}

func runAuditList(cmd *cobra.Command, _ []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	asJSON, _ := cmd.Flags().GetBool("json")
	actionFilter, _ := cmd.Flags().GetString("action")
	statusFilter, _ := cmd.Flags().GetString("status")

	entries, err := audit.ReadAll()
	if err != nil {
		return fmt.Errorf("reading audit log: %w", err)
	}
	entries = filterEntries(entries, actionFilter, statusFilter)

	if len(entries) == 0 {
		path, _ := audit.Path()
		fmt.Printf("No audit entries yet (%s).\n", path)
		return nil
	}

	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}

	if asJSON {
		return printJSON(entries)
	}
	printTable(entries)
	return nil
}

func runAuditShow(_ *cobra.Command, args []string) error {
	target := strings.TrimSpace(args[0])
	entries, err := audit.ReadAll()
	if err != nil {
		return fmt.Errorf("reading audit log: %w", err)
	}

	var matched []audit.Entry
	seq, seqErr := strconv.ParseInt(target, 10, 64)
	for _, e := range entries {
		if e.CommandID == target || (seqErr == nil && e.Seq == seq) {
			matched = append(matched, e)
		}
	}
	if len(matched) == 0 {
		return fmt.Errorf("no audit entries found for %q", target)
	}
	return printJSON(matched)
}

func runAuditVerify(_ *cobra.Command, _ []string) error {
	res, err := audit.Verify()
	if err != nil && res.Count == 0 && !res.OK {
		return fmt.Errorf("audit verify failed: %w", err)
	}
	if res.OK {
		fmt.Printf("OK — audit chain intact (%d entries verified).\n", res.Count)
		return nil
	}
	fmt.Printf("TAMPERED — chain broke at seq %d after %d valid entries.\n", res.BrokenSeq, res.Count)
	fmt.Printf("Reason: %s\n", res.Reason)
	return fmt.Errorf("audit verification failed")
}

func runAuditTail(cmd *cobra.Command, _ []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	follow, _ := cmd.Flags().GetBool("follow")

	entries, err := audit.ReadAll()
	if err != nil {
		return fmt.Errorf("reading audit log: %w", err)
	}

	var lastSeq int64
	if len(entries) > 0 {
		lastSeq = entries[len(entries)-1].Seq
	}
	shown := entries
	if limit > 0 && len(shown) > limit {
		shown = shown[len(shown)-limit:]
	}
	printTable(shown)

	if !follow {
		return nil
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigCh:
			return nil
		case <-ticker.C:
			latest, readErr := audit.ReadAll()
			if readErr != nil {
				continue
			}
			var fresh []audit.Entry
			for _, e := range latest {
				if e.Seq > lastSeq {
					fresh = append(fresh, e)
				}
			}
			if len(fresh) > 0 {
				printRows(os.Stdout, fresh)
				lastSeq = latest[len(latest)-1].Seq
			}
		}
	}
}

func filterEntries(entries []audit.Entry, action, status string) []audit.Entry {
	action = strings.TrimSpace(action)
	status = strings.TrimSpace(status)
	if action == "" && status == "" {
		return entries
	}
	out := make([]audit.Entry, 0, len(entries))
	for _, e := range entries {
		if action != "" && e.Action != action {
			continue
		}
		if status != "" && e.Status != status {
			continue
		}
		out = append(out, e)
	}
	return out
}

func printTable(entries []audit.Entry) {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "TIME\tACTION\tSTATUS\tSOURCE\tDEVICE\tDETAIL")
	printRows(w, entries)
	_ = w.Flush()
}

// printRows writes entries as tab-separated rows.
func printRows(w io.Writer, entries []audit.Entry) {
	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			formatAuditTime(e.TS),
			dash(e.Action),
			dash(e.Status),
			dash(e.Source),
			dash(e.Device),
			truncate(detailOrProject(e), 60),
		)
	}
}

func detailOrProject(e audit.Entry) string {
	if strings.TrimSpace(e.Detail) != "" {
		return e.Detail
	}
	if strings.TrimSpace(e.Project) != "" {
		return "project=" + e.Project
	}
	return ""
}

func printJSON(entries []audit.Entry) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

func formatAuditTime(ts string) string {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return ts
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
