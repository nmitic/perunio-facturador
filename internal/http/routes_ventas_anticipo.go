package http

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/perunio/perunio-facturador/internal/db"
	"github.com/perunio/perunio-facturador/internal/model"
)

// listVentasAnticipoHandler returns the company's ventas con anticipos with
// their derived cobrado / saldo / estado.
func (s *Server) listVentasAnticipoHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	q := r.URL.Query()

	estado := q.Get("estado")
	switch estado {
	case "", string(model.VentaAbierta), string(model.VentaRegularizada), string(model.VentaCancelada):
	default:
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "estado inválido")
		return
	}

	ventas, err := s.pool.ListVentasAnticipo(r.Context(), companyID, db.VentaAnticipoFilter{
		Estado:      estado,
		CustomerDoc: q.Get("customerDoc"),
	})
	if err != nil {
		s.log.Error("list ventas anticipo", "error", err, "companyId", companyID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccess(w, ventas)
}

// getVentaAnticipoHandler returns one venta con anticipos.
func (s *Server) getVentaAnticipoHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	ventaID := chi.URLParam(r, "ventaId")

	venta, err := s.pool.GetVentaAnticipo(r.Context(), companyID, ventaID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Venta con anticipos no encontrada")
			return
		}
		s.log.Error("get venta anticipo", "error", err, "companyId", companyID, "ventaId", ventaID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccess(w, venta)
}

// listVentaAnticipoDocumentsHandler returns the comprobantes of one venta: its
// anticipo facturas plus the factura final when it exists.
func (s *Server) listVentaAnticipoDocumentsHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	ventaID := chi.URLParam(r, "ventaId")

	res, err := s.pool.ListIssuedDocuments(r.Context(), companyID, db.DocumentListFilter{
		VentaAnticipoID: ventaID,
		Page:            1,
		// A deal has a handful of comprobantes, not pages of them; one call is enough.
		Limit: 200,
	})
	if err != nil {
		s.log.Error("list venta anticipo documents", "error", err, "companyId", companyID, "ventaId", ventaID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccess(w, res.Documents)
}

type ventaAnticipoRequest struct {
	Nombre            string         `json:"nombre"`
	Descripcion       *string        `json:"descripcion,omitempty"`
	CustomerDocType   string         `json:"customerDocType"`
	CustomerDocNumber string         `json:"customerDocNumber"`
	CustomerName      string         `json:"customerName"`
	CurrencyCode      string         `json:"currencyCode"`
	MontoAcordado     string         `json:"montoAcordado"`
	FormData          map[string]any `json:"formData"`
}

// validate guards the fields the deal is useless without. The monto acordado is
// the whole point of the record — without it there is no saldo to report — so an
// absent or zero amount is rejected rather than defaulted.
func (b ventaAnticipoRequest) validate() string {
	switch {
	case b.Nombre == "":
		return "Ingrese un nombre para la venta"
	case len(b.Nombre) > 160:
		return "El nombre es demasiado largo"
	case b.CustomerDocNumber == "" || b.CustomerName == "":
		return "Seleccione el cliente de la venta"
	case !decimalRegex.MatchString(b.MontoAcordado) || b.MontoAcordado == "0":
		return "El monto acordado debe ser mayor a 0"
	case len(b.FormData) == 0:
		return "La venta necesita los productos que la componen"
	}
	return ""
}

// createVentaAnticipoHandler inserts a new venta con anticipos.
func (s *Server) createVentaAnticipoHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")

	var req ventaAnticipoRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Datos inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", msg)
		return
	}
	currency := req.CurrencyCode
	if currency == "" {
		currency = "PEN"
	}

	created, err := s.pool.CreateVentaAnticipo(r.Context(), companyID, db.CreateVentaAnticipoInput{
		Nombre:            req.Nombre,
		Descripcion:       req.Descripcion,
		CustomerDocType:   req.CustomerDocType,
		CustomerDocNumber: req.CustomerDocNumber,
		CustomerName:      req.CustomerName,
		CurrencyCode:      currency,
		MontoAcordado:     req.MontoAcordado,
		FormData:          req.FormData,
	})
	if err != nil {
		s.log.Error("create venta anticipo", "error", err, "companyId", companyID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccessStatus(w, http.StatusCreated, created)
}

type updateVentaAnticipoRequest struct {
	Nombre        *string        `json:"nombre,omitempty"`
	Descripcion   *string        `json:"descripcion,omitempty"`
	MontoAcordado *string        `json:"montoAcordado,omitempty"`
	FormData      map[string]any `json:"formData,omitempty"`
	Cancelada     *bool          `json:"cancelada,omitempty"`
}

// updateVentaAnticipoHandler patches a venta — including the cancelada switch,
// which is how a deal is retired once comprobantes exist (it can no longer be
// deleted at that point).
func (s *Server) updateVentaAnticipoHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	ventaID := chi.URLParam(r, "ventaId")

	var req updateVentaAnticipoRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Datos inválidos")
		return
	}
	if req.Nombre != nil && (*req.Nombre == "" || len(*req.Nombre) > 160) {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Nombre inválido")
		return
	}
	if req.MontoAcordado != nil && (!decimalRegex.MatchString(*req.MontoAcordado) || *req.MontoAcordado == "0") {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "El monto acordado debe ser mayor a 0")
		return
	}

	updated, err := s.pool.UpdateVentaAnticipo(r.Context(), companyID, ventaID, db.UpdateVentaAnticipoInput{
		Nombre:        req.Nombre,
		Descripcion:   req.Descripcion,
		MontoAcordado: req.MontoAcordado,
		FormData:      req.FormData,
		Cancelada:     req.Cancelada,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Venta con anticipos no encontrada")
			return
		}
		s.log.Error("update venta anticipo", "error", err, "companyId", companyID, "ventaId", ventaID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccess(w, updated)
}

// deleteVentaAnticipoHandler removes a venta that has no comprobantes yet.
func (s *Server) deleteVentaAnticipoHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	ventaID := chi.URLParam(r, "ventaId")

	if err := s.pool.DeleteVentaAnticipo(r.Context(), companyID, ventaID); err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Venta con anticipos no encontrada")
		case errors.Is(err, db.ErrVentaHasDocuments):
			writeError(w, http.StatusConflict, "VENTA_HAS_DOCUMENTS",
				"Esta venta ya tiene comprobantes emitidos. Cancélala en lugar de eliminarla.")
		default:
			s.log.Error("delete venta anticipo", "error", err, "companyId", companyID, "ventaId", ventaID)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
