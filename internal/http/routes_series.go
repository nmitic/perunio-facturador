package http

import (
	"errors"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"

	"github.com/perunio/perunio-facturador/internal/db"
	"github.com/perunio/perunio-facturador/internal/model"
)

// listSeriesHandler returns every document_series row for a company.
func (s *server) listSeriesHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")

	series, err := s.pool.ListSeries(r.Context(), companyID)
	if err != nil {
		s.log.Error("list series", "error", err, "companyId", companyID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if series == nil {
		series = []model.Series{}
	}
	writeSuccess(w, series)
}

type createSeriesRequest struct {
	DocType     string  `json:"docType"`
	Series      string  `json:"series"`
	Description *string `json:"description,omitempty"`
}

var seriesCodeRegex = regexp.MustCompile(`^[A-Z0-9]{1,4}$`)

// createSeriesHandler inserts a new document_series row.
func (s *server) createSeriesHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")

	var req createSeriesRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Datos inválidos")
		return
	}
	switch req.DocType {
	case "01", "03", "07", "08":
	default:
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "docType inválido")
		return
	}
	if !seriesCodeRegex.MatchString(req.Series) {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Series must be 1-4 alphanumeric uppercase characters")
		return
	}
	if req.Description != nil && len(*req.Description) > 255 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "description too long")
		return
	}

	created, err := s.pool.CreateSeries(r.Context(), companyID, db.CreateSeriesInput{
		DocType:     req.DocType,
		Series:      req.Series,
		Description: req.Description,
	})
	if err != nil {
		if errors.Is(err, db.ErrDuplicate) {
			writeError(w, http.StatusConflict, "SERIES_DUPLICATE", "Esta serie ya existe para este tipo de documento")
			return
		}
		s.log.Error("create series", "error", err, "companyId", companyID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccessStatus(w, http.StatusCreated, created)
}

type updateSeriesRequest struct {
	Description *string `json:"description,omitempty"`
	IsActive    *bool   `json:"isActive,omitempty"`
}

// updateSeriesHandler patches description / isActive.
func (s *server) updateSeriesHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	seriesID := chi.URLParam(r, "seriesId")

	var req updateSeriesRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Datos inválidos")
		return
	}
	if req.Description != nil && len(*req.Description) > 255 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "description too long")
		return
	}

	updated, err := s.pool.UpdateSeries(r.Context(), companyID, seriesID, db.UpdateSeriesInput{
		Description: req.Description,
		IsActive:    req.IsActive,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Serie no encontrada")
			return
		}
		s.log.Error("update series", "error", err, "companyId", companyID, "seriesId", seriesID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccess(w, updated)
}

// deleteSeriesHandler removes a series row if it has no documents.
func (s *server) deleteSeriesHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	seriesID := chi.URLParam(r, "seriesId")

	if err := s.pool.DeleteSeries(r.Context(), companyID, seriesID); err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Serie no encontrada")
		case errors.Is(err, db.ErrSeriesHasDocuments):
			writeError(w, http.StatusConflict, "SERIES_HAS_DOCUMENTS",
				"No se puede eliminar una serie que tiene documentos asociados")
		default:
			s.log.Error("delete series", "error", err, "companyId", companyID, "seriesId", seriesID)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		}
		return
	}
	writeSuccess(w, map[string]string{"message": "Serie eliminada"})
}
