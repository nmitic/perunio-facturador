package http

import (
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/perunio/perunio-facturador/internal/auth"
	"github.com/perunio/perunio-facturador/internal/db"
	"github.com/perunio/perunio-facturador/internal/model"
	"github.com/perunio/perunio-facturador/internal/r2"
)

// documentListResponse mirrors the Node.js response shape:
// { success: true, data: [...], pagination: { page, limit, total, totalPages } }.
type documentListResponse struct {
	Success    bool                   `json:"success"`
	Data       []model.IssuedDocument `json:"data"`
	Pagination documentListPagination `json:"pagination"`
}

type documentListPagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

// listDocumentsHandler returns a paginated, filterable slice of issued
// documents for the company.
func (s *Server) listDocumentsHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))

	filter := db.DocumentListFilter{
		DocType:  q.Get("docType"),
		Status:   q.Get("status"),
		Customer: q.Get("customer"),
		Payment:  q.Get("payment"),
		Page:     page,
		Limit:    limit,
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
func (s *Server) getDocumentHandler(w http.ResponseWriter, r *http.Request) {
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
	decimalRegex = regexp.MustCompile(`^\d+(\.\d+)?$`)
	dateRegex    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	timeRegex    = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}$`)
	docUUIDRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

	detraccionCodigoRegex = regexp.MustCompile(`^\d{3}$`) // Cat.54 bien/servicio

	// billingPeriodMaxMonths mirrors BILLING_PERIOD_MAX in the frontend. A decade of
	// months is generous for a real contract and stops a typo turning S/ 100 into
	// S/ 100,000 worth of stored metadata.
	billingPeriodMaxMonths = 120
)

type createDocumentItemBody struct {
	LineNumber             *int    `json:"lineNumber,omitempty"`
	ProductoID             *string `json:"productoId,omitempty"`
	Description            string  `json:"description"`
	Quantity               string  `json:"quantity"`
	UnitCode               string  `json:"unitCode"`
	UnitPrice              string  `json:"unitPrice"`
	UnitPriceWithTax       *string `json:"unitPriceWithTax,omitempty"`
	TaxExemptionReasonCode *string `json:"taxExemptionReasonCode,omitempty"`
	IgvAmount              string  `json:"igvAmount"`
	IscAmount              *string `json:"iscAmount,omitempty"`
	IscTierRange           *string `json:"iscTierRange,omitempty"`
	DiscountAmount         *string `json:"discountAmount,omitempty"`
	LineTotal              string  `json:"lineTotal"`
	PriceTypeCode          *string `json:"priceTypeCode,omitempty"`
}

// detraccionBody is the detracción (SPOT) object on the create/update wire
// contract. CuentaBN is the optional per-document override — when absent the
// company's default cuenta is used at issue time.
type detraccionBody struct {
	Codigo     string  `json:"codigo"`
	Porcentaje string  `json:"porcentaje"`
	Monto      string  `json:"monto"`
	CuentaBN   *string `json:"cuentaBN,omitempty"`
}

type createDocumentBody struct {
	SeriesID          string  `json:"seriesId"`
	IssueDate         string  `json:"issueDate"`
	IssueTime         *string `json:"issueTime,omitempty"`
	DueDate           *string `json:"dueDate,omitempty"`
	CurrencyCode      string  `json:"currencyCode"`
	ExchangeRate      *string `json:"exchangeRate,omitempty"`
	OperationType     *string `json:"operationType,omitempty"`
	CustomerDocType   string  `json:"customerDocType"`
	CustomerDocNumber string  `json:"customerDocNumber"`
	CustomerName      string  `json:"customerName"`
	CustomerAddress   *string `json:"customerAddress,omitempty"`
	Subtotal          string  `json:"subtotal"`
	TotalIgv          string  `json:"totalIgv"`
	TotalIsc          *string `json:"totalIsc,omitempty"`
	TotalOtherTaxes   *string `json:"totalOtherTaxes,omitempty"`
	TotalDiscount     *string `json:"totalDiscount,omitempty"`
	GlobalDiscount    *string `json:"globalDiscount,omitempty"`
	// BillingPeriodMonths is the periodo de facturación the browser applied. Purely
	// reporting metadata: the items already carry the multiplied prices and the
	// annotated descriptions, and this never reaches the XML.
	BillingPeriodMonths     *int                     `json:"billingPeriodMonths,omitempty"`
	TotalAmount             string                   `json:"totalAmount"`
	TaxInclusiveAmount      *string                  `json:"taxInclusiveAmount,omitempty"`
	Notes                   *string                  `json:"notes,omitempty"`
	FormaPago               *string                  `json:"formaPago,omitempty"`
	Cuotas                  []model.CuotaCredito     `json:"cuotas,omitempty"`
	ReferenceDocType        *string                  `json:"referenceDocType,omitempty"`
	ReferenceDocSeries      *string                  `json:"referenceDocSeries,omitempty"`
	ReferenceDocCorrelative *int                     `json:"referenceDocCorrelative,omitempty"`
	CreditDebitReasonCode   *string                  `json:"creditDebitReasonCode,omitempty"`
	CreditDebitReasonDesc   *string                  `json:"creditDebitReasonDesc,omitempty"`
	Detraccion              *detraccionBody          `json:"detraccion,omitempty"`
	Items                   []createDocumentItemBody `json:"items"`
}

// validate checks a detracción wire object's field formats. Returns "" when nil
// or valid. Business rules (doc type, cuenta BN presence, monto ≈ % × total)
// are enforced by validation.validateDetraccion at issue time.
func (d *detraccionBody) validate() string {
	if d == nil {
		return ""
	}
	if !detraccionCodigoRegex.MatchString(d.Codigo) {
		return "detraccion.codigo inválido"
	}
	if !decimalRegex.MatchString(d.Porcentaje) {
		return "detraccion.porcentaje inválido"
	}
	if !decimalRegex.MatchString(d.Monto) {
		return "detraccion.monto inválido"
	}
	return ""
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
	// Tipo de cambio: solo se valida el formato cuando viene. La obligatoriedad
	// para moneda extranjera la impone el frontend (registro PLE/detracción).
	if b.ExchangeRate != nil && *b.ExchangeRate != "" && !decimalRegex.MatchString(*b.ExchangeRate) {
		return "exchangeRate inválido"
	}
	// Customer is optional at draft time: a boleta with no customer is defaulted
	// to consumidor final when stored, and the issue-time validator is the real
	// gate (facturas still require a RUC there). Only enforce format here.
	if len(b.CustomerDocType) > 2 {
		return "customerDocType inválido"
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
	if b.GlobalDiscount != nil && *b.GlobalDiscount != "" && !decimalRegex.MatchString(*b.GlobalDiscount) {
		return "globalDiscount inválido"
	}
	if msg := validateBillingPeriodMonths(b.BillingPeriodMonths); msg != "" {
		return msg
	}
	if msg := b.Detraccion.validate(); msg != "" {
		return msg
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

// nilIfEmpty returns nil for a nil or empty-string pointer, so an omitted or
// blank optional numeric (e.g. exchangeRate on a PEN doc) is stored as SQL NULL
// rather than an empty string the numeric column would reject.
func nilIfEmpty(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
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
			ProductoID:             it.ProductoID,
			Description:            it.Description,
			Quantity:               it.Quantity,
			UnitCode:               it.UnitCode,
			UnitPrice:              it.UnitPrice,
			UnitPriceWithTax:       it.UnitPriceWithTax,
			TaxExemptionReasonCode: it.TaxExemptionReasonCode,
			IgvAmount:              it.IgvAmount,
			IscAmount:              it.IscAmount,
			IscTierRange:           it.IscTierRange,
			DiscountAmount:         it.DiscountAmount,
			LineTotal:              it.LineTotal,
			PriceTypeCode:          it.PriceTypeCode,
		})
	}
	in := db.CreateDocumentInput{
		SeriesID:                b.SeriesID,
		IssueDate:               b.IssueDate,
		IssueTime:               b.IssueTime,
		DueDate:                 b.DueDate,
		CurrencyCode:            b.CurrencyCode,
		ExchangeRate:            nilIfEmpty(b.ExchangeRate),
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
		GlobalDiscount:          b.GlobalDiscount,
		BillingPeriodMonths:     b.BillingPeriodMonths,
		TotalAmount:             b.TotalAmount,
		TaxInclusiveAmount:      b.TaxInclusiveAmount,
		Notes:                   b.Notes,
		FormaPago:               b.FormaPago,
		Cuotas:                  b.Cuotas,
		ReferenceDocType:        b.ReferenceDocType,
		ReferenceDocSeries:      b.ReferenceDocSeries,
		ReferenceDocCorrelative: b.ReferenceDocCorrelative,
		CreditDebitReasonCode:   b.CreditDebitReasonCode,
		CreditDebitReasonDesc:   b.CreditDebitReasonDesc,
		Items:                   items,
	}
	if d := b.Detraccion; d != nil {
		in.DetraccionCodigo = &d.Codigo
		in.DetraccionPorcentaje = &d.Porcentaje
		in.DetraccionMonto = &d.Monto
		in.DetraccionCuentaBN = d.CuentaBN
	}
	return in
}

// createDocumentHandler inserts a draft issued_document after checking the
// tenant's monthly quota, then atomically bumps the issued-document counter.
//
// The quota check and insert live in createDocumentDraft (pipeline_core.go) so the
// background scheduler creates drafts through the same path.
func (s *Server) createDocumentHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")

	var body createDocumentBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Datos inválidos")
		return
	}
	if msg := body.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", msg)
		return
	}

	doc, pErr := s.createDocumentDraft(r.Context(), companyID, body.toInput())
	if pErr != nil {
		writePipelineError(w, pErr)
		return
	}

	writeSuccessStatus(w, http.StatusCreated, doc)
}

// validateBillingPeriodMonths guards the periodo de facturación. Absent means no
// billing period; present must be a sane month count. It is metadata only, so a bad
// value can't corrupt a comprobante — but a nonsense one would poison the reports.
func validateBillingPeriodMonths(months *int) string {
	if months == nil {
		return ""
	}
	if *months < 1 || *months > billingPeriodMaxMonths {
		return "billingPeriodMonths inválido"
	}
	return ""
}

// updateDocumentBody accepts the same fields as create, all optional. If
// `items` is present it fully replaces the existing line items.
type updateDocumentBody struct {
	IssueDate               *string                  `json:"issueDate,omitempty"`
	IssueTime               *string                  `json:"issueTime,omitempty"`
	DueDate                 *string                  `json:"dueDate,omitempty"`
	CurrencyCode            *string                  `json:"currencyCode,omitempty"`
	ExchangeRate            *string                  `json:"exchangeRate,omitempty"`
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
	GlobalDiscount          *string                  `json:"globalDiscount,omitempty"`
	BillingPeriodMonths     *int                     `json:"billingPeriodMonths,omitempty"`
	TotalAmount             *string                  `json:"totalAmount,omitempty"`
	TaxInclusiveAmount      *string                  `json:"taxInclusiveAmount,omitempty"`
	Notes                   *string                  `json:"notes,omitempty"`
	FormaPago               *string                  `json:"formaPago,omitempty"`
	Cuotas                  *[]model.CuotaCredito    `json:"cuotas,omitempty"`
	ReferenceDocType        *string                  `json:"referenceDocType,omitempty"`
	ReferenceDocSeries      *string                  `json:"referenceDocSeries,omitempty"`
	ReferenceDocCorrelative *int                     `json:"referenceDocCorrelative,omitempty"`
	CreditDebitReasonCode   *string                  `json:"creditDebitReasonCode,omitempty"`
	CreditDebitReasonDesc   *string                  `json:"creditDebitReasonDesc,omitempty"`
	Detraccion              *detraccionBody          `json:"detraccion,omitempty"`
	Items                   []createDocumentItemBody `json:"items,omitempty"`
}

func (b updateDocumentBody) toInput() db.UpdateDocumentInput {
	in := db.UpdateDocumentInput{
		IssueDate:               b.IssueDate,
		IssueTime:               b.IssueTime,
		DueDate:                 b.DueDate,
		CurrencyCode:            b.CurrencyCode,
		ExchangeRate:            nilIfEmpty(b.ExchangeRate),
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
		GlobalDiscount:          b.GlobalDiscount,
		BillingPeriodMonths:     b.BillingPeriodMonths,
		TotalAmount:             b.TotalAmount,
		TaxInclusiveAmount:      b.TaxInclusiveAmount,
		Notes:                   b.Notes,
		FormaPago:               b.FormaPago,
		ReferenceDocType:        b.ReferenceDocType,
		ReferenceDocSeries:      b.ReferenceDocSeries,
		ReferenceDocCorrelative: b.ReferenceDocCorrelative,
		CreditDebitReasonCode:   b.CreditDebitReasonCode,
		CreditDebitReasonDesc:   b.CreditDebitReasonDesc,
	}
	if b.Cuotas != nil {
		in.CuotasIsSet = true
		in.Cuotas = *b.Cuotas
	}
	if d := b.Detraccion; d != nil {
		in.DetraccionCodigo = &d.Codigo
		in.DetraccionPorcentaje = &d.Porcentaje
		in.DetraccionMonto = &d.Monto
		in.DetraccionCuentaBN = d.CuentaBN
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
				ProductoID:             it.ProductoID,
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
func (s *Server) updateDocumentHandler(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "docId")

	var body updateDocumentBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Datos inválidos")
		return
	}

	if msg := validateBillingPeriodMonths(body.BillingPeriodMonths); msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", msg)
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
func (s *Server) deleteDocumentHandler(w http.ResponseWriter, r *http.Request) {
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
func (s *Server) documentFileHandler(w http.ResponseWriter, r *http.Request) {
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
	var r2FileType r2.DocumentFileType
	switch fileType {
	case "xml":
		r2Key, r2FileType = doc.R2XmlKey, r2.FileXML
	case "signed_xml":
		r2Key, r2FileType = doc.R2SignedXmlKey, r2.FileSignedXML
	case "zip":
		r2Key, r2FileType = doc.R2ZipKey, r2.FileZIP
	case "cdr":
		r2Key, r2FileType = doc.R2CdrKey, r2.FileCDR
	case "pdf":
		r2Key, r2FileType = doc.R2PdfKey, r2.FilePDF
	default:
		writeError(w, http.StatusBadRequest, "INVALID_FILE_TYPE", "Tipo de archivo inválido")
		return
	}
	if r2Key == nil || *r2Key == "" {
		writeError(w, http.StatusNotFound, "FILE_NOT_FOUND", "Archivo no disponible")
		return
	}

	// Presign with a canonical SUNAT filename + attachment disposition so the
	// browser downloads (instead of rendering PDF/XML inline in a tab) under a
	// meaningful name rather than the object key's basename ("pdf.pdf"/"zip.zip").
	var downloadName string
	if company, cErr := s.pool.GetCompany(r.Context(), companyID); cErr == nil && company != nil {
		downloadName = r2.DocumentDownloadName(
			company.RUC, doc.DocType, doc.Series, doc.Correlative, r2FileType,
		)
	}

	url, err := s.r2.DocumentPresignedURL(r.Context(), *r2Key, downloadName, 0)
	if err != nil {
		s.log.Error("presign document file", "error", err, "key", *r2Key)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccess(w, map[string]string{"url": url, "fileType": fileType})
}

// maxDocumentPdfBytes caps the client-rendered PDF upload body.
const maxDocumentPdfBytes = 15 << 20 // 15 MiB

// uploadDocumentPdfHandler stores the client-rendered comprobante PDF in R2,
// under the same key layout as the other artifacts
// (documents/{tenant}/{company}/{docId}/pdf.pdf), and persists the key on the
// row so it becomes downloadable later via documentFileHandler. The body is the
// raw PDF bytes (Content-Type: application/pdf).
func (s *Server) uploadDocumentPdfHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	docID := chi.URLParam(r, "docId")

	payload, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "Autenticación requerida")
		return
	}

	doc, err := s.pool.GetIssuedDocument(r.Context(), companyID, docID)
	if err != nil {
		s.log.Error("get document for pdf upload", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Documento no encontrado")
		return
	}
	// Only issued documents have a signed XML; the PDF is rendered from it, so
	// refuse uploads for drafts (mirrors the frontend's canPreviewPdf gate).
	if doc.R2SignedXmlKey == nil || *doc.R2SignedXmlKey == "" {
		writeError(w, http.StatusConflict, "NOT_ISSUED", "El comprobante aún no ha sido emitido")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxDocumentPdfBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "No se pudo leer el archivo PDF")
		return
	}
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, "EMPTY_BODY", "El archivo PDF está vacío")
		return
	}

	key := r2.DocumentKey(payload.TenantID, companyID, docID, r2.FilePDF)
	if err := s.r2.UploadDocumentFile(r.Context(), key, r2.FilePDF, data); err != nil {
		s.log.Error("upload document pdf", "error", err, "key", key)
		writeError(w, http.StatusInternalServerError, "R2_UPLOAD_ERROR", "No se pudo guardar el PDF")
		return
	}

	if err := s.pool.SetDocumentPdfKey(r.Context(), docID, key); err != nil {
		s.log.Error("persist document pdf key", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}

	writeSuccess(w, map[string]string{"r2PdfKey": key})
}

// markPaymentBody is the wire contract for manually marking a contado document
// as paid. paidAt is required; the rest are optional record-keeping.
type markPaymentBody struct {
	PaidAt    string  `json:"paidAt"`
	Method    *string `json:"method,omitempty"`
	Reference *string `json:"reference,omitempty"`
	Notes     *string `json:"notes,omitempty"`
}

// setDocumentPaymentHandler records a manual payment on a CONTADO document.
// Crédito documents track payment per-cuota (see the /payments endpoints) so
// they're rejected here — there must be a single source of truth for their paid
// state. Drafts and voided documents can't be paid either.
func (s *Server) setDocumentPaymentHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	docID := chi.URLParam(r, "docId")

	var b markPaymentBody
	if err := decodeBody(r, &b); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Cuerpo de solicitud inválido")
		return
	}
	if !dateRegex.MatchString(b.PaidAt) {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "paidAt inválido (YYYY-MM-DD)")
		return
	}

	doc, err := s.pool.GetIssuedDocument(r.Context(), companyID, docID)
	if err != nil {
		s.log.Error("get document for mark paid", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Documento no encontrado")
		return
	}
	if doc.FormaPago != nil && *doc.FormaPago == "credito" {
		writeError(w, http.StatusConflict, "CREDIT_DOCUMENT", "Los comprobantes al crédito registran su pago por cuotas")
		return
	}
	if doc.Status == "draft" || doc.Status == "voided" {
		writeError(w, http.StatusConflict, "INVALID_STATUS", "Solo un comprobante emitido y vigente puede marcarse como pagado")
		return
	}

	if err := s.pool.SetDocumentPayment(r.Context(), docID, b.PaidAt, b.Method, b.Reference, b.Notes); err != nil {
		s.log.Error("set document payment", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}

	updated, err := s.pool.GetIssuedDocument(r.Context(), companyID, docID)
	if err != nil || updated == nil {
		writeSuccess(w, map[string]bool{"ok": true})
		return
	}
	writeSuccess(w, updated)
}

// clearDocumentPaymentHandler reverts a document to unpaid.
func (s *Server) clearDocumentPaymentHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	docID := chi.URLParam(r, "docId")

	doc, err := s.pool.GetIssuedDocument(r.Context(), companyID, docID)
	if err != nil {
		s.log.Error("get document for clear payment", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Documento no encontrado")
		return
	}

	if err := s.pool.ClearDocumentPayment(r.Context(), docID); err != nil {
		s.log.Error("clear document payment", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccess(w, map[string]string{"message": "Pago eliminado"})
}
