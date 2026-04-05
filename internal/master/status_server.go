package master

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

//go:embed dashboard.html
var dashboardHTML []byte

const defaultListenAddr = "127.0.0.1"

// StatusServer is an HTTP server that serves the local metrics dashboard.
type StatusServer struct {
	cache      *StatusCache
	port       int
	listenAddr string
}

// NewStatusServer creates a StatusServer on the given port, bound to 127.0.0.1.
func NewStatusServer(cache *StatusCache, port int) *StatusServer {
	return &StatusServer{cache: cache, port: port, listenAddr: defaultListenAddr}
}

// NewStatusServerWithAddr creates a StatusServer bound to a custom address (e.g. "0.0.0.0" for Docker).
func NewStatusServerWithAddr(cache *StatusCache, port int, listenAddr string) *StatusServer {
	return &StatusServer{cache: cache, port: port, listenAddr: listenAddr}
}

// Start binds the listener and begins serving. Blocks until ctx is canceled.
func (s *StatusServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", s.handleAPIStatus)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/", s.handleDashboard)

	addr := fmt.Sprintf("%s:%d", s.listenAddr, s.port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("status server bind %s: %w", addr, err)
	}

	logger.Infof("Dashboard: http://%s:%d  API: http://%s:%d/api/status", s.listenAddr, s.port, s.listenAddr, s.port)

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *StatusServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(dashboardHTML)
}

func (s *StatusServer) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	snap := s.cache.Get()
	data, err := json.Marshal(snap)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *StatusServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	snap := s.cache.Get()
	sys := snap.System

	healthy, degraded, down := 0, 0, 0
	for _, c := range snap.Children {
		switch c.Status {
		case "healthy":
			healthy++
		case "degraded", "warning":
			degraded++
		case "down":
			down++
		}
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	var b strings.Builder
	metric := func(name, help, typ string, val float64) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s %s\n%s %.6g\n", name, help, name, typ, name, val)
	}

	metric("beacon_cpu_percent", "CPU usage percent", "gauge", sys.CPUPercent)
	metric("beacon_memory_percent", "Memory usage percent", "gauge", sys.MemoryPercent)
	metric("beacon_disk_percent", "Disk usage percent", "gauge", sys.DiskPercent)
	metric("beacon_load_1m", "1-minute load average", "gauge", sys.Load1m)
	metric("beacon_load_5m", "5-minute load average", "gauge", sys.Load5m)
	metric("beacon_load_15m", "15-minute load average", "gauge", sys.Load15m)
	metric("beacon_master_uptime_seconds", "Master process uptime in seconds", "counter", float64(snap.Master.UptimeSeconds))
	metric("beacon_projects_total", "Total number of managed projects", "gauge", float64(len(snap.Children)))
	metric("beacon_projects_healthy", "Number of healthy projects", "gauge", float64(healthy))
	metric("beacon_projects_degraded", "Number of degraded projects", "gauge", float64(degraded))
	metric("beacon_projects_down", "Number of down projects", "gauge", float64(down))
	if sys.TempCelsius > 0 {
		metric("beacon_temperature_celsius", "CPU temperature in Celsius", "gauge", sys.TempCelsius)
	}

	_, _ = fmt.Fprint(w, b.String())
}

func (s *StatusServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
