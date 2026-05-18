package util

import (
	"encoding/json"
	"net/http"
)

// WriteJSON writes a JSON response and logs encoding or write failures.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger.Errorf("failed to write json response: %v", err)
	}
}
