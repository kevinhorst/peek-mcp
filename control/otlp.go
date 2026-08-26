package control

import (
	"io"
	"net/http"
	"strings"
)

const maxOtlpBodyBytes = 4 << 20

func (s *Server) handleOtlpMetrics(w http.ResponseWriter, r *http.Request) {
	if s.telemetry == nil {
		http.Error(w, "telemetry disabled", http.StatusNotFound)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "only application/json (OTLP http/json) is supported", http.StatusUnsupportedMediaType)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxOtlpBodyBytes))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}

	if err := s.telemetry.IngestMetrics(body); err != nil {
		http.Error(w, "invalid OTLP payload", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{}"))
}

func (s *Server) handleOtlpLogs(w http.ResponseWriter, r *http.Request) {
	if s.telemetry == nil {
		http.Error(w, "telemetry disabled", http.StatusNotFound)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "only application/json (OTLP http/json) is supported", http.StatusUnsupportedMediaType)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxOtlpBodyBytes))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}

	if err := s.telemetry.IngestLogs(body); err != nil {
		http.Error(w, "invalid OTLP payload", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{}"))
}
