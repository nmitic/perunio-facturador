package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/perunio/perunio-facturador/internal/auth"
	"github.com/perunio/perunio-facturador/internal/cdr"
	facturadorCrypto "github.com/perunio/perunio-facturador/internal/crypto"
	"github.com/perunio/perunio-facturador/internal/db"
	"github.com/perunio/perunio-facturador/internal/model"
	"github.com/perunio/perunio-facturador/internal/r2"
	"github.com/perunio/perunio-facturador/internal/signature"
	"github.com/perunio/perunio-facturador/internal/soap"
	"github.com/perunio/perunio-facturador/internal/validation"
	"github.com/perunio/perunio-facturador/internal/xmlbuilder"
	"github.com/perunio/perunio-facturador/internal/zipper"
)

// pipelineError is a failure from a pipeline core function. It carries both the
// HTTP projection (status/code/message, for handlers) and the classification the
// scheduler needs to decide whether retrying can possibly help.
//
// The core functions never touch http.ResponseWriter — that is what lets the
// background scheduler reuse the exact same emission path as the HTTP handlers.
type pipelineError struct {
	status  int
	code    string
	message string
	// retryable marks failures that a later attempt might survive: network
	// blips, SUNAT 5xx, transient DB errors. A SUNAT rejection or a validation
	// failure is never retryable — the outcome is deterministic, and for a
	// rejection the correlative has already been consumed.
	retryable bool
	// validationErrors is populated only for VALIDATION_ERROR, whose response
	// shape differs from the standard error envelope.
	validationErrors []model.ValidationError
}

func (e *pipelineError) Error() string {
	return e.code + ": " + e.message
}

func newPipelineError(status int, code, message string) *pipelineError {
	return &pipelineError{status: status, code: code, message: message}
}

func newRetryablePipelineError(status int, code, message string) *pipelineError {
	return &pipelineError{status: status, code: code, message: message, retryable: true}
}

// writePipelineError projects a pipelineError onto the HTTP response, preserving
// the exact envelopes the handlers used before the core was extracted.
func writePipelineError(w http.ResponseWriter, e *pipelineError) {
	if e.code == "VALIDATION_ERROR" && e.validationErrors != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success":          false,
			"code":             "VALIDATION_ERROR",
			"error":            e.message,
			"validationErrors": e.validationErrors,
		})
		return
	}
	writeError(w, e.status, e.code, e.message)
}

// resolvePipelineDeps performs the three lookups every pipeline entry point needs:
//  1. Tenant ID from the context (set by the JWT middleware, or synthesized by the
//     scheduler for background runs)
//  2. Company row + decrypted SUNAT SOL password
//  3. Active certificate (PFX fetched from R2, password decrypted, PKCS#12 parsed)
func (s *Server) resolvePipelineDeps(ctx context.Context, companyID string) (*pipelineDeps, *pipelineError) {
	tenantID, ok := auth.TenantIDFromContext(ctx)
	if !ok {
		return nil, newPipelineError(http.StatusUnauthorized, "UNAUTHORIZED", "No autenticado")
	}

	company, err := s.pool.GetCompany(ctx, companyID)
	if err != nil {
		s.log.Error("get company", "error", err, "companyId", companyID)
		return nil, newRetryablePipelineError(http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
	}
	if company == nil {
		return nil, newPipelineError(http.StatusNotFound, "COMPANY_NOT_FOUND", "Empresa no encontrada")
	}
	// Emission requires the dedicated SUNAT SOL secondary-user credential (with the
	// emisor-electrónico profile). We do NOT fall back to the download credential
	// (companies.username/password): that user often lacks the emission profile, which is
	// exactly what SUNAT rejects with fault 0111. The UI gates on this too; this is the
	// server-side enforcement covering every emit path (comprobantes, GRE, scheduler).
	if company.EmissionUsername == nil || *company.EmissionUsername == "" ||
		company.EmissionPassword == nil || *company.EmissionPassword == "" {
		return nil, newPipelineError(http.StatusBadRequest, "EMISSION_CREDENTIALS_MISSING",
			"Configura el usuario secundario SOL de emisión para esta empresa")
	}

	sunatPassword, err := facturadorCrypto.DecryptAES256GCM(*company.EmissionPassword, s.cfg.EncryptionKey)
	if err != nil {
		ivHex := strings.SplitN(*company.EmissionPassword, ":", 2)[0]
		s.log.Error("decrypt emission sunat password", "error", err, "companyId", companyID, "ivHexLen", len(ivHex))
		return nil, newPipelineError(http.StatusInternalServerError, "SUNAT_DECRYPT_ERROR",
			"No se pudo descifrar la contraseña SOL de emisión")
	}

	activeCert, err := s.pool.GetActiveCertificateForSigning(ctx, companyID)
	if err != nil {
		s.log.Error("get active certificate", "error", err, "companyId", companyID)
		return nil, newRetryablePipelineError(http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
	}
	if activeCert == nil {
		return nil, newPipelineError(http.StatusBadRequest, "CERTIFICATE_MISSING",
			"No hay un certificado activo para esta empresa")
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
		return nil, newPipelineError(http.StatusInternalServerError, "CERT_LOAD_ERROR",
			"No se pudo cargar el certificado activo")
	}

	// SUNAT WS-Security UsernameToken: {RUC}{usuario_sol} concatenated, no
	// separator. companies.emission_username stores the SOL secondary user used for
	// emission.
	return &pipelineDeps{
		tenantID:      tenantID,
		company:       company,
		sunatUsername: company.RUC + *company.EmissionUsername,
		sunatPassword: sunatPassword,
		parsedCert:    parsed,
	}, nil
}

// createDocumentDraft inserts a draft issued_document after checking the tenant's
// monthly quota, then bumps the issued-document counter.
//
// Shared by the HTTP create handler and the scheduler, so a scheduled comprobante
// gets the same quota enforcement and correlative reservation as a manual one.
func (s *Server) createDocumentDraft(ctx context.Context, companyID string, in db.CreateDocumentInput) (*model.IssuedDocument, *pipelineError) {
	tenantID, ok := auth.TenantIDFromContext(ctx)
	if !ok {
		return nil, newPipelineError(http.StatusUnauthorized, "UNAUTHORIZED", "No autenticado")
	}

	quota, err := s.pool.CheckDocumentQuota(ctx, tenantID, 1)
	if err != nil {
		s.log.Error("check document quota", "error", err, "tenantId", tenantID)
		return nil, newRetryablePipelineError(http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
	}
	if !quota.Allowed {
		limitStr := "ilimitado"
		if quota.Limit != nil {
			limitStr = strconv.Itoa(*quota.Limit)
		}
		return nil, newPipelineError(http.StatusForbidden, "DOCUMENT_QUOTA_EXCEEDED",
			"Límite mensual de documentos alcanzado ("+limitStr+")")
	}

	doc, err := s.pool.CreateDocumentWithItems(ctx, companyID, in)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrSeriesInactive):
			return nil, newPipelineError(http.StatusBadRequest, "SERIES_NOT_FOUND", "Serie no encontrada o inactiva")
		case errors.Is(err, db.ErrDuplicate):
			return nil, newPipelineError(http.StatusConflict, "DOCUMENT_DUPLICATE", "Documento duplicado")
		default:
			s.log.Error("create document", "error", err, "companyId", companyID)
			return nil, newRetryablePipelineError(http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		}
	}

	if err := s.pool.IncrementDocumentUsage(ctx, 1); err != nil {
		// Usage is best-effort at this point — the draft already exists. Log so we
		// can investigate drift.
		s.log.Error("increment document usage", "error", err, "tenantId", tenantID)
	}

	return doc, nil
}

// runIssuePipeline runs the full issue pipeline against a draft issued_documents
// row: load -> deps -> validate -> build XML -> sign -> zip -> sendBill -> parse
// CDR -> upload artifacts to R2 -> persist the outcome.
//
// Returns the updated document. Note that a SUNAT *rejection* is a successful run
// of the pipeline, not an error: the returned document carries status "rejected"
// and the CDR's response code. Only failures that prevented a verdict come back as
// a pipelineError.
func (s *Server) runIssuePipeline(ctx context.Context, companyID, docID, envOverride string) (*model.IssuedDocument, *pipelineError) {
	// 1. Load the draft document + items.
	doc, err := s.pool.GetIssuedDocument(ctx, companyID, docID)
	if err != nil {
		s.log.Error("get document for issue", "error", err, "docId", docID)
		return nil, newRetryablePipelineError(http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
	}
	if doc == nil {
		return nil, newPipelineError(http.StatusNotFound, "NOT_FOUND", "Documento no encontrado")
	}
	if doc.Status == "accepted" || doc.Status == "accepted_with_observations" {
		return nil, newPipelineError(http.StatusBadRequest, "ALREADY_ACCEPTED",
			"El documento ya fue aceptado por SUNAT")
	}

	items, err := s.pool.GetIssuedDocumentItems(ctx, docID)
	if err != nil {
		s.log.Error("get document items for issue", "error", err, "docId", docID)
		return nil, newRetryablePipelineError(http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
	}
	if len(items) == 0 {
		return nil, newPipelineError(http.StatusBadRequest, "NO_ITEMS", "El documento no tiene líneas")
	}

	// 2. Load company + SUNAT creds + active certificate.
	deps, pErr := s.resolvePipelineDeps(ctx, companyID)
	if pErr != nil {
		return nil, pErr
	}
	env := resolvePipelineEnv(envOverride, deps.company.SunatEnvironment)

	// 3. Fiscal address from the company row (populated during onboarding from the
	// SUNAT RUC scrape), falling back to a placeholder when missing so the pipeline
	// still runs and SUNAT doesn't reject the invoice.
	address := deps.company.FiscalAddress
	if address == "" {
		address = "-"
	}

	// 4. Build the IssueRequest and run the compliance pipeline.
	issueReq := buildIssueRequestFromDoc(doc, items, deps.company.RUC, deps.company.CompanyName, address, deps.company.CuentaDetraccion)
	issueReq.Environment = env

	// Pre-submission validation — fail fast before calling SUNAT.
	if vErrs := validation.Validate(issueReq); len(vErrs) > 0 {
		return nil, &pipelineError{
			status:           http.StatusBadRequest,
			code:             "VALIDATION_ERROR",
			message:          "Documento no pasa validación pre-SUNAT",
			validationErrors: vErrs,
		}
	}

	xmlBytes, err := xmlbuilder.BuildDocumentXML(issueReq)
	if err != nil {
		s.log.Error("build document xml", "error", err, "docId", docID)
		return nil, newPipelineError(http.StatusInternalServerError, "XML_BUILD_ERROR", err.Error())
	}

	signedXML, err := signature.SignXML(xmlBytes, deps.parsedCert.PrivateKeyPEM, deps.parsedCert.CertPEM)
	if err != nil {
		s.log.Error("sign document xml", "error", err, "docId", docID)
		return nil, newPipelineError(http.StatusInternalServerError, "SIGN_ERROR", err.Error())
	}

	filename := xmlbuilder.Filename(issueReq.SupplierRUC, issueReq.DocType, issueReq.Series, issueReq.Correlative)
	zipBytes, err := zipper.CreateZIP(filename, signedXML)
	if err != nil {
		s.log.Error("create zip", "error", err, "docId", docID)
		return nil, newPipelineError(http.StatusInternalServerError, "ZIP_ERROR", err.Error())
	}

	soapClient := soap.NewClient(env, s.cfg.SunatBetaURL, s.cfg.SunatProductionURL, s.cfg.SunatConsultURL, s.cfg.SunatTimeoutSeconds)
	sendStart := time.Now()
	sendResult, err := soapClient.SendBill(deps.sunatUsername, deps.sunatPassword, filename, zipBytes)
	sendDurationMs := int(time.Since(sendStart).Milliseconds())
	if err != nil {
		s.log.Error("send bill", "error", err, "docId", docID)
		docRef := docID
		errDesc := err.Error()
		if logErr := s.pool.InsertSubmissionLog(ctx, db.SubmissionLogEntry{
			CompanyID: companyID, DocumentID: &docRef, Action: "sendBill",
			ResponseDescription: &errDesc, DurationMs: &sendDurationMs,
		}); logErr != nil {
			s.log.Warn("write submission log", "error", logErr, "docId", docID)
		}
		// The transport failed, so SUNAT never returned a verdict — a later
		// attempt can still succeed.
		return nil, newRetryablePipelineError(http.StatusBadGateway, "SUNAT_ERROR", err.Error())
	}

	parsedCDR, err := cdr.Parse(sendResult.ApplicationResponse)
	if err != nil {
		s.log.Error("parse cdr", "error", err, "docId", docID)
		return nil, newPipelineError(http.StatusInternalServerError, "CDR_PARSE_ERROR", err.Error())
	}

	{
		docRef := docID
		resCode := parsedCDR.ResponseCode
		resDesc := parsedCDR.Description
		if logErr := s.pool.InsertSubmissionLog(ctx, db.SubmissionLogEntry{
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

	// Build QR data (rendered client-side from this string when the user downloads the PDF).
	digestVal, _ := signature.DigestValue(signedXML)
	qrData := fmt.Sprintf("%s|%s|%s|%08d|%s|%s|%s|%s|%s|%s|",
		issueReq.SupplierRUC, issueReq.DocType, issueReq.Series, issueReq.Correlative,
		issueReq.TotalIGV, issueReq.TotalAmount, issueReq.IssueDate,
		issueReq.CustomerDocType, issueReq.CustomerDocNumber, digestVal)

	// 5. Upload artifacts to R2 under the canonical documents/{tenantId}/... layout.
	// Failures here are fatal — the DB row must not claim R2 keys that don't exist.
	signedKey := r2.DocumentKey(deps.tenantID, companyID, docID, r2.FileSignedXML)
	if err := s.r2.UploadDocumentFile(ctx, signedKey, r2.FileSignedXML, signedXML); err != nil {
		s.log.Error("upload signed xml", "error", err, "key", signedKey)
		return nil, newPipelineError(http.StatusInternalServerError, "R2_UPLOAD_ERROR", err.Error())
	}
	zipKey := r2.DocumentKey(deps.tenantID, companyID, docID, r2.FileZIP)
	if err := s.r2.UploadDocumentFile(ctx, zipKey, r2.FileZIP, zipBytes); err != nil {
		s.log.Error("upload zip", "error", err, "key", zipKey)
		return nil, newPipelineError(http.StatusInternalServerError, "R2_UPLOAD_ERROR", err.Error())
	}
	cdrKey := r2.DocumentKey(deps.tenantID, companyID, docID, r2.FileCDR)
	if err := s.r2.UploadDocumentFile(ctx, cdrKey, r2.FileCDR, sendResult.ApplicationResponse); err != nil {
		s.log.Error("upload cdr", "error", err, "key", cdrKey)
		return nil, newPipelineError(http.StatusInternalServerError, "R2_UPLOAD_ERROR", err.Error())
	}

	// 6. Update the DB row with the pipeline outcome.
	observations := parseCDRNotes(parsedCDR.Notes)
	status := "rejected"
	if parsedCDR.Accepted {
		if len(observations) > 0 {
			status = "accepted_with_observations"
		} else {
			status = "accepted"
		}
	}

	dbResult := db.IssuedDocumentResult{
		Status:                   status,
		SunatResponseCode:        &parsedCDR.ResponseCode,
		SunatResponseDescription: &parsedCDR.Description,
		SunatObservations:        observations,
		R2SignedXmlKey:           &signedKey,
		R2ZipKey:                 &zipKey,
		R2CdrKey:                 &cdrKey,
		QRData:                   &qrData,
		MarkSent:                 true,
		MarkAccepted:             parsedCDR.Accepted,
	}
	updated, err := s.pool.ApplyIssueResult(ctx, docID, dbResult)
	if err != nil {
		s.log.Error("apply issue result", "error", err, "docId", docID)
		return nil, newPipelineError(http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
	}

	return updated, nil
}
