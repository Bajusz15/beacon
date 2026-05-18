package util

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"beacon/internal/logging"
)

type errorResponseWriter struct {
	header http.Header
}

func (w *errorResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *errorResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func (w *errorResponseWriter) WriteHeader(int) {}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()

	WriteJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q, want application/json", got)
	}
	if got := strings.TrimSpace(w.Body.String()); got != `{"status":"queued"}` {
		t.Fatalf("body = %q", got)
	}
}

func TestWriteJSONLogsWriteError(t *testing.T) {
	var buf bytes.Buffer
	logging.SetOutput(&buf)
	defer logging.SetOutput(nil)

	WriteJSON(&errorResponseWriter{}, http.StatusOK, map[string]string{"status": "queued"})

	if got := buf.String(); !strings.Contains(got, "failed to write json response") {
		t.Fatalf("expected write error log, got %q", got)
	}
}
