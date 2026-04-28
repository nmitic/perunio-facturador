package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/perunio/perunio-facturador/internal/auth"
	"github.com/perunio/perunio-facturador/internal/cdr"
	facturadorCrypto "github.com/perunio/perunio-facturador/internal/crypto"
	"github.com/perunio/perunio-facturador/internal/db"
	"github.com/perunio/perunio-facturador/internal/model"
	"github.com/perunio/perunio-facturador/internal/pdf"
	"github.com/perunio/perunio-facturador/internal/r2"
	"github.com/perunio/perunio-facturador/internal/signature"
	"github.com/perunio/perunio-facturador/internal/soap"
	"github.com/perunio/perunio-facturador/internal/xmlbuilder"
	"github.com/perunio/perunio-facturador/internal/zipper"
)

// pipelineBody is the optional body accepted by every issue/poll endpoint.
// Environment defaults to "beta" when not set.
type pipelineBody struct {
	Environment string `json:"environment,omitempty"`
}

// readPipelineEnv decodes the optional {environment} body without failing on
// empty request bodies. When no body (or an unrecognized value) is provided
// this returns "", letting callers fall back to the per-company default via
// resolvePipelineEnv.
func readPipelineEnv(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	var body pipelineBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		return ""
	}
	switch body.Environment {
	case "beta", "production":
		return body.Environment
	}
	return ""
}

// resolvePipelineEnv picks the SUNAT environment for this pipeline run. An
// explicit override from the request body wins; otherwise the per-company
// default wins; otherwise we fall back to "beta".
func resolvePipelineEnv(override, companyDefault string) string {
	if override == "beta" || override == "production" {
		return override
	}
	if companyDefault == "beta" || companyDefault == "production" {
		return companyDefault
	}
	return "beta"
}

// trailingDigits pulls the trailing integer from a string like "RC-20240115-001".
// Returns 1 when no digits are present so filenames always have a correlative.
var trailingDigitsRegex = regexp.MustCompile(`(\d+)$`)

func trailingDigits(s string) int {
	m := trailingDigitsRegex.FindString(s)
	if m == "" {
		return 1
	}
	n, err := strconv.Atoi(m)
	if err != nil || n <= 0 {
		return 1
	}
	return n
}

// pipelineDeps bundles the values resolved at the start of every pipeline
// handler: tenant id, decrypted SUNAT password, and the parsed PFX certificate.
type pipelineDeps struct {
	tenantID      string
	company       *db.Company
	sunatUsername string
	sunatPassword string
	parsedCert    *signature.ParsedCertificate
}

// loadPipelineDeps performs the three lookups every pipeline handler needs:
//  1. Tenant ID from JWT context
//  2. Company row + decrypted SUNAT SOL password
//  3. Active certificate (PFX fetched from R2, password decrypted, PKCS#12 parsed)
//
// Returns (nil, statusCode, errorCode, message) on failure; the caller writes
// the response directly. The helper never panics on missing data.
func (s *server) loadPipelineDeps(w http.ResponseWriter, r *http.Request, companyID string) (*pipelineDeps, bool) {
	tenantID, ok := auth.TenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "No autenticado")
		return nil, false
	}

	company, err := s.pool.GetCompany(r.Context(), companyID)
	if err != nil {
		s.log.Error("get company", "error", err, "companyId", companyID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return nil, false
	}
	if company == nil {
		writeError(w, http.StatusNotFound, "COMPANY_NOT_FOUND", "Empresa no encontrada")
		return nil, false
	}
	if company.Username == nil || company.EncryptedPassword == nil {
		writeError(w, http.StatusBadRequest, "SUNAT_CREDENTIALS_MISSING",
			"Credenciales SOL no configuradas para esta empresa")
		return nil, false
	}

	sunatPassword, err := facturadorCrypto.DecryptAES256GCM(*company.EncryptedPassword, s.cfg.EncryptionKey)
	if err != nil {
		ivHex := strings.SplitN(*company.EncryptedPassword, ":", 2)[0]
		s.log.Error("decrypt sunat password", "error", err, "companyId", companyID, "ivHexLen", len(ivHex))
		writeError(w, http.StatusInternalServerError, "SUNAT_DECRYPT_ERROR",
			"No se pudo descifrar la contraseña SOL")
		return nil, false
	}

	activeCert, err := s.pool.GetActiveCertificateForSigning(r.Context(), companyID)
	if err != nil {
		s.log.Error("get active certificate", "error", err, "companyId", companyID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return nil, false
	}
	if activeCert == nil {
		writeError(w, http.StatusBadRequest, "CERTIFICATE_MISSING",
			"No hay un certificado activo para esta empresa")
		return nil, false
	}

	parsed, err := s.certCache.GetOrLoad(activeCert.CertID, func() (*signature.ParsedCertificate, error) {
		privateKeyPEM, decryptErr := facturadorCrypto.DecryptAES256GCM(activeCert.EncryptedPrivateKeyPEM, s.cfg.EncryptionKey)
		if decryptErr != nil {
			return nil, fmt.Errorf("decrypt private key: %w", decryptErr)
		}
		return signature.ParsePEMKeyAndCert([]byte(privateKeyPEM), []byte(activeCert.CertificatePEM))
	})
	if err != nil {
		s.log.Error("load active certificate", "error", err, "companyId", companyID, "certId", activeCert.CertID)
		writeError(w, http.StatusInternalServerError, "CERT_LOAD_ERROR",
			"No se pudo cargar el certificado activo")
		return nil, false
	}

	return &pipelineDeps{
		tenantID:      tenantID,
		company:       company,
		sunatUsername: *company.Username,
		sunatPassword: sunatPassword,
		parsedCert:    parsed,
	}, true
}

// buildIssueRequestFromDoc materializes a model.IssueRequest from the DB row +
// its items + the enclosing company context. The model.IssueRequest struct
// still carries the CertificateURL/CertificatePassword/SunatUsername fields
// used by the legacy stateless pipeline, but xmlbuilder/signature/soap never
// read them — they're populated downstream from pipelineDeps. We leave them
// empty here.
func buildIssueRequestFromDoc(
	doc *model.IssuedDocument,
	items []model.IssuedDocumentItem,
	supplierRUC, supplierName, supplierAddress string,
) model.IssueRequest {
	req := model.IssueRequest{
		SupplierRUC:       supplierRUC,
		SupplierName:      supplierName,
		SupplierAddress:   supplierAddress,
		EstablishmentCode: "0000",

		DocType:     doc.DocType,
		Series:      doc.Series,
		Correlative: doc.Correlative,

		IssueDate: doc.IssueDate.Format("2006-01-02"),
		IssueTime: derefString(doc.IssueTime, "00:00:00"),

		CurrencyCode:  doc.CurrencyCode,
		OperationType: derefString(doc.OperationType, "0101"),

		CustomerDocType:   doc.CustomerDocType,
		CustomerDocNumber: doc.CustomerDocNumber,
		CustomerName:      doc.CustomerName,
		CustomerAddress:   derefString(doc.CustomerAddress, ""),

		Subtotal:           doc.Subtotal,
		TotalIGV:           doc.TotalIgv,
		TotalISC:           derefString(doc.TotalIsc, "0.00"),
		TotalOtherTaxes:    derefString(doc.TotalOtherTaxes, "0.00"),
		TotalDiscount:      derefString(doc.TotalDiscount, "0.00"),
		TotalAmount:        doc.TotalAmount,
		TaxInclusiveAmount: derefString(doc.TaxInclusiveAmount, doc.TotalAmount),
	}

	if doc.Notes != nil && *doc.Notes != "" {
		req.Notes = []model.Note{{Code: "1000", Text: *doc.Notes}}
	}

	if doc.ReferenceDocType != nil {
		req.ReferenceDocType = *doc.ReferenceDocType
	}
	if doc.ReferenceDocSeries != nil {
		req.ReferenceDocSeries = *doc.ReferenceDocSeries
	}
	if doc.ReferenceDocCorrelative != nil {
		req.ReferenceDocCorrelative = *doc.ReferenceDocCorrelative
	}
	if doc.CreditDebitReasonCode != nil {
		req.ReasonCode = *doc.CreditDebitReasonCode
	}
	if doc.CreditDebitReasonDesc != nil {
		req.ReasonDescription = *doc.CreditDebitReasonDesc
	}

	for _, it := range items {
		req.Items = append(req.Items, model.LineItem{
			LineNumber:             it.LineNumber,
			Description:            it.Description,
			Quantity:               it.Quantity,
			UnitCode:               it.UnitCode,
			UnitPrice:              it.UnitPrice,
			UnitPriceWithTax:       derefString(it.UnitPriceWithTax, it.UnitPrice),
			TaxExemptionReasonCode: derefString(it.TaxExemptionReasonCode, "10"),
			IGVAmount:              it.IgvAmount,
			ISCAmount:              derefString(it.IscAmount, "0.00"),
			ISCTierRange:           derefString(it.IscTierRange, ""),
			DiscountAmount:         derefString(it.DiscountAmount, "0.00"),
			LineTotal:              it.LineTotal,
			PriceTypeCode:          derefString(it.PriceTypeCode, "01"),
		})
	}
	return req
}

func derefString(s *string, fallback string) string {
	if s == nil || *s == "" {
		return fallback
	}
	return *s
}

// issueDocumentPipelineHandler runs the full issue pipeline against a draft
// issued_documents row loaded from the DB. This replaces the legacy stateless
// POST /api/v1/documents/issue endpoint.
func (s *server) issueDocumentPipelineHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	docID := chi.URLParam(r, "docId")
	envOverride := readPipelineEnv(r)

	// 1. Load the draft document + items.
	doc, err := s.pool.GetIssuedDocument(r.Context(), companyID, docID)
	if err != nil {
		s.log.Error("get document for issue", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Documento no encontrado")
		return
	}
	if doc.Status == "accepted" || doc.Status == "accepted_with_observations" {
		writeError(w, http.StatusBadRequest, "ALREADY_ACCEPTED",
			"El documento ya fue aceptado por SUNAT")
		return
	}

	items, err := s.pool.GetIssuedDocumentItems(r.Context(), docID)
	if err != nil {
		s.log.Error("get document items for issue", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if len(items) == 0 {
		writeError(w, http.StatusBadRequest, "NO_ITEMS", "El documento no tiene líneas")
		return
	}

	// 2. Load company + SUNAT creds + active certificate.
	deps, ok := s.loadPipelineDeps(w, r, companyID)
	if !ok {
		return
	}
	env := resolvePipelineEnv(envOverride, deps.company.SunatEnvironment)

	// 3. Fiscal address from the public SSCO table, falling back to a
	// placeholder so SUNAT doesn't reject the invoice during onboarding.
	address, _ := s.pool.GetFiscalAddressByRUC(r.Context(), deps.company.RUC)
	if address == "" {
		address = "-"
	}

	// 4. Build the IssueRequest and run the compliance pipeline. These are
	// the same pure functions the old stateless handler used.
	issueReq := buildIssueRequestFromDoc(doc, items, deps.company.RUC, deps.company.CompanyName, address)
	issueReq.Environment = env

	xmlBytes, err := xmlbuilder.BuildDocumentXML(issueReq)
	if err != nil {
		s.log.Error("build document xml", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "XML_BUILD_ERROR", err.Error())
		return
	}

	signedXML, err := signature.SignXML(xmlBytes, deps.parsedCert.PrivateKeyPEM, deps.parsedCert.CertPEM)
	if err != nil {
		s.log.Error("sign document xml", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "SIGN_ERROR", err.Error())
		return
	}

	filename := xmlbuilder.Filename(issueReq.SupplierRUC, issueReq.DocType, issueReq.Series, issueReq.Correlative)
	zipBytes, err := zipper.CreateZIP(filename, signedXML)
	if err != nil {
		s.log.Error("create zip", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "ZIP_ERROR", err.Error())
		return
	}

	soapClient := soap.NewClient(env, s.cfg.SunatBetaURL, s.cfg.SunatProductionURL, s.cfg.SunatConsultURL, s.cfg.SunatTimeoutSeconds)
	sendStart := time.Now()
	sendResult, err := soapClient.SendBill(deps.sunatUsername, deps.sunatPassword, filename, zipBytes)
	sendDurationMs := int(time.Since(sendStart).Milliseconds())
	if err != nil {
		s.log.Error("send bill", "error", err, "docId", docID)
		docRef := docID
		errDesc := err.Error()
		if logErr := s.pool.InsertSubmissionLog(r.Context(), db.SubmissionLogEntry{
			CompanyID: companyID, DocumentID: &docRef, Action: "sendBill",
			ResponseDescription: &errDesc, DurationMs: &sendDurationMs,
		}); logErr != nil {
			s.log.Warn("write submission log", "error", logErr, "docId", docID)
		}
		writeError(w, http.StatusBadGateway, "SUNAT_ERROR", err.Error())
		return
	}

	parsedCDR, err := cdr.Parse(sendResult.ApplicationResponse)
	if err != nil {
		s.log.Error("parse cdr", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "CDR_PARSE_ERROR", err.Error())
		return
	}

	{
		docRef := docID
		resCode := parsedCDR.ResponseCode
		resDesc := parsedCDR.Description
		if logErr := s.pool.InsertSubmissionLog(r.Context(), db.SubmissionLogEntry{
			CompanyID:           companyID,
			DocumentID:          &docRef,
			Action:              "sendBill",
			ResponseCode:        &resCode,
			ResponseDescription: &resDesc,
			DurationMs:          &sendDurationMs,
		}); logErr != nil {
			s.log.Warn("write submission log", "error", logErr, "docId", docID)
		}
	}

	// Build QR data (same format the legacy handler used).
	digestVal, _ := signature.DigestValue(signedXML)
	qrData := fmt.Sprintf("%s|%s|%s|%08d|%s|%s|%s|%s|%s|%s|",
		issueReq.SupplierRUC, issueReq.DocType, issueReq.Series, issueReq.Correlative,
		issueReq.TotalIGV, issueReq.TotalAmount, issueReq.IssueDate,
		issueReq.CustomerDocType, issueReq.CustomerDocNumber, digestVal)

	// PDF is non-fatal — persist the document even if PDF generation fails.
	pdfBytes, pdfErr := pdf.Generate(issueReq, qrData)
	if pdfErr != nil {
		s.log.Warn("generate pdf", "error", pdfErr, "docId", docID)
	}

	// 5. Upload artifacts to R2 under the canonical documents/{tenantId}/...
	// layout. Failures here are fatal — the DB row must not claim R2 keys
	// that don't exist.
	signedKey := r2.DocumentKey(deps.tenantID, companyID, docID, r2.FileSignedXML)
	if err := s.r2.UploadDocumentFile(r.Context(), signedKey, r2.FileSignedXML, signedXML); err != nil {
		s.log.Error("upload signed xml", "error", err, "key", signedKey)
		writeError(w, http.StatusInternalServerError, "R2_UPLOAD_ERROR", err.Error())
		return
	}
	zipKey := r2.DocumentKey(deps.tenantID, companyID, docID, r2.FileZIP)
	if err := s.r2.UploadDocumentFile(r.Context(), zipKey, r2.FileZIP, zipBytes); err != nil {
		s.log.Error("upload zip", "error", err, "key", zipKey)
		writeError(w, http.StatusInternalServerError, "R2_UPLOAD_ERROR", err.Error())
		return
	}
	cdrKey := r2.DocumentKey(deps.tenantID, companyID, docID, r2.FileCDR)
	if err := s.r2.UploadDocumentFile(r.Context(), cdrKey, r2.FileCDR, sendResult.ApplicationResponse); err != nil {
		s.log.Error("upload cdr", "error", err, "key", cdrKey)
		writeError(w, http.StatusInternalServerError, "R2_UPLOAD_ERROR", err.Error())
		return
	}
	var pdfKey *string
	if pdfBytes != nil {
		k := r2.DocumentKey(deps.tenantID, companyID, docID, r2.FilePDF)
		if err := s.r2.UploadDocumentFile(r.Context(), k, r2.FilePDF, pdfBytes); err != nil {
			s.log.Warn("upload pdf", "error", err, "key", k)
		} else {
			pdfKey = &k
		}
	}

	// 6. Update the DB row with the pipeline outcome.
	status := "rejected"
	if parsedCDR.Accepted {
		if len(parsedCDR.Notes) > 0 {
			status = "accepted_with_observations"
		} else {
			status = "accepted"
		}
	}

	dbResult := db.IssuedDocumentResult{
		Status:                   status,
		SunatResponseCode:        &parsedCDR.ResponseCode,
		SunatResponseDescription: &parsedCDR.Description,
		R2SignedXmlKey:           &signedKey,
		R2ZipKey:                 &zipKey,
		R2CdrKey:                 &cdrKey,
		R2PdfKey:                 pdfKey,
		QRData:                   &qrData,
		MarkSent:                 true,
		MarkAccepted:             parsedCDR.Accepted,
	}
	updated, err := s.pool.ApplyIssueResult(r.Context(), docID, dbResult)
	if err != nil {
		s.log.Error("apply issue result", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}

	writeSuccess(w, updated)
}

// issueSummaryPipelineHandler signs a Resumen Diario (RC) and ships it off to
// SUNAT, storing the returned ticket on the daily_summaries row for later
// polling.
func (s *server) issueSummaryPipelineHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	summaryID := chi.URLParam(r, "summaryId")
	envOverride := readPipelineEnv(r)

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
	if summary.Status == "accepted" || summary.Status == "accepted_with_observations" {
		writeError(w, http.StatusBadRequest, "ALREADY_ACCEPTED",
			"El resumen ya fue aceptado por SUNAT")
		return
	}

	items, err := s.pool.GetDailySummaryIssueItems(r.Context(), summaryID)
	if err != nil {
		s.log.Error("get summary issue items", "error", err, "summaryId", summaryID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if len(items) == 0 {
		writeError(w, http.StatusBadRequest, "NO_ITEMS", "El resumen no tiene documentos enlazados")
		return
	}

	deps, ok := s.loadPipelineDeps(w, r, companyID)
	if !ok {
		return
	}
	env := resolvePipelineEnv(envOverride, deps.company.SunatEnvironment)

	correlative := trailingDigits(summary.SummaryID)
	issueDate := summary.ReferenceDate.Format("2006-01-02")
	refDate := summary.ReferenceDate.Format("2006-01-02")

	summaryReq := model.SummaryRequest{
		SupplierRUC:   deps.company.RUC,
		SupplierName:  deps.company.CompanyName,
		IssueDate:     issueDate,
		ReferenceDate: refDate,
		Correlative:   correlative,
		Environment:   env,
	}
	for i, it := range items {
		summaryReq.Items = append(summaryReq.Items, model.SummaryItem{
			LineNumber:        i + 1,
			DocType:           it.DocType,
			Series:            it.Series,
			StartCorrelative:  it.Correlative,
			EndCorrelative:    it.Correlative,
			ConditionCode:     it.ConditionCode,
			CustomerDocType:   it.CustomerDocType,
			CustomerDocNumber: it.CustomerDocNumber,
			CurrencyCode:      it.CurrencyCode,
			TotalAmount:       it.TotalAmount,
			TotalIGV:          it.TotalIgv,
			TotalISC:          it.TotalIsc,
			TotalOtherTaxes:   it.TotalOtherTaxes,
		})
	}

	xmlBytes, err := xmlbuilder.BuildSummaryXML(summaryReq)
	if err != nil {
		s.log.Error("build summary xml", "error", err, "summaryId", summaryID)
		writeError(w, http.StatusInternalServerError, "XML_BUILD_ERROR", err.Error())
		return
	}

	signedXML, err := signature.SignXML(xmlBytes, deps.parsedCert.PrivateKeyPEM, deps.parsedCert.CertPEM)
	if err != nil {
		s.log.Error("sign summary xml", "error", err, "summaryId", summaryID)
		writeError(w, http.StatusInternalServerError, "SIGN_ERROR", err.Error())
		return
	}

	filename := xmlbuilder.SummaryFilename(summaryReq.SupplierRUC, summaryReq.IssueDate, summaryReq.Correlative)
	zipBytes, err := zipper.CreateZIP(filename, signedXML)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ZIP_ERROR", err.Error())
		return
	}

	soapClient := soap.NewClient(env, s.cfg.SunatBetaURL, s.cfg.SunatProductionURL, s.cfg.SunatConsultURL, s.cfg.SunatTimeoutSeconds)
	sendResult, err := soapClient.SendSummary(deps.sunatUsername, deps.sunatPassword, filename, zipBytes)
	if err != nil {
		s.log.Error("send summary", "error", err, "summaryId", summaryID)
		writeError(w, http.StatusBadGateway, "SUNAT_ERROR", err.Error())
		return
	}

	// Upload XML artifacts — the CDR will come in later via poll.
	signedKey := r2.DocumentKey(deps.tenantID, companyID, summaryID, r2.FileSignedXML)
	if err := s.r2.UploadDocumentFile(r.Context(), signedKey, r2.FileSignedXML, signedXML); err != nil {
		s.log.Error("upload summary signed xml", "error", err, "key", signedKey)
		writeError(w, http.StatusInternalServerError, "R2_UPLOAD_ERROR", err.Error())
		return
	}

	ticket := sendResult.Ticket
	dbResult := db.SummaryIssueResult{
		Status:         "sent",
		SunatTicket:    &ticket,
		R2SignedXmlKey: &signedKey,
		MarkSent:       true,
	}
	updated, err := s.pool.ApplySummaryResult(r.Context(), summary.ID, dbResult)
	if err != nil {
		s.log.Error("apply summary result", "error", err, "summaryId", summaryID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}

	writeSuccess(w, updated)
}

// pollSummaryPipelineHandler calls SUNAT's getStatus with the stored ticket and
// writes the result back to the daily_summaries row.
func (s *server) pollSummaryPipelineHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	summaryID := chi.URLParam(r, "summaryId")
	envOverride := readPipelineEnv(r)

	summary, err := s.pool.GetDailySummary(r.Context(), companyID, summaryID)
	if err != nil {
		s.log.Error("get summary for poll", "error", err, "summaryId", summaryID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if summary == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Resumen no encontrado")
		return
	}
	if summary.SunatTicket == nil || *summary.SunatTicket == "" {
		writeError(w, http.StatusBadRequest, "NO_TICKET",
			"El resumen aún no fue enviado a SUNAT")
		return
	}

	deps, ok := s.loadPipelineDeps(w, r, companyID)
	if !ok {
		return
	}
	env := resolvePipelineEnv(envOverride, deps.company.SunatEnvironment)

	soapClient := soap.NewClient(env, s.cfg.SunatBetaURL, s.cfg.SunatProductionURL, s.cfg.SunatConsultURL, s.cfg.SunatTimeoutSeconds)
	result, err := soapClient.GetStatus(deps.sunatUsername, deps.sunatPassword, *summary.SunatTicket)
	if err != nil {
		s.log.Error("get status summary", "error", err, "summaryId", summaryID)
		writeError(w, http.StatusBadGateway, "SUNAT_ERROR", err.Error())
		return
	}

	if result.StatusCode != "0" {
		// Still processing — don't touch the row, just echo the status.
		writeSuccess(w, map[string]string{"statusCode": result.StatusCode})
		return
	}

	parsedCDR, err := cdr.Parse(result.Content)
	if err != nil {
		s.log.Error("parse summary cdr", "error", err, "summaryId", summaryID)
		writeError(w, http.StatusInternalServerError, "CDR_PARSE_ERROR", err.Error())
		return
	}

	cdrKey := r2.DocumentKey(deps.tenantID, companyID, summaryID, r2.FileCDR)
	if err := s.r2.UploadDocumentFile(r.Context(), cdrKey, r2.FileCDR, result.Content); err != nil {
		s.log.Error("upload summary cdr", "error", err, "key", cdrKey)
		writeError(w, http.StatusInternalServerError, "R2_UPLOAD_ERROR", err.Error())
		return
	}

	status := "rejected"
	if parsedCDR.Accepted {
		if len(parsedCDR.Notes) > 0 {
			status = "accepted_with_observations"
		} else {
			status = "accepted"
		}
	}
	dbResult := db.SummaryIssueResult{
		Status:                   status,
		SunatResponseCode:        &parsedCDR.ResponseCode,
		SunatResponseDescription: &parsedCDR.Description,
		R2CdrKey:                 &cdrKey,
		MarkAccepted:             parsedCDR.Accepted,
	}
	updated, err := s.pool.ApplySummaryResult(r.Context(), summary.ID, dbResult)
	if err != nil {
		s.log.Error("apply summary poll result", "error", err, "summaryId", summaryID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccess(w, updated)
}

// issueVoidPipelineHandler signs a Comunicacion de Baja (RA) and sends it to
// SUNAT, storing the returned ticket on the voided_documents row.
func (s *server) issueVoidPipelineHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	voidID := chi.URLParam(r, "voidId")
	envOverride := readPipelineEnv(r)

	voidDoc, err := s.pool.GetVoidedDocument(r.Context(), companyID, voidID)
	if err != nil {
		s.log.Error("get void", "error", err, "voidId", voidID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if voidDoc == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Comunicación de baja no encontrada")
		return
	}
	if voidDoc.Status == "accepted" || voidDoc.Status == "accepted_with_observations" {
		writeError(w, http.StatusBadRequest, "ALREADY_ACCEPTED",
			"La comunicación ya fue aceptada por SUNAT")
		return
	}

	items, err := s.pool.GetVoidedDocumentItems(r.Context(), voidDoc.ID)
	if err != nil {
		s.log.Error("get void items", "error", err, "voidId", voidID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if len(items) == 0 {
		writeError(w, http.StatusBadRequest, "NO_ITEMS",
			"La comunicación no tiene documentos enlazados")
		return
	}

	deps, ok := s.loadPipelineDeps(w, r, companyID)
	if !ok {
		return
	}
	env := resolvePipelineEnv(envOverride, deps.company.SunatEnvironment)

	correlative := trailingDigits(voidDoc.VoidID)
	voidReq := model.VoidRequest{
		SupplierRUC:  deps.company.RUC,
		SupplierName: deps.company.CompanyName,
		IssueDate:    voidDoc.VoidDate.Format("2006-01-02"),
		Correlative:  correlative,
		Environment:  env,
	}
	for i, it := range items {
		voidReq.Items = append(voidReq.Items, model.VoidItem{
			LineNumber:  i + 1,
			DocType:     it.DocType,
			Series:      it.Series,
			Correlative: it.Correlative,
			VoidReason:  it.Reason,
		})
	}

	xmlBytes, err := xmlbuilder.BuildVoidedXML(voidReq)
	if err != nil {
		s.log.Error("build voided xml", "error", err, "voidId", voidID)
		writeError(w, http.StatusInternalServerError, "XML_BUILD_ERROR", err.Error())
		return
	}

	signedXML, err := signature.SignXML(xmlBytes, deps.parsedCert.PrivateKeyPEM, deps.parsedCert.CertPEM)
	if err != nil {
		s.log.Error("sign voided xml", "error", err, "voidId", voidID)
		writeError(w, http.StatusInternalServerError, "SIGN_ERROR", err.Error())
		return
	}

	filename := xmlbuilder.VoidFilename(voidReq.SupplierRUC, voidReq.IssueDate, voidReq.Correlative)
	zipBytes, err := zipper.CreateZIP(filename, signedXML)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ZIP_ERROR", err.Error())
		return
	}

	soapClient := soap.NewClient(env, s.cfg.SunatBetaURL, s.cfg.SunatProductionURL, s.cfg.SunatConsultURL, s.cfg.SunatTimeoutSeconds)
	sendResult, err := soapClient.SendSummary(deps.sunatUsername, deps.sunatPassword, filename, zipBytes)
	if err != nil {
		s.log.Error("send void", "error", err, "voidId", voidID)
		writeError(w, http.StatusBadGateway, "SUNAT_ERROR", err.Error())
		return
	}

	signedKey := r2.DocumentKey(deps.tenantID, companyID, voidDoc.ID, r2.FileSignedXML)
	if err := s.r2.UploadDocumentFile(r.Context(), signedKey, r2.FileSignedXML, signedXML); err != nil {
		s.log.Error("upload void signed xml", "error", err, "key", signedKey)
		writeError(w, http.StatusInternalServerError, "R2_UPLOAD_ERROR", err.Error())
		return
	}

	ticket := sendResult.Ticket
	dbResult := db.VoidIssueResult{
		Status:         "sent",
		SunatTicket:    &ticket,
		R2SignedXmlKey: &signedKey,
		MarkSent:       true,
	}
	updated, err := s.pool.ApplyVoidResult(r.Context(), voidDoc.ID, dbResult)
	if err != nil {
		s.log.Error("apply void result", "error", err, "voidId", voidID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccess(w, updated)
}

// pollVoidPipelineHandler calls SUNAT's getStatus with the stored ticket and
// writes the result back to the voided_documents row.
func (s *server) pollVoidPipelineHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	voidID := chi.URLParam(r, "voidId")
	envOverride := readPipelineEnv(r)

	voidDoc, err := s.pool.GetVoidedDocument(r.Context(), companyID, voidID)
	if err != nil {
		s.log.Error("get void for poll", "error", err, "voidId", voidID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if voidDoc == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Comunicación de baja no encontrada")
		return
	}
	if voidDoc.SunatTicket == nil || *voidDoc.SunatTicket == "" {
		writeError(w, http.StatusBadRequest, "NO_TICKET",
			"La comunicación aún no fue enviada a SUNAT")
		return
	}

	deps, ok := s.loadPipelineDeps(w, r, companyID)
	if !ok {
		return
	}
	env := resolvePipelineEnv(envOverride, deps.company.SunatEnvironment)

	soapClient := soap.NewClient(env, s.cfg.SunatBetaURL, s.cfg.SunatProductionURL, s.cfg.SunatConsultURL, s.cfg.SunatTimeoutSeconds)
	result, err := soapClient.GetStatus(deps.sunatUsername, deps.sunatPassword, *voidDoc.SunatTicket)
	if err != nil {
		s.log.Error("get status void", "error", err, "voidId", voidID)
		writeError(w, http.StatusBadGateway, "SUNAT_ERROR", err.Error())
		return
	}

	if result.StatusCode != "0" {
		writeSuccess(w, map[string]string{"statusCode": result.StatusCode})
		return
	}

	parsedCDR, err := cdr.Parse(result.Content)
	if err != nil {
		s.log.Error("parse void cdr", "error", err, "voidId", voidID)
		writeError(w, http.StatusInternalServerError, "CDR_PARSE_ERROR", err.Error())
		return
	}

	cdrKey := r2.DocumentKey(deps.tenantID, companyID, voidDoc.ID, r2.FileCDR)
	if err := s.r2.UploadDocumentFile(r.Context(), cdrKey, r2.FileCDR, result.Content); err != nil {
		s.log.Error("upload void cdr", "error", err, "key", cdrKey)
		writeError(w, http.StatusInternalServerError, "R2_UPLOAD_ERROR", err.Error())
		return
	}

	status := "rejected"
	if parsedCDR.Accepted {
		if len(parsedCDR.Notes) > 0 {
			status = "accepted_with_observations"
		} else {
			status = "accepted"
		}
	}
	dbResult := db.VoidIssueResult{
		Status:                   status,
		SunatResponseCode:        &parsedCDR.ResponseCode,
		SunatResponseDescription: &parsedCDR.Description,
		R2CdrKey:                 &cdrKey,
		MarkAccepted:             parsedCDR.Accepted,
	}
	updated, err := s.pool.ApplyVoidResult(r.Context(), voidDoc.ID, dbResult)
	if err != nil {
		s.log.Error("apply void poll result", "error", err, "voidId", voidID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccess(w, updated)
}
