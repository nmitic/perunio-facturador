package http

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/perunio/perunio-facturador/internal/db"
	"github.com/perunio/perunio-facturador/internal/model"
)

// listPaymentsHandler returns every recorded installment payment for a document.
func (s *Server) listPaymentsHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	docID := chi.URLParam(r, "docId")

	payments, err := s.pool.ListInstallmentPayments(r.Context(), companyID, docID)
	if err != nil {
		s.log.Error("list installment payments", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if payments == nil {
		payments = []model.InstallmentPayment{}
	}
	writeSuccess(w, payments)
}

// createPaymentBody is the record-a-payment wire contract.
type createPaymentBody struct {
	CuotaNumero int     `json:"cuotaNumero"`
	Amount      string  `json:"amount"`
	PaidAt      string  `json:"paidAt"`
	Method      *string `json:"method,omitempty"`
	Reference   *string `json:"reference,omitempty"`
	Notes       *string `json:"notes,omitempty"`
}

func (b *createPaymentBody) validate() string {
	if b.CuotaNumero < 1 {
		return "cuotaNumero inválido"
	}
	if !decimalRegex.MatchString(b.Amount) {
		return "amount inválido"
	}
	if !dateRegex.MatchString(b.PaidAt) {
		return "paidAt inválido (YYYY-MM-DD)"
	}
	return ""
}

// createPaymentHandler records one payment against a document's cuota.
func (s *Server) createPaymentHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	docID := chi.URLParam(r, "docId")

	var b createPaymentBody
	if err := decodeBody(r, &b); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Cuerpo de solicitud inválido")
		return
	}
	if msg := b.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", msg)
		return
	}

	pmt, err := s.pool.CreateInstallmentPayment(r.Context(), companyID, docID, db.CreateInstallmentPaymentInput{
		CuotaNumero: b.CuotaNumero,
		Amount:      b.Amount,
		PaidAt:      b.PaidAt,
		Method:      b.Method,
		Reference:   b.Reference,
		Notes:       b.Notes,
	})
	if err != nil {
		if errors.Is(err, db.ErrDocumentNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Documento no encontrado")
			return
		}
		s.log.Error("create installment payment", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccessStatus(w, http.StatusCreated, pmt)
}

// deletePaymentHandler removes a recorded payment.
func (s *Server) deletePaymentHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	docID := chi.URLParam(r, "docId")
	paymentID := chi.URLParam(r, "paymentId")

	err := s.pool.DeleteInstallmentPayment(r.Context(), companyID, docID, paymentID)
	if err != nil {
		if errors.Is(err, db.ErrDocumentNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Pago no encontrado")
			return
		}
		s.log.Error("delete installment payment", "error", err, "paymentId", paymentID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccess(w, map[string]string{"message": "Pago eliminado"})
}
