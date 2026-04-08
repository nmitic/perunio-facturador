package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/perunio/perunio-facturador/internal/cdr"
	facturadorCrypto "github.com/perunio/perunio-facturador/internal/crypto"
	"github.com/perunio/perunio-facturador/internal/model"
	"github.com/perunio/perunio-facturador/internal/pdf"
	"github.com/perunio/perunio-facturador/internal/signature"
	"github.com/perunio/perunio-facturador/internal/soap"
	"github.com/perunio/perunio-facturador/internal/validation"
	"github.com/perunio/perunio-facturador/internal/xmlbuilder"
	"github.com/perunio/perunio-facturador/internal/zipper"
)

func (s *server) issueDocumentHandler(w http.ResponseWriter, r *http.Request) {
	var req model.IssueRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, model.IssueResponse{ErrorCode: "BAD_REQUEST", ErrorMessage: err.Error()})
		return
	}

	// Validate
	if errs := validation.Validate(req); len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, model.IssueResponse{
			ErrorCode:    "VALIDATION_ERROR",
			ErrorMessage: fmt.Sprintf("%d validation errors", len(errs)),
			Observations: validationToObservations(errs),
		})
		return
	}

	// Decrypt certificate password
	certPassword, err := facturadorCrypto.DecryptAES256GCM(req.CertificatePassword, s.cfg.EncryptionKey)
	if err != nil {
		s.log.Error("decrypt certificate password", "error", err)
		writeJSON(w, http.StatusBadRequest, model.IssueResponse{ErrorCode: "CERT_DECRYPT_ERROR", ErrorMessage: "failed to decrypt certificate password"})
		return
	}

	// Load certificate
	parsed, err := signature.LoadCertificateFromURL(req.CertificateURL, certPassword)
	if err != nil {
		s.log.Error("load certificate", "error", err)
		writeJSON(w, http.StatusBadRequest, model.IssueResponse{ErrorCode: "CERT_LOAD_ERROR", ErrorMessage: "failed to load certificate"})
		return
	}

	// Build XML
	xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
	if err != nil {
		s.log.Error("build XML", "error", err)
		writeJSON(w, http.StatusInternalServerError, model.IssueResponse{ErrorCode: "XML_BUILD_ERROR", ErrorMessage: err.Error()})
		return
	}

	// Sign XML
	signedXML, err := signature.SignXML(xmlBytes, parsed.Certificate, parsed.PrivateKey)
	if err != nil {
		s.log.Error("sign XML", "error", err)
		writeJSON(w, http.StatusInternalServerError, model.IssueResponse{ErrorCode: "SIGN_ERROR", ErrorMessage: err.Error()})
		return
	}

	// Build filename
	filename := xmlbuilder.Filename(req.SupplierRUC, req.DocType, req.Series, req.Correlative)

	// Create ZIP
	zipBytes, err := zipper.CreateZIP(filename, signedXML)
	if err != nil {
		s.log.Error("create ZIP", "error", err)
		writeJSON(w, http.StatusInternalServerError, model.IssueResponse{ErrorCode: "ZIP_ERROR", ErrorMessage: err.Error()})
		return
	}

	// Send to SUNAT
	soapClient := soap.NewClient(req.Environment, s.cfg.SunatBetaURL, s.cfg.SunatProductionURL, s.cfg.SunatConsultURL, s.cfg.SunatTimeoutSeconds)
	result, err := soapClient.SendBill(req.SunatUsername, req.SunatPassword, filename, zipBytes)
	if err != nil {
		s.log.Error("send to SUNAT", "error", err)
		writeJSON(w, http.StatusBadGateway, model.IssueResponse{ErrorCode: "SUNAT_ERROR", ErrorMessage: err.Error()})
		return
	}

	// Parse CDR
	parsedCDR, err := cdr.Parse(result.ApplicationResponse)
	if err != nil {
		s.log.Error("parse CDR", "error", err)
		writeJSON(w, http.StatusInternalServerError, model.IssueResponse{
			ErrorCode: "CDR_PARSE_ERROR", ErrorMessage: err.Error(),
			SignedXML: signedXML, ZipBytes: zipBytes, CDRBytes: result.ApplicationResponse,
		})
		return
	}

	// Build QR data
	digestVal, _ := signature.DigestValue(signedXML)
	qrData := fmt.Sprintf("%s|%s|%s|%08d|%s|%s|%s|%s|%s|%s|",
		req.SupplierRUC, req.DocType, req.Series, req.Correlative,
		req.TotalIGV, req.TotalAmount, req.IssueDate,
		req.CustomerDocType, req.CustomerDocNumber, digestVal)

	// Generate PDF
	pdfBytes, err := pdf.Generate(req, qrData)
	if err != nil {
		s.log.Error("generate PDF", "error", err)
		// Non-fatal — return other artifacts without PDF
		pdfBytes = nil
	}

	// Build observations
	var observations []model.Observation
	for _, note := range parsedCDR.Notes {
		observations = append(observations, model.Observation{Message: note})
	}

	resp := model.IssueResponse{
		Success:      parsedCDR.Accepted,
		SignedXML:    signedXML,
		ZipBytes:     zipBytes,
		CDRBytes:     result.ApplicationResponse,
		PDFBytes:     pdfBytes,
		ResponseCode: parsedCDR.ResponseCode,
		Description:  parsedCDR.Description,
		Observations: observations,
		QRData:       qrData,
		Filename:     filename,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *server) validateDocumentHandler(w http.ResponseWriter, r *http.Request) {
	var req model.IssueRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, model.ValidateResponse{Errors: []model.ValidationError{{Message: err.Error()}}})
		return
	}

	errs := validation.Validate(req)
	writeJSON(w, http.StatusOK, model.ValidateResponse{
		Valid:  len(errs) == 0,
		Errors: errs,
	})
}

func (s *server) queryCDRHandler(w http.ResponseWriter, r *http.Request) {
	var req model.CDRQueryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, model.CDRQueryResponse{ErrorCode: "BAD_REQUEST", ErrorMessage: err.Error()})
		return
	}

	soapClient := soap.NewClient(req.Environment, s.cfg.SunatBetaURL, s.cfg.SunatProductionURL, s.cfg.SunatConsultURL, s.cfg.SunatTimeoutSeconds)
	result, err := soapClient.GetStatusCdr(req.SunatUsername, req.SunatPassword, req.SupplierRUC, req.DocType, req.Series, req.Correlative)
	if err != nil {
		s.log.Error("query CDR", "error", err)
		writeJSON(w, http.StatusBadGateway, model.CDRQueryResponse{ErrorCode: "SUNAT_ERROR", ErrorMessage: err.Error()})
		return
	}

	resp := model.CDRQueryResponse{
		Success:  true,
		CDRBytes: result.Content,
	}

	if result.Content != nil {
		parsedCDR, err := cdr.Parse(result.Content)
		if err == nil {
			resp.ResponseCode = parsedCDR.ResponseCode
			resp.Description = parsedCDR.Description
			for _, note := range parsedCDR.Notes {
				resp.Observations = append(resp.Observations, model.Observation{Message: note})
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *server) issueSummaryHandler(w http.ResponseWriter, r *http.Request) {
	var req model.SummaryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, model.SummaryResponse{ErrorCode: "BAD_REQUEST", ErrorMessage: err.Error()})
		return
	}

	// Decrypt certificate password
	certPassword, err := facturadorCrypto.DecryptAES256GCM(req.CertificatePassword, s.cfg.EncryptionKey)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.SummaryResponse{ErrorCode: "CERT_DECRYPT_ERROR", ErrorMessage: "failed to decrypt certificate password"})
		return
	}

	// Load certificate
	parsed, err := signature.LoadCertificateFromURL(req.CertificateURL, certPassword)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.SummaryResponse{ErrorCode: "CERT_LOAD_ERROR", ErrorMessage: "failed to load certificate"})
		return
	}

	// Build XML (UBL 2.0!)
	xmlBytes, err := xmlbuilder.BuildSummaryXML(req)
	if err != nil {
		s.log.Error("build summary XML", "error", err)
		writeJSON(w, http.StatusInternalServerError, model.SummaryResponse{ErrorCode: "XML_BUILD_ERROR", ErrorMessage: err.Error()})
		return
	}

	// Sign
	signedXML, err := signature.SignXML(xmlBytes, parsed.Certificate, parsed.PrivateKey)
	if err != nil {
		s.log.Error("sign summary XML", "error", err)
		writeJSON(w, http.StatusInternalServerError, model.SummaryResponse{ErrorCode: "SIGN_ERROR", ErrorMessage: err.Error()})
		return
	}

	filename := xmlbuilder.SummaryFilename(req.SupplierRUC, req.IssueDate, req.Correlative)

	// ZIP
	zipBytes, err := zipper.CreateZIP(filename, signedXML)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.SummaryResponse{ErrorCode: "ZIP_ERROR", ErrorMessage: err.Error()})
		return
	}

	// Send to SUNAT (async — returns ticket)
	soapClient := soap.NewClient(req.Environment, s.cfg.SunatBetaURL, s.cfg.SunatProductionURL, s.cfg.SunatConsultURL, s.cfg.SunatTimeoutSeconds)
	result, err := soapClient.SendSummary(req.SunatUsername, req.SunatPassword, filename, zipBytes)
	if err != nil {
		s.log.Error("send summary to SUNAT", "error", err)
		writeJSON(w, http.StatusBadGateway, model.SummaryResponse{ErrorCode: "SUNAT_ERROR", ErrorMessage: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, model.SummaryResponse{
		Success:   true,
		Ticket:    result.Ticket,
		Filename:  filename,
		SignedXML: signedXML,
		ZipBytes:  zipBytes,
	})
}

func (s *server) summaryStatusHandler(w http.ResponseWriter, r *http.Request) {
	var req model.TicketStatusRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, model.TicketStatusResponse{ErrorCode: "BAD_REQUEST", ErrorMessage: err.Error()})
		return
	}

	soapClient := soap.NewClient(req.Environment, s.cfg.SunatBetaURL, s.cfg.SunatProductionURL, s.cfg.SunatConsultURL, s.cfg.SunatTimeoutSeconds)
	result, err := soapClient.GetStatus(req.SunatUsername, req.SunatPassword, req.Ticket)
	if err != nil {
		s.log.Error("poll ticket", "error", err)
		writeJSON(w, http.StatusBadGateway, model.TicketStatusResponse{ErrorCode: "SUNAT_ERROR", ErrorMessage: err.Error()})
		return
	}

	resp := model.TicketStatusResponse{
		Success:    true,
		StatusCode: result.StatusCode,
		CDRBytes:   result.Content,
	}

	if result.Content != nil {
		parsedCDR, err := cdr.Parse(result.Content)
		if err == nil {
			resp.ResponseCode = parsedCDR.ResponseCode
			resp.Description = parsedCDR.Description
			for _, note := range parsedCDR.Notes {
				resp.Observations = append(resp.Observations, model.Observation{Message: note})
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *server) issueVoidHandler(w http.ResponseWriter, r *http.Request) {
	var req model.VoidRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, model.SummaryResponse{ErrorCode: "BAD_REQUEST", ErrorMessage: err.Error()})
		return
	}

	certPassword, err := facturadorCrypto.DecryptAES256GCM(req.CertificatePassword, s.cfg.EncryptionKey)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.SummaryResponse{ErrorCode: "CERT_DECRYPT_ERROR", ErrorMessage: "failed to decrypt certificate password"})
		return
	}

	parsed, err := signature.LoadCertificateFromURL(req.CertificateURL, certPassword)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.SummaryResponse{ErrorCode: "CERT_LOAD_ERROR", ErrorMessage: "failed to load certificate"})
		return
	}

	xmlBytes, err := xmlbuilder.BuildVoidedXML(req)
	if err != nil {
		s.log.Error("build voided XML", "error", err)
		writeJSON(w, http.StatusInternalServerError, model.SummaryResponse{ErrorCode: "XML_BUILD_ERROR", ErrorMessage: err.Error()})
		return
	}

	signedXML, err := signature.SignXML(xmlBytes, parsed.Certificate, parsed.PrivateKey)
	if err != nil {
		s.log.Error("sign voided XML", "error", err)
		writeJSON(w, http.StatusInternalServerError, model.SummaryResponse{ErrorCode: "SIGN_ERROR", ErrorMessage: err.Error()})
		return
	}

	filename := xmlbuilder.VoidFilename(req.SupplierRUC, req.IssueDate, req.Correlative)

	zipBytes, err := zipper.CreateZIP(filename, signedXML)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.SummaryResponse{ErrorCode: "ZIP_ERROR", ErrorMessage: err.Error()})
		return
	}

	soapClient := soap.NewClient(req.Environment, s.cfg.SunatBetaURL, s.cfg.SunatProductionURL, s.cfg.SunatConsultURL, s.cfg.SunatTimeoutSeconds)
	result, err := soapClient.SendSummary(req.SunatUsername, req.SunatPassword, filename, zipBytes)
	if err != nil {
		s.log.Error("send void to SUNAT", "error", err)
		writeJSON(w, http.StatusBadGateway, model.SummaryResponse{ErrorCode: "SUNAT_ERROR", ErrorMessage: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, model.SummaryResponse{
		Success:   true,
		Ticket:    result.Ticket,
		Filename:  filename,
		SignedXML: signedXML,
		ZipBytes:  zipBytes,
	})
}

func (s *server) voidStatusHandler(w http.ResponseWriter, r *http.Request) {
	// Reuse summary status handler — same ticket polling mechanism
	s.summaryStatusHandler(w, r)
}

func (s *server) validateCertificateHandler(w http.ResponseWriter, r *http.Request) {
	var req model.CertificateValidateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, model.CertificateValidateResponse{ErrorCode: "BAD_REQUEST", ErrorMessage: err.Error()})
		return
	}

	certPassword, err := facturadorCrypto.DecryptAES256GCM(req.CertificatePassword, s.cfg.EncryptionKey)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.CertificateValidateResponse{ErrorCode: "CERT_DECRYPT_ERROR", ErrorMessage: "failed to decrypt certificate password"})
		return
	}

	// Download certificate
	certResp, err := http.Get(req.CertificateURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.CertificateValidateResponse{ErrorCode: "CERT_DOWNLOAD_ERROR", ErrorMessage: "failed to download certificate"})
		return
	}
	defer certResp.Body.Close()

	pfxData, err := io.ReadAll(certResp.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.CertificateValidateResponse{ErrorCode: "CERT_READ_ERROR", ErrorMessage: "failed to read certificate"})
		return
	}

	info, err := signature.CertificateMetadata(pfxData, certPassword)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.CertificateValidateResponse{ErrorCode: "CERT_PARSE_ERROR", ErrorMessage: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, model.CertificateValidateResponse{
		Valid: !info.IsExpired,
		Info:  info,
	})
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func validationToObservations(errs []model.ValidationError) []model.Observation {
	obs := make([]model.Observation, len(errs))
	for i, e := range errs {
		obs[i] = model.Observation{
			Code:    fmt.Sprint(e.Code),
			Message: e.Message,
		}
	}
	return obs
}
