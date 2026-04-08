package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/perunio/perunio-facturador/internal/config"
)

// server is the HTTP server for the facturador service.
type server struct {
	mux *chi.Mux
	cfg config.Config
	log *slog.Logger
}

// NewServer creates a new HTTP handler with routes configured.
func NewServer(cfg config.Config, log *slog.Logger) http.Handler {
	s := &server{
		mux: chi.NewRouter(),
		cfg: cfg,
		log: log,
	}

	s.mux.Use(middleware.RequestID)
	s.mux.Use(middleware.RealIP)
	s.mux.Use(middleware.Recoverer)
	s.mux.Use(middleware.Timeout(120 * time.Second))
	s.mux.Use(s.requestLogger)

	s.mux.Get("/health", s.healthHandler)

	s.mux.Route("/api/v1", func(r chi.Router) {
		r.Use(s.apiKeyAuth)

		r.Post("/documents/issue", s.issueDocumentHandler)
		r.Post("/documents/validate", s.validateDocumentHandler)
		r.Post("/documents/cdr", s.queryCDRHandler)

		r.Post("/summaries/issue", s.issueSummaryHandler)
		r.Post("/summaries/status", s.summaryStatusHandler)

		r.Post("/voids/issue", s.issueVoidHandler)
		r.Post("/voids/status", s.voidStatusHandler)

		r.Post("/certificates/validate", s.validateCertificateHandler)
	})

	return s.mux
}

func (s *server) healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
