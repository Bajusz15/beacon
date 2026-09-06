package master

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"

	"beacon/internal/audit"
	"beacon/internal/storage"
	"beacon/internal/util"
)

func (s *StatusServer) handleAPIStorageShares(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/storage/shares" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc := storage.NewService(s.cache.getStorageConfig())
	shares, err := svc.Shares()
	if err != nil {
		writeStorageError(w, err)
		return
	}
	util.WriteJSON(w, http.StatusOK, map[string]any{"shares": shares})
}

func (s *StatusServer) handleAPIStorageShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/storage/shares/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.NotFound(w, r)
		return
	}
	shareID, err := url.PathUnescape(parts[0])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	action := parts[1]
	svc := storage.NewService(s.cache.getStorageConfig())

	switch action {
	case "list":
		if r.Method == http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		resp, err := svc.List(shareID, r.URL.Query().Get("path"))
		if err != nil {
			writeStorageError(w, err)
			return
		}
		audit.Log(audit.Event{Action: "storage_list", Source: "local", Status: "executed", Detail: "share=" + resp.Share.ID})
		util.WriteJSON(w, http.StatusOK, resp)
	case "metadata":
		if r.Method == http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		resp, err := svc.Metadata(shareID, r.URL.Query().Get("path"))
		if err != nil {
			writeStorageError(w, err)
			return
		}
		audit.Log(audit.Event{Action: "storage_metadata", Source: "local", Status: "executed", Detail: "share=" + resp.Share.ID})
		util.WriteJSON(w, http.StatusOK, resp)
	case "download":
		resp, err := svc.Open(shareID, r.URL.Query().Get("path"))
		if err != nil {
			writeStorageError(w, err)
			return
		}
		defer func() { _ = resp.File.Close() }()
		audit.Log(audit.Event{Action: "storage_download", Source: "local", Status: "executed", Detail: "share=" + resp.Share.ID})
		if resp.ContentType != "" {
			w.Header().Set("Content-Type", resp.ContentType)
		}
		w.Header().Set("Accept-Ranges", "bytes")
		http.ServeContent(w, r, resp.Name, resp.ModTime, resp.File)
	default:
		http.NotFound(w, r)
	}
}

func writeStorageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrNotConfigured):
		util.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "storage not configured"})
	case errors.Is(err, storage.ErrShareNotFound):
		util.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "storage share not found"})
	case errors.Is(err, storage.ErrForbidden):
		util.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "storage path forbidden"})
	case errors.Is(err, storage.ErrIsDirectory):
		util.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "storage path is a directory"})
	case errors.Is(err, os.ErrNotExist):
		util.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "storage path not found"})
	default:
		writeAPIError(w, http.StatusInternalServerError, err)
	}
}
