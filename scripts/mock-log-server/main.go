// mock-log-server is a minimal HTTP server for e2e tests. It accepts POST /agent/logs,
// records the request body to stdout (so we can verify via kubectl logs), and returns 200.
package main

import (
	"io"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	http.HandleFunc("/agent/logs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("ERROR: read body: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Log so e2e can kubectl logs and grep for expected content
		log.Printf("RECEIVED_POST_AGENT_LOGS: %s", string(body))
		w.WriteHeader(http.StatusOK)
	})
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	log.Printf("mock-log-server listening on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
