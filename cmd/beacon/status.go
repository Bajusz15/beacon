package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"beacon/internal/master"

	"github.com/spf13/cobra"
)

// ANSI color constants (24-bit RGB, exact BeaconInfra palette).
const (
	colorAmber    = "\033[38;2;245;158;11m"
	colorTeal     = "\033[38;2;6;182;212m"
	colorRed      = "\033[38;2;239;68;68m"
	colorWhite    = "\033[38;2;255;255;255m"
	colorBody     = "\033[38;2;203;213;225m"
	colorMuted    = "\033[38;2;148;163;184m"
	colorSubtle   = "\033[38;2;100;116;139m"
	colorBarFull  = "\033[38;2;22;45;80m"
	colorBarEmpty = "\033[38;2;15;31;58m"
	colorLink     = "\033[38;2;13;148;136m"
	colorReset    = "\033[0m"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show master and project health",
	Long: `Connects to the running beacon master's local HTTP server (default port 9100)
and renders a colored terminal status report.

If the master is not running, prints a helpful error and exits non-zero.`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().Int("port", 9100, "Master metrics port")
	statusCmd.Flags().Bool("no-color", false, "Disable ANSI color output")
	statusCmd.Flags().Bool("json", false, "Output raw JSON")
	statusCmd.Flags().Bool("watch", false, "Refresh every 5 seconds")
}

func runStatus(cmd *cobra.Command, args []string) error {
	port, _ := cmd.Flags().GetInt("port")
	noColor, _ := cmd.Flags().GetBool("no-color")
	asJSON, _ := cmd.Flags().GetBool("json")
	watch, _ := cmd.Flags().GetBool("watch")

	if os.Getenv("NO_COLOR") != "" {
		noColor = true
	}

	snap, err := fetchStatus(port)
	if err != nil {
		return err
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(snap)
	}

	renderStatus(snap, noColor, port)

	if !watch {
		return nil
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		snap, err = fetchStatus(port)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\033[2J\033[H") // clear screen
			fmt.Fprintf(os.Stderr, "%sbeacon master unreachable%s\n", colorRed, colorReset)
			continue
		}
		fmt.Print("\033[2J\033[H") // clear screen, move cursor home
		renderStatus(snap, noColor, port)
	}
	return nil
}

// fetchStatus calls GET http://127.0.0.1:{port}/api/status with a 3s timeout.
func fetchStatus(port int) (*master.StatusSnapshot, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/api/status", port)
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") ||
			strings.Contains(err.Error(), "connect: connection refused") ||
			strings.Contains(err.Error(), "No connection could be made") {
			return nil, fmt.Errorf(
				"beacon master is not running (nothing on port %d)\n\n"+
					"  Start it with:  beacon master\n"+
					"  Or as a service: systemctl --user start beacon-master.service", port)
		}
		return nil, fmt.Errorf("connecting to master on port %d: %w", port, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("master returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var snap master.StatusSnapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return &snap, nil
}

// c returns the ANSI code if color is enabled, empty string otherwise.
func c(noColor bool, code string) string {
	if noColor {
		return ""
	}
	return code
}

// thresholdColor returns the appropriate color code based on a percentage threshold.
func thresholdColor(pct float64, noColor bool) string {
	if noColor {
		return ""
	}
	if pct >= 85 {
		return colorRed
	}
	if pct >= 60 {
		return colorAmber
	}
	return colorTeal
}

// statusDot returns the dot character and color for a child status.
func statusDot(status string, noColor bool) string {
	switch status {
	case "healthy":
		return c(noColor, colorTeal) + "●" + c(noColor, colorReset)
	case "degraded", "warning":
		return c(noColor, colorAmber) + "◐" + c(noColor, colorReset)
	case "down":
		return c(noColor, colorRed) + "✕" + c(noColor, colorReset)
	default:
		return c(noColor, colorSubtle) + "○" + c(noColor, colorReset)
	}
}

// renderBar renders a progress bar using block characters.
func renderBar(pct float64, width int, noColor bool) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(math.Round(pct / 100.0 * float64(width)))
	if filled > width {
		filled = width
	}
	col := thresholdColor(pct, noColor)
	var sb strings.Builder
	sb.WriteString(c(noColor, col))
	for i := 0; i < filled; i++ {
		sb.WriteRune('█')
	}
	sb.WriteString(c(noColor, colorReset))
	sb.WriteString(c(noColor, colorBarEmpty))
	for i := filled; i < width; i++ {
		sb.WriteRune('░')
	}
	sb.WriteString(c(noColor, colorReset))
	return sb.String()
}

// formatUptime formats seconds into a human-readable uptime string.
func formatUptime(s int64) string {
	d := s / 86400
	h := (s % 86400) / 3600
	m := (s % 3600) / 60
	sec := s % 60
	if d > 0 {
		return fmt.Sprintf("%dd %dh", d, h)
	}
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%ds", sec)
}

// formatRelTime formats a time as a relative string like "2h ago".
func formatRelTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	diff := time.Since(t)
	if diff < time.Minute {
		return "just now"
	}
	if diff < time.Hour {
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	}
	if diff < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
}

// renderStatus writes the ANSI-colored status output to stdout.
func renderStatus(snap *master.StatusSnapshot, noColor bool, port int) {
	renderStatusHeader(snap, noColor)
	renderStatusDevice(&snap.Device, noColor)
	renderStatusSystem(&snap.System, noColor)
	renderStatusProjects(snap.Children, noColor)
	renderStatusTunnels(snap.Tunnels, noColor)
	renderStatusEvents(snap.Events, noColor)
	renderStatusFooter(port, noColor)
}

func renderStatusHeader(snap *master.StatusSnapshot, noColor bool) {
	ver := snap.Version
	if ver == "" {
		ver = "—"
	}
	fmt.Printf("%s⬡ beacon%s %s%s%s  %s●%s master running  %spid %d  uptime %s%s\n",
		c(noColor, colorAmber), c(noColor, colorReset),
		c(noColor, colorSubtle), ver, c(noColor, colorReset),
		c(noColor, colorTeal), c(noColor, colorReset),
		c(noColor, colorSubtle), snap.Master.PID,
		formatUptime(snap.Master.UptimeSeconds),
		c(noColor, colorReset),
	)
	fmt.Println()
}

func renderStatusDevice(dev *master.DeviceInfo, noColor bool) {
	fmt.Printf("%sDEVICE%s  %s%s%s  %s%s  %s  %s%s\n",
		c(noColor, colorMuted), c(noColor, colorReset),
		c(noColor, colorWhite), dev.Hostname, c(noColor, colorReset),
		c(noColor, colorSubtle), dev.IP, dev.Arch, dev.OS, c(noColor, colorReset),
	)
	fmt.Println()
}

func renderStatusSystem(sys *master.DeviceMetrics, noColor bool) {
	const barWidth = 16
	cpuBar := renderBar(sys.CPUPercent, barWidth, noColor)
	memBar := renderBar(sys.MemoryPercent, barWidth, noColor)
	diskBar := renderBar(sys.DiskPercent, barWidth, noColor)

	fmt.Printf("%sSYSTEM%s  %scpu%s %s%.0f%%%s %s  %smem%s %s%.0f%%%s %s  %sdisk%s %s%.0f%%%s %s\n",
		c(noColor, colorMuted), c(noColor, colorReset),
		c(noColor, colorSubtle), c(noColor, colorReset),
		thresholdColor(sys.CPUPercent, noColor), sys.CPUPercent, c(noColor, colorReset), cpuBar,
		c(noColor, colorSubtle), c(noColor, colorReset),
		thresholdColor(sys.MemoryPercent, noColor), sys.MemoryPercent, c(noColor, colorReset), memBar,
		c(noColor, colorSubtle), c(noColor, colorReset),
		thresholdColor(sys.DiskPercent, noColor), sys.DiskPercent, c(noColor, colorReset), diskBar,
	)

	tempStr := ""
	if sys.TempCelsius > 0 {
		tempStr = fmt.Sprintf("  %stemp%s %s%.1f°C%s",
			c(noColor, colorSubtle), c(noColor, colorReset),
			c(noColor, colorBody), sys.TempCelsius, c(noColor, colorReset))
	}
	fmt.Printf("        %sload%s %s%.2f %.2f %.2f%s%s\n",
		c(noColor, colorSubtle), c(noColor, colorReset),
		c(noColor, colorBody), sys.Load1m, sys.Load5m, sys.Load15m, c(noColor, colorReset),
		tempStr,
	)
	fmt.Println()
}

func renderStatusProjects(children []master.ChildStatus, noColor bool) {
	healthy, warn, down := 0, 0, 0
	for _, ch := range children {
		switch ch.Status {
		case "healthy":
			healthy++
		case "down":
			down++
		default:
			if ch.Status != "unknown" {
				warn++
			}
		}
	}

	fmt.Printf("%sPROJECTS%s  %s%d healthy%s  %s%d warning%s  %s%d down%s\n",
		c(noColor, colorMuted), c(noColor, colorReset),
		c(noColor, colorTeal), healthy, c(noColor, colorReset),
		c(noColor, colorAmber), warn, c(noColor, colorReset),
		c(noColor, colorRed), down, c(noColor, colorReset),
	)
	fmt.Println()

	for _, ch := range children {
		renderStatusProjectRow(ch, noColor)
	}

	if len(children) > 0 {
		fmt.Println()
	}
}

func renderStatusProjectRow(ch master.ChildStatus, noColor bool) {
	dot := statusDot(ch.Status, noColor)
	chkColor := colorTeal
	if ch.Checks.Failing > 0 {
		chkColor = colorAmber
	}
	nameColor := colorWhite
	switch ch.Status {
	case "down":
		nameColor = colorRed
	case "degraded", "warning":
		nameColor = colorAmber
	}

	deployed := ""
	if ch.DeployedAt != nil {
		deployed = fmt.Sprintf("deployed %-10s", formatRelTime(*ch.DeployedAt))
	} else {
		deployed = fmt.Sprintf("%-21s", "")
	}

	fmt.Printf("  %s %s%-20s%s %s%-8s%s %s%s%s%s%d/%d checks passing%s\n",
		dot,
		c(noColor, nameColor), ch.Name, c(noColor, colorReset),
		c(noColor, colorSubtle), ch.Version, c(noColor, colorReset),
		c(noColor, colorSubtle), deployed, c(noColor, colorReset),
		c(noColor, chkColor), ch.Checks.Passing, ch.Checks.Total, c(noColor, colorReset),
	)

	if ch.Checks.Failing == 0 {
		return
	}
	for _, detail := range ch.Checks.Details {
		if detail.Status != "failing" {
			continue
		}
		errMsg := detail.Error
		if errMsg == "" {
			errMsg = "check failed"
		}
		fmt.Printf("    %s└─ ⚠ %s  %s%s\n",
			c(noColor, colorAmber),
			detail.Name,
			errMsg,
			c(noColor, colorReset),
		)
	}
}

func renderStatusTunnels(tunnels []master.TunnelStatusInfo, noColor bool) {
	if len(tunnels) == 0 {
		return
	}
	fmt.Printf("%sTUNNELS%s\n", c(noColor, colorMuted), c(noColor, colorReset))
	fmt.Println()
	for _, t := range tunnels {
		dot := statusDot("unknown", noColor)
		nameColor := colorWhite
		switch t.Status {
		case "connected":
			dot = c(noColor, colorTeal) + "●" + c(noColor, colorReset)
		case "reconnecting":
			dot = c(noColor, colorAmber) + "◐" + c(noColor, colorReset)
			nameColor = colorAmber
		case "failed":
			dot = c(noColor, colorRed) + "✕" + c(noColor, colorReset)
			nameColor = colorRed
		}
		host := t.UpstreamHost
		if host == "" {
			host = "127.0.0.1"
		}
		proto := t.UpstreamProtocol
		if proto == "" {
			proto = "http"
		}
		target := fmt.Sprintf("%s://%s:%d", proto, host, t.LocalPort)
		fmt.Printf("  %s %s%-20s%s %s%s%s  %s%s%s\n",
			dot,
			c(noColor, nameColor), t.ID, c(noColor, colorReset),
			c(noColor, colorSubtle), target, c(noColor, colorReset),
			c(noColor, colorBody), t.Status, c(noColor, colorReset),
		)
	}
	fmt.Println()
}

func renderStatusEvents(events []master.Event, noColor bool) {
	if len(events) == 0 {
		return
	}
	fmt.Printf("%sRECENT%s  %slast 24h%s\n",
		c(noColor, colorMuted), c(noColor, colorReset),
		c(noColor, colorSubtle), c(noColor, colorReset),
	)
	limit := len(events)
	if limit > 10 {
		limit = 10
	}
	for i := len(events) - 1; i >= len(events)-limit; i-- {
		e := events[i]
		t := e.Timestamp
		timeStr := fmt.Sprintf("%02d:%02d", t.Local().Hour(), t.Local().Minute())

		typeColor := colorTeal
		if e.Type == "alert" || e.Type == "restart" {
			typeColor = colorAmber
		}

		sourcePart := ""
		if e.Child != "" {
			sourcePart = e.Child + " "
		}

		durPart := ""
		if e.DurationMs > 0 {
			durPart = fmt.Sprintf(" %s(%ds)%s", c(noColor, colorSubtle), e.DurationMs/1000, c(noColor, colorReset))
		}

		fmt.Printf("  %s%s%s  %s%-10s%s %s%s%s%s\n",
			c(noColor, colorSubtle), timeStr, c(noColor, colorReset),
			c(noColor, typeColor), string(e.Type), c(noColor, colorReset),
			c(noColor, colorBody), sourcePart+e.Message, c(noColor, colorReset),
			durPart,
		)
	}
	fmt.Println()
}

func renderStatusFooter(port int, noColor bool) {
	fmt.Printf("%smetrics%s %shttp://localhost:%d%s  %sprometheus%s %shttp://localhost:%d/metrics%s\n",
		c(noColor, colorSubtle), c(noColor, colorReset),
		c(noColor, colorLink), port, c(noColor, colorReset),
		c(noColor, colorSubtle), c(noColor, colorReset),
		c(noColor, colorLink), port, c(noColor, colorReset),
	)
}
