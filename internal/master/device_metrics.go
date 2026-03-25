package master

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// DeviceMetrics holds a snapshot of system-level metrics.
type DeviceMetrics struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
	MemoryUsedMB  int64   `json:"memory_used_mb"`
	MemoryTotalMB int64   `json:"memory_total_mb"`
	DiskPercent   float64 `json:"disk_percent"`
	DiskUsedGB    float64 `json:"disk_used_gb"`
	DiskTotalGB   float64 `json:"disk_total_gb"`
	Load1m        float64 `json:"load_1m"`
	Load5m        float64 `json:"load_5m"`
	Load15m       float64 `json:"load_15m"`
	TempCelsius   float64 `json:"temperature_celsius,omitempty"`
	UptimeSeconds int64   `json:"uptime_seconds"`
}

// DeviceInfo holds static device identification.
type DeviceInfo struct {
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
	Arch     string `json:"arch"`
	OS       string `json:"os"`
	Kernel   string `json:"kernel,omitempty"`
}

// CollectDeviceMetrics reads all system metrics in one pass.
// On non-Linux systems, /proc reads return zero values gracefully.
func CollectDeviceMetrics() DeviceMetrics {
	m := DeviceMetrics{}

	load1m, load5m, load15m, _ := readLoadAvg()
	m.Load1m = load1m
	m.Load5m = load5m
	m.Load15m = load15m
	m.CPUPercent = cpuPercentFromLoad(load1m)

	usedMB, totalMB, memPct, _ := readMemInfo()
	m.MemoryUsedMB = usedMB
	m.MemoryTotalMB = totalMB
	m.MemoryPercent = memPct

	usedGB, totalGB, diskPct, _ := readDiskUsage()
	m.DiskUsedGB = usedGB
	m.DiskTotalGB = totalGB
	m.DiskPercent = diskPct

	m.UptimeSeconds, _ = readUptime()

	temp := readTemperature()
	if temp > 0 {
		m.TempCelsius = temp
	}

	return m
}

// CollectDeviceInfo reads static device identity.
func CollectDeviceInfo() DeviceInfo {
	h, _ := os.Hostname()
	if h == "" {
		h = "unknown"
	}
	return DeviceInfo{
		Hostname: h,
		IP:       detectOutboundIP(),
		Arch:     runtime.GOARCH,
		OS:       readOSName(),
		Kernel:   readKernelVersion(),
	}
}

// readLoadAvg parses /proc/loadavg, returns (1m, 5m, 15m, error).
func readLoadAvg() (float64, float64, float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0, fmt.Errorf("unexpected /proc/loadavg format")
	}
	v1, e1 := strconv.ParseFloat(fields[0], 64)
	v5, e2 := strconv.ParseFloat(fields[1], 64)
	v15, e3 := strconv.ParseFloat(fields[2], 64)
	if e1 != nil || e2 != nil || e3 != nil {
		return 0, 0, 0, fmt.Errorf("parse loadavg")
	}
	return v1, v5, v15, nil
}

// readMemInfo parses /proc/meminfo, returns (usedMB, totalMB, percentUsed, error).
func readMemInfo() (int64, int64, float64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0, err
	}
	defer f.Close()

	vals := make(map[string]int64)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		v, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		vals[key] = v // kB
	}

	totalKB := vals["MemTotal"]
	availKB, hasAvail := vals["MemAvailable"]
	if !hasAvail {
		availKB = vals["MemFree"]
	}

	if totalKB == 0 {
		return 0, 0, 0, fmt.Errorf("MemTotal not found")
	}

	usedKB := totalKB - availKB
	usedMB := usedKB / 1024
	totalMB := totalKB / 1024
	pct := float64(usedKB) / float64(totalKB) * 100
	return usedMB, totalMB, pct, nil
}

// readDiskUsage calls syscall.Statfs on "/", returns (usedGB, totalGB, percent, error).
func readDiskUsage() (float64, float64, float64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return 0, 0, 0, err
	}
	totalBytes := float64(stat.Blocks) * float64(stat.Bsize)
	freeBytes := float64(stat.Bfree) * float64(stat.Bsize)
	usedBytes := totalBytes - freeBytes
	if totalBytes == 0 {
		return 0, 0, 0, nil
	}
	const gb = 1 << 30
	pct := usedBytes / totalBytes * 100
	return usedBytes / gb, totalBytes / gb, pct, nil
}

// readUptime parses /proc/uptime first field in seconds.
func readUptime() (int64, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, fmt.Errorf("unexpected /proc/uptime format")
	}
	f, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, err
	}
	return int64(f), nil
}

// readTemperature reads /sys/class/thermal/thermal_zone0/temp.
// Returns 0 on any error (graceful macOS skip). Raw value is millidegrees.
func readTemperature() float64 {
	data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0
	}
	return float64(v) / 1000.0
}

// readOSName returns the OS pretty name from /etc/os-release, falls back to runtime.GOOS.
func readOSName() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return runtime.GOOS
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			val := strings.TrimPrefix(line, "PRETTY_NAME=")
			val = strings.Trim(val, `"`)
			return val
		}
	}
	return runtime.GOOS
}

// readKernelVersion reads /proc/version and extracts the kernel version string.
func readKernelVersion() string {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return ""
	}
	// Format: "Linux version 6.1.0-rpi7-rpi-v8 (..."
	fields := strings.Fields(string(data))
	if len(fields) >= 3 {
		return fields[2]
	}
	return ""
}

// detectOutboundIP returns the local IP used for outbound connections.
func detectOutboundIP() string {
	c, err := net.DialTimeout("udp", "8.8.8.8:53", 2*time.Second)
	if err != nil {
		return "unknown"
	}
	defer c.Close()
	addr, ok := c.LocalAddr().(*net.UDPAddr)
	if !ok || addr == nil {
		return "unknown"
	}
	return addr.IP.String()
}

// cpuPercentFromLoad converts 1m load average to approximate CPU percent.
// Uses (load / numCPU) * 100, capped at 100.
func cpuPercentFromLoad(load1m float64) float64 {
	numCPU := runtime.NumCPU()
	if numCPU <= 0 {
		numCPU = 1
	}
	pct := (load1m / float64(numCPU)) * 100
	if pct > 100 {
		pct = 100
	}
	return pct
}
