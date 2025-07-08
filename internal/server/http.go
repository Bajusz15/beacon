package server

import (
	"beacon/internal/config"
	"beacon/internal/state"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type statusResponse struct {
	LastTag      string `json:"last_tag"`
	LastDeployed string `json:"last_deployed"`
}

func StartHTTPServer(cfg *config.Config, status *state.Status) {
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		tag, deployed := status.Get()
		resp := statusResponse{
			LastTag:      tag,
			LastDeployed: deployed.Format("2006-01-02T15:04:05Z07:00"),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	log.Printf("[Beacon] Status server listening on :%s\n", cfg.Port)
	http.ListenAndServe(fmt.Sprintf(":%s", cfg.Port), nil)
}
