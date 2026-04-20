// Command mock-webhook is used by tests/e2e/test-cli.sh to receive POST bodies (alert webhook e2e).
package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
)

func main() {
	port := 18080
	if p := os.Getenv("MOCK_WEBHOOK_PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}
	outPath := os.Getenv("MOCK_WEBHOOK_OUT")
	if outPath == "" {
		outPath = "/tmp/beacon-e2e-webhook.log"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read", http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(outPath, body, 0o644); err != nil {
			log.Printf("write %s: %v", outPath, err)
			http.Error(w, "write", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	addr := ":" + strconv.Itoa(port)
	log.Printf("mock-webhook listening on %s -> %s", addr, outPath)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}
