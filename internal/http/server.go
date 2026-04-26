package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/perunio/perunio-facturador/internal/auth"
	"github.com/perunio/perunio-facturador/internal/awssecrets"
	"github.com/perunio/perunio-facturador/internal/config"
	"github.com/perunio/perunio-facturador/internal/db"
	"github.com/perunio/perunio-facturador/internal/greclient"
	"github.com/perunio/perunio-facturador/internal/r2"
	"github.com/perunio/perunio-facturador/internal/signature"
)

// certCacheTTL is the lifetime of a parsed certificate in the in-process cache.
// Activation rotates the cert ID, so a stale entry never returns the wrong
// cert; the TTL only bounds how long an out-of-band DB edit goes unnoticed.
const certCacheTTL = 10 * time.Minute

// Deps bundles every dependency the HTTP server needs. Constructed in
// cmd/app/main.go and passed to NewServer.
type Deps struct {
	Config  config.Config
	Log     *slog.Logger
	Secrets *awssecrets.Service
	Pool    *db.Pool
	R2      *r2.Client
}

// server is the HTTP server for the facturador service.
type server struct {
	mux       *chi.Mux
	cfg       config.Config
	log       *slog.Logger
	secrets   *awssecrets.Service
	pool      *db.Pool
	r2        *r2.Client
	greClient *greclient.Client
	authMW    *auth.Middleware
	certCache *signature.Cache
}

// NewServer creates a new HTTP handler with routes configured.
func NewServer(deps Deps) http.Handler {
	s := &server{
		mux:     chi.NewRouter(),
		cfg:     deps.Config,
		log:     deps.Log,
		secrets: deps.Secrets,
		pool:    deps.Pool,
		r2:      deps.R2,
		greClient: greclient.NewClient(
			deps.Config.SunatGRESecurityURL,
			deps.Config.SunatGREBetaURL,
			deps.Config.SunatGREProductionURL,
			deps.Config.SunatTimeoutSeconds,
		),
		authMW:    auth.NewMiddleware(deps.Secrets.JWTSecret(), deps.Pool),
		certCache: signature.NewCache(certCacheTTL),
	}

	s.mux.Use(middleware.RequestID)
	s.mux.Use(middleware.RealIP)
	s.mux.Use(middleware.Recoverer)
	s.mux.Use(middleware.Timeout(120 * time.Second))
	s.mux.Use(s.requestLogger)
	s.mux.Use(s.cors(deps.Config.AllowedOrigins))

	s.mux.Get("/health", s.healthHandler)

	// New JWT-authenticated routes — the long-term home for all facturador
	// functionality. Routes are added phase-by-phase as the migration from
	// perunio-backend progresses. JWT, blacklist, and tokenVersion checks all
	// happen inside authMW.Authenticate.
	s.mux.Route("/api/facturador", func(r chi.Router) {
		r.Use(s.authMW.Authenticate)

		r.Get("/usage", s.usageHandler)

		// Certificate management lives in perunio-backend now. The signing
		// pipeline reads the active cert from the DB on its own.

		// Series.
		r.Get("/series/{companyId}", s.listSeriesHandler)
		r.Post("/series/{companyId}", s.createSeriesHandler)
		r.Put("/series/{companyId}/{seriesId}", s.updateSeriesHandler)
		r.Delete("/series/{companyId}/{seriesId}", s.deleteSeriesHandler)

		// Documents.
		r.Get("/documents/{companyId}", s.listDocumentsHandler)
		r.Post("/documents/{companyId}", s.createDocumentHandler)
		r.Get("/documents/{companyId}/{docId}", s.getDocumentHandler)
		r.Put("/documents/{companyId}/{docId}", s.updateDocumentHandler)
		r.Delete("/documents/{companyId}/{docId}", s.deleteDocumentHandler)
		r.Post("/documents/{companyId}/{docId}/issue", s.issueDocumentPipelineHandler)
		r.Get("/documents/{companyId}/{docId}/files/{fileType}", s.documentFileHandler)

		// Summaries.
		r.Get("/summaries/{companyId}", s.listSummariesHandler)
		r.Post("/summaries/{companyId}", s.createSummaryHandler)
		r.Get("/summaries/{companyId}/{summaryId}", s.getSummaryHandler)
		r.Post("/summaries/{companyId}/{summaryId}/issue", s.issueSummaryPipelineHandler)
		r.Post("/summaries/{companyId}/{summaryId}/poll", s.pollSummaryPipelineHandler)

		// Voids.
		r.Get("/voids/{companyId}", s.listVoidsHandler)
		r.Post("/voids/{companyId}", s.createVoidHandler)
		r.Get("/voids/{companyId}/{voidId}", s.getVoidHandler)
		r.Post("/voids/{companyId}/{voidId}/issue", s.issueVoidPipelineHandler)
		r.Post("/voids/{companyId}/{voidId}/poll", s.pollVoidPipelineHandler)

		// Guías de Remisión Electrónica (GRE REST API).
		r.Get("/gre/{companyId}", s.listDespatchesHandler)
		r.Post("/gre/{companyId}", s.createDespatchHandler)
		r.Get("/gre/{companyId}/{despatchId}", s.getDespatchHandler)
		r.Put("/gre/{companyId}/{despatchId}", s.updateDespatchHandler)
		r.Delete("/gre/{companyId}/{despatchId}", s.deleteDespatchHandler)
		r.Post("/gre/{companyId}/{despatchId}/issue", s.issueDespatchHandler)
		r.Post("/gre/{companyId}/{despatchId}/poll", s.pollDespatchHandler)
		r.Get("/gre/{companyId}/{despatchId}/files/{fileType}", s.despatchFileHandler)
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
