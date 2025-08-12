package main

import (
	"beacon/internal/config"
	"beacon/internal/deploy"
	"beacon/internal/server"
	"beacon/internal/state"
	"os"
	"path/filepath"

	"context"
	"log"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log.Println("[Beacon] Agent starting...")

	// Set up graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()

	// Create data directory for persistence
	statusStorage := filepath.Join(os.Getenv("HOME"), ".beacon", cfg.ProjectDir)
	status := state.NewStatus(statusStorage)

	// Start HTTP status/metrics endpoint
	go server.StartHTTPServer(cfg, status)

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	// Main polling loop
	for {
		select {
		case <-ctx.Done():
			log.Println("[Beacon] Shutdown signal received, stopping...")
			return
		case <-ticker.C:
			deploy.CheckForNewTag(cfg, status)
		}
	}
}
