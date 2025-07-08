package main

import (
	"beacon/internal/config"
	"beacon/internal/deploy"
	"beacon/internal/server"
	"beacon/internal/state"
	"log"
	"time"
)

func main() {
	log.Println("[Beacon] Agent starting...")

	cfg := config.Load()
	status := &state.Status{}

	// Start HTTP status/metrics endpoint
	go server.StartHTTPServer(cfg, status)

	// Main polling loop
	for {
		deploy.CheckForNewTag(cfg, status)
		time.Sleep(cfg.PollInterval)
	}
}
