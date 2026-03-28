package master

import (
	"strings"
	"time"

	"beacon/internal/identity"
)

// heartbeatSystemMetrics matches beaconinfra AgentMetricsRequest JSON for POST /api/agent/heartbeat.
type heartbeatSystemMetrics struct {
	Hostname      string    `json:"hostname"`
	IPAddress     string    `json:"ip_address"`
	CPUUsage      float64   `json:"cpu_usage"`
	MemoryUsage   float64   `json:"memory_usage"`
	DiskUsage     float64   `json:"disk_usage"`
	UptimeSeconds int64     `json:"uptime_seconds"`
	LoadAverage   float64   `json:"load_average"`
	Timestamp     time.Time `json:"timestamp"`
}

func buildSystemMetricsForCloud(cfg *identity.UserConfig, lastSent time.Time) (*heartbeatSystemMetrics, bool) {
	if cfg == nil || cfg.SystemMetrics == nil || !cfg.SystemMetrics.Enabled {
		return nil, false
	}
	u := cfg.SystemMetrics
	interval := u.Interval
	if interval <= 0 {
		interval = time.Minute
	}
	if !lastSent.IsZero() && time.Since(lastSent) < interval {
		return nil, false
	}

	dm := CollectDeviceMetrics()
	diskPath := strings.TrimSpace(u.DiskPath)
	if diskPath == "" {
		diskPath = "/"
	}
	diskPct := dm.DiskPercent
	if diskPath != "/" {
		if p, err := DiskUsagePercentAt(diskPath); err == nil {
			diskPct = p
		}
	}

	out := &heartbeatSystemMetrics{
		Hostname:      getHostname(),
		IPAddress:     getOutboundIP(),
		UptimeSeconds: dm.UptimeSeconds,
		Timestamp:     time.Now(),
	}
	if u.CPU {
		out.CPUUsage = dm.CPUPercent
	}
	if u.Memory {
		out.MemoryUsage = dm.MemoryPercent
	}
	if u.Disk {
		out.DiskUsage = diskPct
	}
	if u.LoadAverage {
		out.LoadAverage = dm.Load1m
	}
	return out, true
}
