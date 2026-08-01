package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/perunio/perunio-facturador/internal/db"
)

// parseReportFilter reads the shared date-range + currency query params. Missing
// dates default to the current month; missing currency defaults to PEN. Returns
// a non-empty message when a param is malformed (caller replies 400).
func parseReportFilter(r *http.Request) (db.ReportFilter, string) {
	q := r.URL.Query()
	from := q.Get("from")
	to := q.Get("to")
	now := time.Now().UTC()
	if from == "" {
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
	if to == "" {
		to = now.Format("2006-01-02")
	}
	if !dateRegex.MatchString(from) {
		return db.ReportFilter{}, "from inválido (YYYY-MM-DD)"
	}
	if !dateRegex.MatchString(to) {
		return db.ReportFilter{}, "to inválido (YYYY-MM-DD)"
	}
	currency := q.Get("currency")
	if currency == "" {
		currency = "PEN"
	}
	if len(currency) != 3 {
		return db.ReportFilter{}, "currency inválido"
	}
	return db.ReportFilter{From: from, To: to, Currency: currency}, ""
}

// reportError logs and replies 500 for a failed report query.
func (s *Server) reportError(w http.ResponseWriter, name string, err error) {
	s.log.Error("report query", "report", name, "error", err)
	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
}

func (s *Server) salesSummaryHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	filter, msg := parseReportFilter(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", msg)
		return
	}
	res, err := s.pool.GetSalesSummary(r.Context(), companyID, filter)
	if err != nil {
		s.reportError(w, "sales/summary", err)
		return
	}
	writeSuccess(w, res)
}

func (s *Server) salesSeriesHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	filter, msg := parseReportFilter(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", msg)
		return
	}
	bucket := r.URL.Query().Get("bucket")
	switch bucket {
	case "day", "month", "year":
	case "":
		bucket = "month"
	default:
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "bucket debe ser day|month|year")
		return
	}
	res, err := s.pool.GetSalesSeries(r.Context(), companyID, bucket, filter)
	if err != nil {
		s.reportError(w, "sales/series", err)
		return
	}
	writeSuccess(w, res)
}

func (s *Server) customerRankingHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	filter, msg := parseReportFilter(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", msg)
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	res, err := s.pool.GetCustomerRanking(r.Context(), companyID, q.Get("orderBy"), limit, filter)
	if err != nil {
		s.reportError(w, "customers", err)
		return
	}
	writeSuccess(w, res)
}

func (s *Server) productRankingHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	filter, msg := parseReportFilter(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", msg)
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	res, err := s.pool.GetProductRanking(r.Context(), companyID, q.Get("orderBy"), limit, filter)
	if err != nil {
		s.reportError(w, "products", err)
		return
	}
	writeSuccess(w, res)
}

func (s *Server) taxBreakdownHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	filter, msg := parseReportFilter(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", msg)
		return
	}
	res, err := s.pool.GetTaxBreakdown(r.Context(), companyID, filter)
	if err != nil {
		s.reportError(w, "tax/breakdown", err)
		return
	}
	writeSuccess(w, res)
}

func (s *Server) taxNotesHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	filter, msg := parseReportFilter(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", msg)
		return
	}
	docType := r.URL.Query().Get("docType")
	if docType != "07" && docType != "08" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "docType debe ser 07|08")
		return
	}
	res, err := s.pool.GetNotesBreakdown(r.Context(), companyID, docType, filter)
	if err != nil {
		s.reportError(w, "tax/notes", err)
		return
	}
	writeSuccess(w, res)
}

func (s *Server) installmentsReportHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	filter, msg := parseReportFilter(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", msg)
		return
	}
	res, err := s.pool.GetInstallmentsReport(r.Context(), companyID, r.URL.Query().Get("status"), filter)
	if err != nil {
		s.reportError(w, "installments", err)
		return
	}
	writeSuccess(w, res)
}

func (s *Server) anticiposReportHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	filter, msg := parseReportFilter(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", msg)
		return
	}
	q := r.URL.Query()
	status := q.Get("status")
	switch status {
	case "", "pendiente", "aplicado":
	default:
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "status debe ser pendiente|aplicado")
		return
	}
	res, err := s.pool.GetAnticiposReport(r.Context(), companyID, status, q.Get("customerDoc"), filter)
	if err != nil {
		s.reportError(w, "anticipos", err)
		return
	}
	writeSuccess(w, res)
}

func (s *Server) sunatSubmissionsHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	filter, msg := parseReportFilter(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", msg)
		return
	}
	res, err := s.pool.GetSunatSubmissions(r.Context(), companyID, filter)
	if err != nil {
		s.reportError(w, "sunat/submissions", err)
		return
	}
	writeSuccess(w, res)
}
