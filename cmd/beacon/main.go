package main

import (
	"beacon/internal/deploy"
	"beacon/internal/server"
	"beacon/internal/state"
	"log"
	"time"
)

const pollInterval = 60 * time.Second

func main() {
	log.Println("[Beacon] Agent starting...")

	status := &state.Status{}

	// Start HTTP status/metrics endpoint
	go server.StartHTTPServer(status)

	// Main polling loop
	for {
		deploy.CheckForNewTag(status)
		time.Sleep(pollInterval)
	}
}
