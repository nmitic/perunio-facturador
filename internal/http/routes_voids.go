package http

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/perunio/perunio-facturador/internal/db"
	"github.com/perunio/perunio-facturador/internal/model"
)

// listVoidsHandler returns every void communication for a company.
func (s *server) listVoidsHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")

	voids, err := s.pool.ListVoidedDocuments(r.Context(), companyID)
	if err != nil {
		s.log.Error("list voids", "error", err, "companyId", companyID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if voids == nil {
		voids = []model.VoidedDocument{}
	}
	writeSuccess(w, voids)
}

// voidDetailResponse is the get-by-id shape: void fields plus its line items.
type voidDetailResponse struct {
	model.VoidedDocument
	Items []model.VoidedDocumentItem `json:"items"`
}

// getVoidHandler returns one void communication with its line items.
func (s *server) getVoidHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	voidID := chi.URLParam(r, "voidId")

	v, err := s.pool.GetVoidedDocument(r.Context(), companyID, voidID)
	if err != nil {
		s.log.Error("get void", "error", err, "voidId", voidID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if v == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Comunicación de baja no encontrada")
		return
	}

	items, err := s.pool.GetVoidedDocumentItems(r.Context(), voidID)
	if err != nil {
		s.log.Error("get void items", "error", err, "voidId", voidID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if items == nil {
		items = []model.VoidedDocumentItem{}
	}

	writeSuccess(w, voidDetailResponse{VoidedDocument: *v, Items: items})
}

type createVoidBody struct {
	VoidID      string   `json:"voidId"`
	VoidDate    string   `json:"voidDate"`
	DocumentIDs []string `json:"documentIds"`
	Reason      string   `json:"reason"`
}

// createVoidHandler groups existing accepted documents into a void request,
// enforcing the 7-day SUNAT void window in the DB layer.
func (s *server) createVoidHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")

	var body createVoidBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Datos inválidos")
		return
	}
	if body.VoidID == "" || len(body.VoidID) > 30 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "voidId requerido (1-30 chars)")
		return
	}
	if !dateRegex.MatchString(body.VoidDate) {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Formato: YYYY-MM-DD")
		return
	}
	if len(body.DocumentIDs) == 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "documentIds requerido")
		return
	}
	for _, id := range body.DocumentIDs {
		if !docUUIDRegex.MatchString(id) {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "documentId inválido: "+id)
			return
		}
	}
	if body.Reason == "" || len(body.Reason) > 500 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "reason requerido (1-500 chars)")
		return
	}

	voidDoc, err := s.pool.CreateVoidRequest(r.Context(), companyID, body.VoidID,
		body.VoidDate, body.DocumentIDs, body.Reason)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			writeError(w, http.StatusBadRequest, "DOCUMENTS_NOT_FOUND",
				"Uno o más documentos no fueron encontrados")
		case errors.Is(err, db.ErrInvalidDocStatus):
			writeError(w, http.StatusBadRequest, "INVALID_DOCUMENT_STATUS", err.Error())
		case errors.Is(err, db.ErrVoidWindowExpired):
			writeError(w, http.StatusBadRequest, "VOID_WINDOW_EXPIRED", err.Error())
		default:
			s.log.Error("create void", "error", err, "companyId", companyID)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		}
		return
	}
	writeSuccessStatus(w, http.StatusCreated, voidDoc)
}
