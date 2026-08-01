package http

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/perunio/perunio-facturador/internal/cdr"
	"github.com/perunio/perunio-facturador/internal/db"
	"github.com/perunio/perunio-facturador/internal/model"
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
// default wins; otherwise we fall back to "production".
func resolvePipelineEnv(override, companyDefault string) string {
	if override == "beta" || override == "production" {
		return override
	}
	if companyDefault == "beta" || companyDefault == "production" {
		return companyDefault
	}
	return "production"
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

// loadPipelineDeps is the HTTP-facing wrapper around resolvePipelineDeps: it
// writes the error response itself and reports success as a bool, which is the
// shape the summary/void/GRE handlers expect.
//
// The context-only core lives in pipeline_core.go so the background scheduler can
// share it.
func (s *Server) loadPipelineDeps(w http.ResponseWriter, r *http.Request, companyID string) (*pipelineDeps, bool) {
	deps, pErr := s.resolvePipelineDeps(r.Context(), companyID)
	if pErr != nil {
		writePipelineError(w, pErr)
		return nil, false
	}
	return deps, true
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
	supplierRUC, supplierName, supplierAddress, defaultCuentaDetraccion string,
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
		ExchangeRate:  derefString(doc.ExchangeRate, ""),
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
		GlobalDiscount:     derefString(doc.GlobalDiscount, "0.00"),
		TotalAmount:        doc.TotalAmount,
		TaxInclusiveAmount: derefString(doc.TaxInclusiveAmount, doc.TotalAmount),

		FormaPago: derefString(doc.FormaPago, "contado"),
		Cuotas:    doc.Cuotas,
		Anticipos: doc.Anticipos,
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

	// Detracción: present iff the doc row carries a código. The cuenta BN is the
	// per-document override, falling back to the company default.
	if doc.DetraccionCodigo != nil && *doc.DetraccionCodigo != "" {
		req.Detraccion = &model.Detraccion{
			Codigo:     *doc.DetraccionCodigo,
			Porcentaje: derefString(doc.DetraccionPorcentaje, "0"),
			Monto:      derefString(doc.DetraccionMonto, "0.00"),
			CuentaBN:   derefString(doc.DetraccionCuentaBN, defaultCuentaDetraccion),
		}
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

// parseCDRNotes converts raw CDR note strings (e.g. "4319 - Descripcion")
// into structured Observation values.
func parseCDRNotes(notes []string) []model.Observation {
	if len(notes) == 0 {
		return nil
	}
	obs := make([]model.Observation, 0, len(notes))
	for _, n := range notes {
		code, msg, _ := strings.Cut(n, " - ")
		if msg == "" {
			// No separator found — treat the whole string as the message.
			obs = append(obs, model.Observation{Code: "", Message: n})
		} else {
			obs = append(obs, model.Observation{Code: strings.TrimSpace(code), Message: strings.TrimSpace(msg)})
		}
	}
	return obs
}

// issueDocumentPipelineHandler runs the full issue pipeline against a draft
// issued_documents row loaded from the DB. This replaces the legacy stateless
// POST /api/v1/documents/issue endpoint.
//
// The pipeline itself lives in runIssuePipeline (pipeline_core.go) so the
// background scheduler emits through exactly the same code path; this handler is
// only param parsing and the HTTP projection of the result.
func (s *Server) issueDocumentPipelineHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	docID := chi.URLParam(r, "docId")
	envOverride := readPipelineEnv(r)

	updated, pErr := s.runIssuePipeline(r.Context(), companyID, docID, envOverride)
	if pErr != nil {
		writePipelineError(w, pErr)
		return
	}

	writeSuccess(w, updated)
}

// issueSummaryPipelineHandler signs a Resumen Diario (RC) and ships it off to
// SUNAT, storing the returned ticket on the daily_summaries row for later
// polling.
func (s *Server) issueSummaryPipelineHandler(w http.ResponseWriter, r *http.Request) {
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
			TotalGravada:      it.Subtotal,
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
		s.log.Error("send summary", "error", err, "summaryId", summaryID, "env", env, "solUser", deps.sunatUsername)
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
func (s *Server) pollSummaryPipelineHandler(w http.ResponseWriter, r *http.Request) {
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
		s.log.Error("get status summary", "error", err, "summaryId", summaryID, "env", env, "solUser", deps.sunatUsername)
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
func (s *Server) issueVoidPipelineHandler(w http.ResponseWriter, r *http.Request) {
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
		s.log.Error("send void", "error", err, "voidId", voidID, "env", env, "solUser", deps.sunatUsername)
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
func (s *Server) pollVoidPipelineHandler(w http.ResponseWriter, r *http.Request) {
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
		s.log.Error("get status void", "error", err, "voidId", voidID, "env", env, "solUser", deps.sunatUsername)
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
