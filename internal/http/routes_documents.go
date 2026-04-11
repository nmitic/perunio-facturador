package http

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/perunio/perunio-facturador/internal/auth"
	"github.com/perunio/perunio-facturador/internal/db"
	"github.com/perunio/perunio-facturador/internal/model"
)

// documentListResponse mirrors the Node.js response shape:
// { success: true, data: [...], pagination: { page, limit, total, totalPages } }.
type documentListResponse struct {
	Success    bool                       `json:"success"`
	Data       []model.IssuedDocument     `json:"data"`
	Pagination documentListPagination     `json:"pagination"`
}

type documentListPagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

// listDocumentsHandler returns a paginated, filterable slice of issued
// documents for the company.
func (s *server) listDocumentsHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))

	filter := db.DocumentListFilter{
		DocType:           q.Get("docType"),
		Status:            q.Get("status"),
		CustomerDocNumber: q.Get("customer"),
		Page:              page,
		Limit:             limit,
	}

	result, err := s.pool.ListIssuedDocuments(r.Context(), companyID, filter)
	if err != nil {
		s.log.Error("list documents", "error", err, "companyId", companyID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}

	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 20
	}

	totalPages := 0
	if filter.Limit > 0 {
		totalPages = (result.Total + filter.Limit - 1) / filter.Limit
	}
	if result.Documents == nil {
		result.Documents = []model.IssuedDocument{}
	}

	resp := documentListResponse{
		Success: true,
		Data:    result.Documents,
		Pagination: documentListPagination{
			Page: filter.Page, Limit: filter.Limit,
			Total: result.Total, TotalPages: totalPages,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

// documentDetailResponse is the get-by-id shape: doc fields plus its items.
type documentDetailResponse struct {
	model.IssuedDocument
	Items []model.IssuedDocumentItem `json:"items"`
}

// getDocumentHandler returns one issued document with its line items.
func (s *server) getDocumentHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	docID := chi.URLParam(r, "docId")

	doc, err := s.pool.GetIssuedDocument(r.Context(), companyID, docID)
	if err != nil {
		s.log.Error("get document", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Documento no encontrado")
		return
	}

	items, err := s.pool.GetIssuedDocumentItems(r.Context(), docID)
	if err != nil {
		s.log.Error("get document items", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if items == nil {
		items = []model.IssuedDocumentItem{}
	}

	writeSuccess(w, documentDetailResponse{IssuedDocument: *doc, Items: items})
}

// Zod-equivalent validators.
var (
	decimalRegex   = regexp.MustCompile(`^\d+(\.\d+)?$`)
	dateRegex      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	timeRegex      = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}$`)
	docUUIDRegex   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

type createDocumentItemBody struct {
	LineNumber             *int    `json:"lineNumber,omitempty"`
	Description            string  `json:"description"`
	Quantity               string  `json:"quantity"`
	UnitCode               string  `json:"unitCode"`
	UnitPrice              string  `json:"unitPrice"`
	UnitPriceWithTax       *string `json:"unitPriceWithTax,omitempty"`
	TaxExemptionReasonCode *string `json:"taxExemptionReasonCode,omitempty"`
	IgvAmount              string  `json:"igvAmount"`
	IscAmount              *string `json:"iscAmount,omitempty"`
	DiscountAmount         *string `json:"discountAmount,omitempty"`
	LineTotal              string  `json:"lineTotal"`
	PriceTypeCode          *string `json:"priceTypeCode,omitempty"`
}

type createDocumentBody struct {
	SeriesID                string                   `json:"seriesId"`
	IssueDate               string                   `json:"issueDate"`
	IssueTime               *string                  `json:"issueTime,omitempty"`
	DueDate                 *string                  `json:"dueDate,omitempty"`
	CurrencyCode            string                   `json:"currencyCode"`
	OperationType           *string                  `json:"operationType,omitempty"`
	CustomerDocType         string                   `json:"customerDocType"`
	CustomerDocNumber       string                   `json:"customerDocNumber"`
	CustomerName            string                   `json:"customerName"`
	CustomerAddress         *string                  `json:"customerAddress,omitempty"`
	Subtotal                string                   `json:"subtotal"`
	TotalIgv                string                   `json:"totalIgv"`
	TotalIsc                *string                  `json:"totalIsc,omitempty"`
	TotalOtherTaxes         *string                  `json:"totalOtherTaxes,omitempty"`
	TotalDiscount           *string                  `json:"totalDiscount,omitempty"`
	TotalAmount             string                   `json:"totalAmount"`
	TaxInclusiveAmount      *string                  `json:"taxInclusiveAmount,omitempty"`
	Notes                   *string                  `json:"notes,omitempty"`
	ReferenceDocType        *string                  `json:"referenceDocType,omitempty"`
	ReferenceDocSeries      *string                  `json:"referenceDocSeries,omitempty"`
	ReferenceDocCorrelative *int                     `json:"referenceDocCorrelative,omitempty"`
	CreditDebitReasonCode   *string                  `json:"creditDebitReasonCode,omitempty"`
	CreditDebitReasonDesc   *string                  `json:"creditDebitReasonDesc,omitempty"`
	Items                   []createDocumentItemBody `json:"items"`
}

func (b *createDocumentBody) validate() string {
	if !docUUIDRegex.MatchString(b.SeriesID) {
		return "seriesId inválido"
	}
	if !dateRegex.MatchString(b.IssueDate) {
		return "issueDate inválido"
	}
	if b.IssueTime != nil && !timeRegex.MatchString(*b.IssueTime) {
		return "issueTime inválido"
	}
	if b.DueDate != nil && !dateRegex.MatchString(*b.DueDate) {
		return "dueDate inválido"
	}
	if b.CurrencyCode == "" {
		b.CurrencyCode = "PEN"
	}
	if len(b.CurrencyCode) != 3 {
		return "currencyCode inválido"
	}
	if b.CustomerDocType == "" || len(b.CustomerDocType) > 2 {
		return "customerDocType inválido"
	}
	if b.CustomerDocNumber == "" {
		return "customerDocNumber requerido"
	}
	if b.CustomerName == "" {
		return "customerName requerido"
	}
	for _, f := range []struct {
		name string
		val  string
	}{
		{"subtotal", b.Subtotal},
		{"totalIgv", b.TotalIgv},
		{"totalAmount", b.TotalAmount},
	} {
		if !decimalRegex.MatchString(f.val) {
			return f.name + " inválido"
		}
	}
	if len(b.Items) == 0 {
		return "items requerido"
	}
	for i, it := range b.Items {
		if it.Description == "" {
			return "item description requerido"
		}
		if !decimalRegex.MatchString(it.Quantity) {
			return "item quantity inválido"
		}
		if !decimalRegex.MatchString(it.UnitPrice) {
			return "item unitPrice inválido"
		}
		if !decimalRegex.MatchString(it.IgvAmount) {
			return "item igvAmount inválido"
		}
		if !decimalRegex.MatchString(it.LineTotal) {
			return "item lineTotal inválido"
		}
		_ = i
	}
	return ""
}

func (b createDocumentBody) toInput() db.CreateDocumentInput {
	items := make([]db.CreateDocumentItemInput, 0, len(b.Items))
	for i, it := range b.Items {
		line := i + 1
		if it.LineNumber != nil {
			line = *it.LineNumber
		}
		items = append(items, db.CreateDocumentItemInput{
			LineNumber:             line,
			Description:            it.Description,
			Quantity:               it.Quantity,
			UnitCode:               it.UnitCode,
			UnitPrice:              it.UnitPrice,
			UnitPriceWithTax:       it.UnitPriceWithTax,
			TaxExemptionReasonCode: it.TaxExemptionReasonCode,
			IgvAmount:              it.IgvAmount,
			IscAmount:              it.IscAmount,
			DiscountAmount:         it.DiscountAmount,
			LineTotal:              it.LineTotal,
			PriceTypeCode:          it.PriceTypeCode,
		})
	}
	return db.CreateDocumentInput{
		SeriesID:                b.SeriesID,
		IssueDate:               b.IssueDate,
		IssueTime:               b.IssueTime,
		DueDate:                 b.DueDate,
		CurrencyCode:            b.CurrencyCode,
		OperationType:           b.OperationType,
		CustomerDocType:         b.CustomerDocType,
		CustomerDocNumber:       b.CustomerDocNumber,
		CustomerName:            b.CustomerName,
		CustomerAddress:         b.CustomerAddress,
		Subtotal:                b.Subtotal,
		TotalIgv:                b.TotalIgv,
		TotalIsc:                b.TotalIsc,
		TotalOtherTaxes:         b.TotalOtherTaxes,
		TotalDiscount:           b.TotalDiscount,
		TotalAmount:             b.TotalAmount,
		TaxInclusiveAmount:      b.TaxInclusiveAmount,
		Notes:                   b.Notes,
		ReferenceDocType:        b.ReferenceDocType,
		ReferenceDocSeries:      b.ReferenceDocSeries,
		ReferenceDocCorrelative: b.ReferenceDocCorrelative,
		CreditDebitReasonCode:   b.CreditDebitReasonCode,
		CreditDebitReasonDesc:   b.CreditDebitReasonDesc,
		Items:                   items,
	}
}

// createDocumentHandler inserts a draft issued_document after checking the
// tenant's monthly quota, then atomically bumps the issued-document counter.
func (s *server) createDocumentHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	tenantID, ok := auth.TenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "No autenticado")
		return
	}

	var body createDocumentBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Datos inválidos")
		return
	}
	if msg := body.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", msg)
		return
	}

	quota, err := s.pool.CheckDocumentQuota(r.Context(), tenantID, 1)
	if err != nil {
		s.log.Error("check document quota", "error", err, "tenantId", tenantID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if !quota.Allowed {
		limitStr := "ilimitado"
		if quota.Limit != nil {
			limitStr = strconv.Itoa(*quota.Limit)
		}
		writeError(w, http.StatusForbidden, "DOCUMENT_QUOTA_EXCEEDED",
			"Límite mensual de documentos alcanzado ("+limitStr+")")
		return
	}

	doc, err := s.pool.CreateDocumentWithItems(r.Context(), companyID, body.toInput())
	if err != nil {
		switch {
		case errors.Is(err, db.ErrSeriesInactive):
			writeError(w, http.StatusBadRequest, "SERIES_NOT_FOUND", "Serie no encontrada o inactiva")
		case errors.Is(err, db.ErrDuplicate):
			writeError(w, http.StatusConflict, "DOCUMENT_DUPLICATE", "Documento duplicado")
		default:
			s.log.Error("create document", "error", err, "companyId", companyID)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		}
		return
	}

	if err := s.pool.IncrementDocumentUsage(r.Context(), 1); err != nil {
		// Usage is best-effort at this point — the draft already exists. Log
		// so we can investigate drift.
		s.log.Error("increment document usage", "error", err, "tenantId", tenantID)
	}

	writeSuccessStatus(w, http.StatusCreated, doc)
}

// updateDocumentBody accepts the same fields as create, all optional. If
// `items` is present it fully replaces the existing line items.
type updateDocumentBody struct {
	IssueDate               *string                  `json:"issueDate,omitempty"`
	IssueTime               *string                  `json:"issueTime,omitempty"`
	DueDate                 *string                  `json:"dueDate,omitempty"`
	CurrencyCode            *string                  `json:"currencyCode,omitempty"`
	OperationType           *string                  `json:"operationType,omitempty"`
	CustomerDocType         *string                  `json:"customerDocType,omitempty"`
	CustomerDocNumber       *string                  `json:"customerDocNumber,omitempty"`
	CustomerName            *string                  `json:"customerName,omitempty"`
	CustomerAddress         *string                  `json:"customerAddress,omitempty"`
	Subtotal                *string                  `json:"subtotal,omitempty"`
	TotalIgv                *string                  `json:"totalIgv,omitempty"`
	TotalIsc                *string                  `json:"totalIsc,omitempty"`
	TotalOtherTaxes         *string                  `json:"totalOtherTaxes,omitempty"`
	TotalDiscount           *string                  `json:"totalDiscount,omitempty"`
	TotalAmount             *string                  `json:"totalAmount,omitempty"`
	TaxInclusiveAmount      *string                  `json:"taxInclusiveAmount,omitempty"`
	Notes                   *string                  `json:"notes,omitempty"`
	ReferenceDocType        *string                  `json:"referenceDocType,omitempty"`
	ReferenceDocSeries      *string                  `json:"referenceDocSeries,omitempty"`
	ReferenceDocCorrelative *int                     `json:"referenceDocCorrelative,omitempty"`
	CreditDebitReasonCode   *string                  `json:"creditDebitReasonCode,omitempty"`
	CreditDebitReasonDesc   *string                  `json:"creditDebitReasonDesc,omitempty"`
	Items                   []createDocumentItemBody `json:"items,omitempty"`
}

func (b updateDocumentBody) toInput() db.UpdateDocumentInput {
	in := db.UpdateDocumentInput{
		IssueDate:               b.IssueDate,
		IssueTime:               b.IssueTime,
		DueDate:                 b.DueDate,
		CurrencyCode:            b.CurrencyCode,
		OperationType:           b.OperationType,
		CustomerDocType:         b.CustomerDocType,
		CustomerDocNumber:       b.CustomerDocNumber,
		CustomerName:            b.CustomerName,
		CustomerAddress:         b.CustomerAddress,
		Subtotal:                b.Subtotal,
		TotalIgv:                b.TotalIgv,
		TotalIsc:                b.TotalIsc,
		TotalOtherTaxes:         b.TotalOtherTaxes,
		TotalDiscount:           b.TotalDiscount,
		TotalAmount:             b.TotalAmount,
		TaxInclusiveAmount:      b.TaxInclusiveAmount,
		Notes:                   b.Notes,
		ReferenceDocType:        b.ReferenceDocType,
		ReferenceDocSeries:      b.ReferenceDocSeries,
		ReferenceDocCorrelative: b.ReferenceDocCorrelative,
		CreditDebitReasonCode:   b.CreditDebitReasonCode,
		CreditDebitReasonDesc:   b.CreditDebitReasonDesc,
	}
	if b.Items != nil {
		items := make([]db.CreateDocumentItemInput, 0, len(b.Items))
		for i, it := range b.Items {
			line := i + 1
			if it.LineNumber != nil {
				line = *it.LineNumber
			}
			items = append(items, db.CreateDocumentItemInput{
				LineNumber:             line,
				Description:            it.Description,
				Quantity:               it.Quantity,
				UnitCode:               it.UnitCode,
				UnitPrice:              it.UnitPrice,
				UnitPriceWithTax:       it.UnitPriceWithTax,
				TaxExemptionReasonCode: it.TaxExemptionReasonCode,
				IgvAmount:              it.IgvAmount,
				IscAmount:              it.IscAmount,
				DiscountAmount:         it.DiscountAmount,
				LineTotal:              it.LineTotal,
				PriceTypeCode:          it.PriceTypeCode,
			})
		}
		in.Items = items
	}
	return in
}

// updateDocumentHandler patches a draft document.
func (s *server) updateDocumentHandler(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "docId")

	var body updateDocumentBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Datos inválidos")
		return
	}

	doc, err := s.pool.UpdateDraftDocument(r.Context(), docID, body.toInput())
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Documento no encontrado")
		case errors.Is(err, db.ErrNotDraft):
			writeError(w, http.StatusBadRequest, "NOT_DRAFT",
				"Solo documentos en borrador pueden ser modificados")
		default:
			s.log.Error("update document", "error", err, "docId", docID)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		}
		return
	}
	writeSuccess(w, doc)
}

// deleteDocumentHandler removes a draft document.
func (s *server) deleteDocumentHandler(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "docId")

	if err := s.pool.DeleteDraftDocument(r.Context(), docID); err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Documento no encontrado")
		case errors.Is(err, db.ErrNotDraft):
			writeError(w, http.StatusBadRequest, "NOT_DRAFT",
				"Solo documentos en borrador pueden ser eliminados")
		default:
			s.log.Error("delete document", "error", err, "docId", docID)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		}
		return
	}
	writeSuccess(w, map[string]string{"message": "Documento eliminado"})
}

// documentFileHandler returns a presigned R2 URL for the requested file type.
// Valid fileType values: xml, signed_xml, zip, cdr, pdf.
func (s *server) documentFileHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	docID := chi.URLParam(r, "docId")
	fileType := chi.URLParam(r, "fileType")

	doc, err := s.pool.GetIssuedDocument(r.Context(), companyID, docID)
	if err != nil {
		s.log.Error("get document for file", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Documento no encontrado")
		return
	}

	var r2Key *string
	switch fileType {
	case "xml":
		r2Key = doc.R2XmlKey
	case "signed_xml":
		r2Key = doc.R2SignedXmlKey
	case "zip":
		r2Key = doc.R2ZipKey
	case "cdr":
		r2Key = doc.R2CdrKey
	case "pdf":
		r2Key = doc.R2PdfKey
	default:
		writeError(w, http.StatusBadRequest, "INVALID_FILE_TYPE", "Tipo de archivo inválido")
		return
	}
	if r2Key == nil || *r2Key == "" {
		writeError(w, http.StatusNotFound, "FILE_NOT_FOUND", "Archivo no disponible")
		return
	}

	url, err := s.r2.DocumentPresignedURL(r.Context(), *r2Key, 0)
	if err != nil {
		s.log.Error("presign document file", "error", err, "key", *r2Key)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccess(w, map[string]string{"url": url, "fileType": fileType})
}
