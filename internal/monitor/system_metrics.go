package monitor

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"beacon/internal/util"
)

// System metric helper functions for Linux IoT devices
// Uses direct /proc file parsing - no external dependencies or shell commands

func getCPUUsage() (float64, error) {
	// For IoT devices, we'll use load average as a proxy for CPU usage
	// This is more reliable than trying to calculate CPU percentage
	loadAvg, err := getLoadAverage()
	if err != nil {
		return 0, err
	}

	// Convert load average to approximate CPU usage percentage
	// Load average represents average number of processes waiting for CPU
	// We'll use the 1-minute load average and scale it based on CPU cores
	numCPU := runtime.NumCPU()
	if numCPU == 0 {
		numCPU = 1 // Fallback to 1 if we can't determine CPU count
	}

	// Scale load average to percentage (load/cpu_cores * 100)
	// Cap at 100% since load average can exceed CPU count
	cpuUsage := (loadAvg / float64(numCPU)) * 100
	if cpuUsage > 100 {
		cpuUsage = 100
	}

	return cpuUsage, nil
}

func getMemoryUsage() (float64, error) {
	// Parse /proc/meminfo for memory usage
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, err
	}

	var memTotal, memFree, memAvailable uint64
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		switch fields[0] {
		case "MemTotal:":
			memTotal, _ = strconv.ParseUint(fields[1], 10, 64)
		case "MemFree:":
			memFree, _ = strconv.ParseUint(fields[1], 10, 64)
		case "MemAvailable:":
			memAvailable, _ = strconv.ParseUint(fields[1], 10, 64)
		}
	}

	if memTotal == 0 {
		return 0, fmt.Errorf("invalid memory data: MemTotal is 0")
	}

	// Use MemAvailable if available (Linux 3.14+), otherwise calculate from MemFree
	var usedMemory uint64
	if memAvailable > 0 {
		usedMemory = memTotal - memAvailable
	} else {
		usedMemory = memTotal - memFree
	}

	// Calculate usage percentage
	usage := float64(usedMemory) / float64(memTotal) * 100
	return usage, nil
}

func getDiskUsage(path string) (float64, error) {
	// Use syscall.Statfs for disk usage - no external dependencies
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return 0, err
	}

	// Calculate usage percentage
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	used := total - free

	if total == 0 {
		return 0, fmt.Errorf("invalid disk data: total blocks is 0")
	}

	usage := float64(used) / float64(total) * 100
	return usage, nil
}

func getLoadAverage() (float64, error) {
	// Parse /proc/loadavg for load average
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, err
	}

	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, fmt.Errorf("invalid load average data: insufficient fields")
	}

	// Parse 1-minute load average (first value)
	loadAvg, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse 1-minute load average: %v", err)
	}

	return loadAvg, nil
}

func getUptime() (int64, error) {
	// Parse /proc/uptime for system uptime
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}

	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, fmt.Errorf("invalid uptime data: insufficient fields")
	}

	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse uptime: %v", err)
	}

	return int64(uptime), nil
}

func getHostname() (string, error) {
	// Get system hostname
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("failed to get hostname: %v", err)
	}
	return hostname, nil
}

func getIPAddress() (string, error) {
	// Get primary IP address by connecting to a dummy address
	conn, err := net.Dial("udp4", "8.8.8.8:80") // <-- "udp4" forces IPv4
	if err != nil {
		return "", fmt.Errorf("failed to get IP address: %v", err)
	}
	defer util.DeferClose(conn, "UDP connection")()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}
