package http

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/perunio/perunio-facturador/internal/db"
	"github.com/perunio/perunio-facturador/internal/model"
)

// listSummariesHandler returns every daily summary for a company.
func (s *server) listSummariesHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")

	summaries, err := s.pool.ListDailySummaries(r.Context(), companyID)
	if err != nil {
		s.log.Error("list summaries", "error", err, "companyId", companyID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if summaries == nil {
		summaries = []model.DailySummary{}
	}
	writeSuccess(w, summaries)
}

// summaryDetailResponse is the get-by-id shape: summary fields plus the
// linked documents.
type summaryDetailResponse struct {
	model.DailySummary
	Items []db.SummaryItemRow `json:"items"`
}

// getSummaryHandler returns one daily summary with its linked documents.
func (s *server) getSummaryHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	summaryID := chi.URLParam(r, "summaryId")

	summary, err := s.pool.GetDailySummary(r.Context(), companyID, summaryID)
	if err != nil {
		s.log.Error("get summary", "error", err, "summaryId", summaryID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if summary == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Resumen no encontrado")
		return
	}

	items, err := s.pool.GetDailySummaryItems(r.Context(), summaryID)
	if err != nil {
		s.log.Error("get summary items", "error", err, "summaryId", summaryID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if items == nil {
		items = []db.SummaryItemRow{}
	}

	writeSuccess(w, summaryDetailResponse{DailySummary: *summary, Items: items})
}

type createSummaryBody struct {
	ReferenceDate string `json:"referenceDate"`
	SummaryID     string `json:"summaryId"`
}

// createSummaryHandler groups every un-summarized accepted boleta for a date
// into a new daily summary row.
func (s *server) createSummaryHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")

	var body createSummaryBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Datos inválidos")
		return
	}
	if !dateRegex.MatchString(body.ReferenceDate) {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Formato: YYYY-MM-DD")
		return
	}
	if body.SummaryID == "" || len(body.SummaryID) > 30 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "summaryId requerido (1-30 chars)")
		return
	}

	summary, err := s.pool.CreateDailySummary(r.Context(), companyID, body.ReferenceDate, body.SummaryID)
	if err != nil {
		if errors.Is(err, db.ErrNoBoletas) {
			writeError(w, http.StatusBadRequest, "NO_BOLETAS",
				"No hay boletas aceptadas sin resumir para esta fecha")
			return
		}
		s.log.Error("create summary", "error", err, "companyId", companyID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccessStatus(w, http.StatusCreated, summary)
}
