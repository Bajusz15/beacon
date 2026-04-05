package server

import (
	"beacon/internal/config"
	"beacon/internal/logging"
	"beacon/internal/state"

	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

var logger = logging.New("server")

type statusResponse struct {
	LastTag      string `json:"last_tag"`
	LastDeployed string `json:"last_deployed"`
}

func StartHTTPServer(cfg *config.Config, status *state.Status) {
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		tag, deployed := status.Get()
		resp := statusResponse{
			LastTag:      tag,
			LastDeployed: deployed.Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(resp)
		if err != nil {
			logger.Infof("Failed to encode status response: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Failed to encode status response"))
			return
		}
	})

	logger.Infof("Status server listening on :%s\n", cfg.Port)
	http.ListenAndServe(fmt.Sprintf(":%s", cfg.Port), nil)
}
