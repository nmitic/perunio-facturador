package http

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/perunio/perunio-facturador/internal/auth"
	"github.com/perunio/perunio-facturador/internal/cdr"
	facturadorCrypto "github.com/perunio/perunio-facturador/internal/crypto"
	"github.com/perunio/perunio-facturador/internal/db"
	"github.com/perunio/perunio-facturador/internal/greclient"
	"github.com/perunio/perunio-facturador/internal/model"
	"github.com/perunio/perunio-facturador/internal/r2"
	"github.com/perunio/perunio-facturador/internal/signature"
	"github.com/perunio/perunio-facturador/internal/validation"
	"github.com/perunio/perunio-facturador/internal/xmlbuilder"
	"github.com/perunio/perunio-facturador/internal/zipper"
)

// greDeps is pipelineDeps + the decrypted GRE REST credentials. The
// GRE pipeline still signs with the company's PFX (same XMLDSig
// RSA-SHA1 machinery as SOAP), so we reuse the signing half of
// pipelineDeps and just layer the OAuth2 credentials on top.
type greDeps struct {
	*pipelineDeps
	greCredentials greclient.Credentials
}

// loadGREDeps wraps loadPipelineDeps with the extra lookups the GRE
// pipeline needs: decrypted client_id + client_secret from the
// companies row. Returns (nil, false) after writing the error.
func (s *server) loadGREDeps(w http.ResponseWriter, r *http.Request, companyID string) (*greDeps, bool) {
	deps, ok := s.loadPipelineDeps(w, r, companyID)
	if !ok {
		return nil, false
	}

	if deps.company.EncryptedClientID == nil || deps.company.EncryptedClientSecret == nil {
		writeError(w, http.StatusBadRequest, "GRE_CREDENTIALS_MISSING",
			"Credenciales API GRE no configuradas para esta empresa")
		return nil, false
	}

	clientID, err := facturadorCrypto.DecryptAES256GCM(*deps.company.EncryptedClientID, s.cfg.EncryptionKey)
	if err != nil {
		s.log.Error("decrypt gre client id", "error", err, "companyId", companyID)
		writeError(w, http.StatusInternalServerError, "GRE_DECRYPT_ERROR",
			"No se pudo descifrar el client_id GRE")
		return nil, false
	}
	clientSecret, err := facturadorCrypto.DecryptAES256GCM(*deps.company.EncryptedClientSecret, s.cfg.EncryptionKey)
	if err != nil {
		s.log.Error("decrypt gre client secret", "error", err, "companyId", companyID)
		writeError(w, http.StatusInternalServerError, "GRE_DECRYPT_ERROR",
			"No se pudo descifrar el client_secret GRE")
		return nil, false
	}

	return &greDeps{
		pipelineDeps: deps,
		greCredentials: greclient.Credentials{
			ClientID:      clientID,
			ClientSecret:  clientSecret,
			SunatUsername: deps.sunatUsername,
			SunatPassword: deps.sunatPassword,
		},
	}, true
}

// ---------- list + get ----------

type despatchListResponse struct {
	Success    bool                   `json:"success"`
	Data       []model.Despatch       `json:"data"`
	Pagination documentListPagination `json:"pagination"`
}

func (s *server) listDespatchesHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	filter := db.DespatchListFilter{
		DocType: q.Get("docType"),
		Status:  q.Get("status"),
		Page:    page,
		Limit:   limit,
	}

	result, err := s.pool.ListDespatches(r.Context(), companyID, filter)
	if err != nil {
		s.log.Error("list despatches", "error", err, "companyId", companyID)
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
	if result.Despatches == nil {
		result.Despatches = []model.Despatch{}
	}

	writeJSON(w, http.StatusOK, despatchListResponse{
		Success: true,
		Data:    result.Despatches,
		Pagination: documentListPagination{
			Page: filter.Page, Limit: filter.Limit,
			Total: result.Total, TotalPages: totalPages,
		},
	})
}

type despatchDetailResponse struct {
	model.Despatch
	Lines []model.DespatchLine `json:"lines"`
}

func (s *server) getDespatchHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	despatchID := chi.URLParam(r, "despatchId")

	d, err := s.pool.GetDespatch(r.Context(), companyID, despatchID)
	if err != nil {
		s.log.Error("get despatch", "error", err, "despatchId", despatchID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Guía no encontrada")
		return
	}

	lines, err := s.pool.GetDespatchLines(r.Context(), despatchID)
	if err != nil {
		s.log.Error("get despatch lines", "error", err, "despatchId", despatchID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if lines == nil {
		lines = []model.DespatchLine{}
	}

	writeSuccess(w, despatchDetailResponse{Despatch: *d, Lines: lines})
}

// ---------- create / update / delete ----------

type despatchLineBody struct {
	LineNumber  *int    `json:"lineNumber,omitempty"`
	Description string  `json:"description"`
	Quantity    string  `json:"quantity"`
	UnitCode    string  `json:"unitCode"`
	ProductCode *string `json:"productCode,omitempty"`
}

type createDespatchBody struct {
	SeriesID    string `json:"seriesId"`
	DocType     string `json:"docType"`
	Series      string `json:"series"`
	Correlative int    `json:"correlative"`

	IssueDate string  `json:"issueDate"`
	IssueTime *string `json:"issueTime,omitempty"`

	RecipientDocType   string  `json:"recipientDocType"`
	RecipientDocNumber string  `json:"recipientDocNumber"`
	RecipientName      string  `json:"recipientName"`
	RecipientAddress   *string `json:"recipientAddress,omitempty"`

	TransportModality  string  `json:"transportModality"`
	TransferReason     string  `json:"transferReason"`
	TransferReasonDesc *string `json:"transferReasonDesc,omitempty"`
	StartDate          *string `json:"startDate,omitempty"`

	TotalWeightKg  string  `json:"totalWeightKg"`
	WeightUnitCode string  `json:"weightUnitCode"`
	TotalPackages  *int    `json:"totalPackages,omitempty"`

	StartUbigeo    string `json:"startUbigeo"`
	StartAddress   string `json:"startAddress"`
	ArrivalUbigeo  string `json:"arrivalUbigeo"`
	ArrivalAddress string `json:"arrivalAddress"`

	DriverDocType   *string `json:"driverDocType,omitempty"`
	DriverDocNumber *string `json:"driverDocNumber,omitempty"`
	DriverLicense   *string `json:"driverLicense,omitempty"`
	DriverName      *string `json:"driverName,omitempty"`
	VehiclePlate    *string `json:"vehiclePlate,omitempty"`

	CarrierRUC  *string `json:"carrierRuc,omitempty"`
	CarrierName *string `json:"carrierName,omitempty"`
	CarrierMTC  *string `json:"carrierMtc,omitempty"`

	EventCode     *string `json:"eventCode,omitempty"`
	OriginalGreID *string `json:"originalGreId,omitempty"`

	RelatedDocType   *string `json:"relatedDocType,omitempty"`
	RelatedDocSeries *string `json:"relatedDocSeries,omitempty"`
	RelatedDocNumber *string `json:"relatedDocNumber,omitempty"`

	Lines []despatchLineBody `json:"lines"`
}

func (b *createDespatchBody) validate() string {
	if !docUUIDRegex.MatchString(b.SeriesID) {
		return "seriesId inválido"
	}
	if !dateRegex.MatchString(b.IssueDate) {
		return "issueDate inválido"
	}
	if b.IssueTime != nil && !timeRegex.MatchString(*b.IssueTime) {
		return "issueTime inválido"
	}
	if b.StartDate != nil && *b.StartDate != "" && !dateRegex.MatchString(*b.StartDate) {
		return "startDate inválido"
	}
	if b.DocType == "" {
		return "docType requerido"
	}
	if b.Series == "" {
		return "series requerido"
	}
	if b.Correlative <= 0 {
		return "correlative inválido"
	}
	if b.RecipientDocNumber == "" {
		return "recipientDocNumber requerido"
	}
	if b.RecipientName == "" {
		return "recipientName requerido"
	}
	if len(b.Lines) == 0 {
		return "lines requerido"
	}
	for _, l := range b.Lines {
		if l.Description == "" {
			return "line description requerido"
		}
		if !decimalRegex.MatchString(l.Quantity) {
			return "line quantity inválido"
		}
		if l.UnitCode == "" {
			return "line unitCode requerido"
		}
	}
	return ""
}

func (b createDespatchBody) toInput(companyID string) db.DespatchCreateInput {
	lines := make([]db.DespatchLineInput, 0, len(b.Lines))
	for i, l := range b.Lines {
		line := i + 1
		if l.LineNumber != nil {
			line = *l.LineNumber
		}
		lines = append(lines, db.DespatchLineInput{
			LineNumber:  line,
			Description: l.Description,
			Quantity:    l.Quantity,
			UnitCode:    l.UnitCode,
			ProductCode: l.ProductCode,
		})
	}
	return db.DespatchCreateInput{
		CompanyID:          companyID,
		SeriesID:           b.SeriesID,
		DocType:            b.DocType,
		Series:             b.Series,
		Correlative:        b.Correlative,
		IssueDate:          b.IssueDate,
		IssueTime:          b.IssueTime,
		RecipientDocType:   b.RecipientDocType,
		RecipientDocNumber: b.RecipientDocNumber,
		RecipientName:      b.RecipientName,
		RecipientAddress:   b.RecipientAddress,
		TransportModality:  b.TransportModality,
		TransferReason:     b.TransferReason,
		TransferReasonDesc: b.TransferReasonDesc,
		StartDate:          b.StartDate,
		TotalWeightKg:      b.TotalWeightKg,
		WeightUnitCode:     b.WeightUnitCode,
		TotalPackages:      b.TotalPackages,
		StartUbigeo:        b.StartUbigeo,
		StartAddress:       b.StartAddress,
		ArrivalUbigeo:      b.ArrivalUbigeo,
		ArrivalAddress:     b.ArrivalAddress,
		DriverDocType:      b.DriverDocType,
		DriverDocNumber:    b.DriverDocNumber,
		DriverLicense:      b.DriverLicense,
		DriverName:         b.DriverName,
		VehiclePlate:       b.VehiclePlate,
		CarrierRUC:         b.CarrierRUC,
		CarrierName:        b.CarrierName,
		CarrierMTC:         b.CarrierMTC,
		EventCode:          b.EventCode,
		OriginalGreID:      b.OriginalGreID,
		RelatedDocType:     b.RelatedDocType,
		RelatedDocSeries:   b.RelatedDocSeries,
		RelatedDocNumber:   b.RelatedDocNumber,
		Lines:              lines,
	}
}

func (s *server) createDespatchHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	if _, ok := auth.TenantIDFromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "No autenticado")
		return
	}

	var body createDespatchBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Datos inválidos")
		return
	}
	if msg := body.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", msg)
		return
	}

	d, err := s.pool.CreateDespatch(r.Context(), body.toInput(companyID))
	if err != nil {
		s.log.Error("create despatch", "error", err, "companyId", companyID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccessStatus(w, http.StatusCreated, d)
}

type updateDespatchBody struct {
	IssueDate *string `json:"issueDate,omitempty"`
	IssueTime *string `json:"issueTime,omitempty"`

	RecipientDocType   *string `json:"recipientDocType,omitempty"`
	RecipientDocNumber *string `json:"recipientDocNumber,omitempty"`
	RecipientName      *string `json:"recipientName,omitempty"`
	RecipientAddress   *string `json:"recipientAddress,omitempty"`

	TransportModality  *string `json:"transportModality,omitempty"`
	TransferReason     *string `json:"transferReason,omitempty"`
	TransferReasonDesc *string `json:"transferReasonDesc,omitempty"`
	StartDate          *string `json:"startDate,omitempty"`

	TotalWeightKg  *string `json:"totalWeightKg,omitempty"`
	WeightUnitCode *string `json:"weightUnitCode,omitempty"`
	TotalPackages  *int    `json:"totalPackages,omitempty"`

	StartUbigeo    *string `json:"startUbigeo,omitempty"`
	StartAddress   *string `json:"startAddress,omitempty"`
	ArrivalUbigeo  *string `json:"arrivalUbigeo,omitempty"`
	ArrivalAddress *string `json:"arrivalAddress,omitempty"`

	DriverDocType   *string `json:"driverDocType,omitempty"`
	DriverDocNumber *string `json:"driverDocNumber,omitempty"`
	DriverLicense   *string `json:"driverLicense,omitempty"`
	DriverName      *string `json:"driverName,omitempty"`
	VehiclePlate    *string `json:"vehiclePlate,omitempty"`

	CarrierRUC  *string `json:"carrierRuc,omitempty"`
	CarrierName *string `json:"carrierName,omitempty"`
	CarrierMTC  *string `json:"carrierMtc,omitempty"`

	EventCode     *string `json:"eventCode,omitempty"`
	OriginalGreID *string `json:"originalGreId,omitempty"`

	RelatedDocType   *string `json:"relatedDocType,omitempty"`
	RelatedDocSeries *string `json:"relatedDocSeries,omitempty"`
	RelatedDocNumber *string `json:"relatedDocNumber,omitempty"`

	Lines []despatchLineBody `json:"lines,omitempty"`
}

func (b updateDespatchBody) toInput() db.DespatchUpdateInput {
	in := db.DespatchUpdateInput{
		IssueDate:          b.IssueDate,
		IssueTime:          b.IssueTime,
		RecipientDocType:   b.RecipientDocType,
		RecipientDocNumber: b.RecipientDocNumber,
		RecipientName:      b.RecipientName,
		RecipientAddress:   b.RecipientAddress,
		TransportModality:  b.TransportModality,
		TransferReason:     b.TransferReason,
		TransferReasonDesc: b.TransferReasonDesc,
		StartDate:          b.StartDate,
		TotalWeightKg:      b.TotalWeightKg,
		WeightUnitCode:     b.WeightUnitCode,
		TotalPackages:      b.TotalPackages,
		StartUbigeo:        b.StartUbigeo,
		StartAddress:       b.StartAddress,
		ArrivalUbigeo:      b.ArrivalUbigeo,
		ArrivalAddress:     b.ArrivalAddress,
		DriverDocType:      b.DriverDocType,
		DriverDocNumber:    b.DriverDocNumber,
		DriverLicense:      b.DriverLicense,
		DriverName:         b.DriverName,
		VehiclePlate:       b.VehiclePlate,
		CarrierRUC:         b.CarrierRUC,
		CarrierName:        b.CarrierName,
		CarrierMTC:         b.CarrierMTC,
		EventCode:          b.EventCode,
		OriginalGreID:      b.OriginalGreID,
		RelatedDocType:     b.RelatedDocType,
		RelatedDocSeries:   b.RelatedDocSeries,
		RelatedDocNumber:   b.RelatedDocNumber,
	}
	if b.Lines != nil {
		lines := make([]db.DespatchLineInput, 0, len(b.Lines))
		for i, l := range b.Lines {
			line := i + 1
			if l.LineNumber != nil {
				line = *l.LineNumber
			}
			lines = append(lines, db.DespatchLineInput{
				LineNumber:  line,
				Description: l.Description,
				Quantity:    l.Quantity,
				UnitCode:    l.UnitCode,
				ProductCode: l.ProductCode,
			})
		}
		in.Lines = lines
	}
	return in
}

func (s *server) updateDespatchHandler(w http.ResponseWriter, r *http.Request) {
	despatchID := chi.URLParam(r, "despatchId")

	var body updateDespatchBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Datos inválidos")
		return
	}

	d, err := s.pool.UpdateDraftDespatch(r.Context(), despatchID, body.toInput())
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Guía no encontrada")
		case errors.Is(err, db.ErrNotDraft):
			writeError(w, http.StatusBadRequest, "NOT_DRAFT",
				"Solo guías en borrador pueden ser modificadas")
		default:
			s.log.Error("update despatch", "error", err, "despatchId", despatchID)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		}
		return
	}
	writeSuccess(w, d)
}

func (s *server) deleteDespatchHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	despatchID := chi.URLParam(r, "despatchId")

	if err := s.pool.DeleteDespatch(r.Context(), companyID, despatchID); err != nil {
		if errors.Is(err, db.ErrDespatchNotDeletable) {
			writeError(w, http.StatusBadRequest, "NOT_DRAFT",
				"Solo guías en borrador pueden ser eliminadas")
			return
		}
		s.log.Error("delete despatch", "error", err, "despatchId", despatchID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccess(w, map[string]string{"message": "Guía eliminada"})
}

// ---------- issue pipeline ----------

// issueDespatchHandler runs the full GRE pipeline: validate → build XML →
// sign → zip → send to SUNAT's REST API, persisting the ticket on the
// despatches row for later polling.
func (s *server) issueDespatchHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	despatchID := chi.URLParam(r, "despatchId")
	envOverride := readPipelineEnv(r)

	d, err := s.pool.GetDespatch(r.Context(), companyID, despatchID)
	if err != nil {
		s.log.Error("get despatch for issue", "error", err, "despatchId", despatchID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Guía no encontrada")
		return
	}
	if d.Status == model.DespatchStatusAccepted {
		writeError(w, http.StatusBadRequest, "ALREADY_ACCEPTED",
			"La guía ya fue aceptada por SUNAT")
		return
	}

	lines, err := s.pool.GetDespatchLines(r.Context(), despatchID)
	if err != nil {
		s.log.Error("get despatch lines for issue", "error", err, "despatchId", despatchID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}

	// Pre-submission validation — fail fast before calling SUNAT.
	if vErrs := validation.ValidateDespatch(d, lines); len(vErrs) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success":          false,
			"code":             "VALIDATION_ERROR",
			"error":            "Guía no pasa validación pre-SUNAT",
			"validationErrors": vErrs,
		})
		return
	}

	deps, ok := s.loadGREDeps(w, r, companyID)
	if !ok {
		return
	}
	env := resolvePipelineEnv(envOverride, deps.company.SunatEnvironment)

	address, _ := s.pool.GetFiscalAddressByRUC(r.Context(), deps.company.RUC)
	if address == "" {
		address = "-"
	}

	// Por-eventos guías still serialize as 09 or 31 on the wire — the
	// base doc type defaults to Remitente (09). A future iteration can
	// accept it from the request body.
	eventBase := model.DespatchTypeRemitente
	if d.DocType == model.DespatchTypeEvento && len(d.Series) > 0 && d.Series[0] == 'V' {
		eventBase = model.DespatchTypeTransportista
	}

	xmlBytes, err := xmlbuilder.BuildDespatchXML(xmlbuilder.DespatchXMLInput{
		Despatch:         d,
		Lines:            lines,
		RUC:              deps.company.RUC,
		CompanyName:      deps.company.CompanyName,
		CompanyAddress:   address,
		EventBaseDocType: eventBase,
	})
	if err != nil {
		s.log.Error("build despatch xml", "error", err, "despatchId", despatchID)
		writeError(w, http.StatusInternalServerError, "XML_BUILD_ERROR", err.Error())
		return
	}

	signedXML, err := signature.SignXML(xmlBytes, deps.parsedCert.Certificate, deps.parsedCert.PrivateKey)
	if err != nil {
		s.log.Error("sign despatch xml", "error", err, "despatchId", despatchID)
		writeError(w, http.StatusInternalServerError, "SIGN_ERROR", err.Error())
		return
	}

	filename := xmlbuilder.DespatchFilename(deps.company.RUC, d.DocType, d.Series, d.Correlative, eventBase)
	zipBytes, err := zipper.CreateZIP(filename, signedXML)
	if err != nil {
		s.log.Error("zip despatch", "error", err, "despatchId", despatchID)
		writeError(w, http.StatusInternalServerError, "ZIP_ERROR", err.Error())
		return
	}

	// Upload artifacts before calling SUNAT so we can inspect what was
	// sent even if the API call fails.
	xmlKey := r2.DocumentKey(deps.tenantID, companyID, despatchID, r2.FileXML)
	if err := s.r2.UploadDocumentFile(r.Context(), xmlKey, r2.FileXML, xmlBytes); err != nil {
		s.log.Error("upload despatch xml", "error", err, "key", xmlKey)
		writeError(w, http.StatusInternalServerError, "R2_UPLOAD_ERROR", err.Error())
		return
	}
	signedKey := r2.DocumentKey(deps.tenantID, companyID, despatchID, r2.FileSignedXML)
	if err := s.r2.UploadDocumentFile(r.Context(), signedKey, r2.FileSignedXML, signedXML); err != nil {
		s.log.Error("upload despatch signed xml", "error", err, "key", signedKey)
		writeError(w, http.StatusInternalServerError, "R2_UPLOAD_ERROR", err.Error())
		return
	}
	zipKey := r2.DocumentKey(deps.tenantID, companyID, despatchID, r2.FileZIP)
	if err := s.r2.UploadDocumentFile(r.Context(), zipKey, r2.FileZIP, zipBytes); err != nil {
		s.log.Error("upload despatch zip", "error", err, "key", zipKey)
		writeError(w, http.StatusInternalServerError, "R2_UPLOAD_ERROR", err.Error())
		return
	}

	sendResp, err := s.greClient.Send(r.Context(), companyID, env, deps.greCredentials, filename, zipBytes)
	if err != nil {
		s.log.Error("gre send", "error", err, "despatchId", despatchID)
		// Persist R2 artifacts + error state.
		errStatus := model.DespatchStatusError
		errMsg := err.Error()
		_, _ = s.pool.ApplyDespatchResult(r.Context(), despatchID, db.DespatchIssueResult{
			Status:                   errStatus,
			SunatResponseDescription: &errMsg,
			R2XmlKey:                 &xmlKey,
			R2SignedXmlKey:           &signedKey,
			R2ZipKey:                 &zipKey,
		})
		writeError(w, http.StatusBadGateway, "SUNAT_ERROR", err.Error())
		return
	}

	ticket := sendResp.NumTicket
	updated, err := s.pool.ApplyDespatchResult(r.Context(), despatchID, db.DespatchIssueResult{
		Status:         model.DespatchStatusSent,
		SunatTicket:    &ticket,
		R2XmlKey:       &xmlKey,
		R2SignedXmlKey: &signedKey,
		R2ZipKey:       &zipKey,
		MarkSent:       true,
	})
	if err != nil {
		s.log.Error("apply despatch send result", "error", err, "despatchId", despatchID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccess(w, updated)
}

// ---------- poll pipeline ----------

// pollDespatchHandler queries SUNAT for the status of a previously-sent
// GRE ticket, decodes the base64 CDR (when present), persists it, and
// writes the outcome back to the despatches row.
func (s *server) pollDespatchHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	despatchID := chi.URLParam(r, "despatchId")
	envOverride := readPipelineEnv(r)

	d, err := s.pool.GetDespatch(r.Context(), companyID, despatchID)
	if err != nil {
		s.log.Error("get despatch for poll", "error", err, "despatchId", despatchID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Guía no encontrada")
		return
	}
	if d.SunatTicket == nil || *d.SunatTicket == "" {
		writeError(w, http.StatusBadRequest, "NO_TICKET",
			"La guía aún no fue enviada a SUNAT")
		return
	}

	deps, ok := s.loadGREDeps(w, r, companyID)
	if !ok {
		return
	}
	env := resolvePipelineEnv(envOverride, deps.company.SunatEnvironment)

	status, err := s.greClient.Poll(r.Context(), companyID, env, deps.greCredentials, *d.SunatTicket)
	if err != nil {
		s.log.Error("gre poll", "error", err, "despatchId", despatchID)
		writeError(w, http.StatusBadGateway, "SUNAT_ERROR", err.Error())
		return
	}

	switch status.CodRespuesta {
	case "98":
		// Still in process — echo status without touching the row.
		writeSuccess(w, map[string]string{"statusCode": status.CodRespuesta})
		return
	case "0":
		// Accepted — decode CDR, persist, mark accepted.
		if status.ArcCdr == "" {
			writeError(w, http.StatusBadGateway, "CDR_MISSING",
				"SUNAT aceptó la guía pero no devolvió CDR")
			return
		}
		cdrBytes, err := base64.StdEncoding.DecodeString(status.ArcCdr)
		if err != nil {
			s.log.Error("decode gre cdr", "error", err, "despatchId", despatchID)
			writeError(w, http.StatusInternalServerError, "CDR_DECODE_ERROR", err.Error())
			return
		}
		parsed, err := cdr.Parse(cdrBytes)
		if err != nil {
			s.log.Error("parse gre cdr", "error", err, "despatchId", despatchID)
			writeError(w, http.StatusInternalServerError, "CDR_PARSE_ERROR", err.Error())
			return
		}
		cdrKey := r2.DocumentKey(deps.tenantID, companyID, despatchID, r2.FileCDR)
		if err := s.r2.UploadDocumentFile(r.Context(), cdrKey, r2.FileCDR, cdrBytes); err != nil {
			s.log.Error("upload gre cdr", "error", err, "key", cdrKey)
			writeError(w, http.StatusInternalServerError, "R2_UPLOAD_ERROR", err.Error())
			return
		}

		finalStatus := model.DespatchStatusAccepted
		if !parsed.Accepted {
			finalStatus = model.DespatchStatusRejected
		}
		code := parsed.ResponseCode
		desc := parsed.Description
		updated, err := s.pool.ApplyDespatchResult(r.Context(), despatchID, db.DespatchIssueResult{
			Status:                   finalStatus,
			SunatResponseCode:        &code,
			SunatResponseDescription: &desc,
			R2CdrKey:                 &cdrKey,
			MarkAccepted:             parsed.Accepted,
		})
		if err != nil {
			s.log.Error("apply despatch poll result", "error", err, "despatchId", despatchID)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
			return
		}
		writeSuccess(w, updated)
		return
	case "99":
		// Rejected.
		var numErr, desErr string
		if status.Error != nil {
			numErr = status.Error.NumError
			desErr = status.Error.DesError
		}
		updated, err := s.pool.ApplyDespatchResult(r.Context(), despatchID, db.DespatchIssueResult{
			Status:                   model.DespatchStatusRejected,
			SunatResponseCode:        &numErr,
			SunatResponseDescription: &desErr,
		})
		if err != nil {
			s.log.Error("apply despatch reject result", "error", err, "despatchId", despatchID)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
			return
		}
		writeSuccess(w, updated)
		return
	default:
		writeJSON(w, http.StatusOK, map[string]any{
			"success":    true,
			"statusCode": status.CodRespuesta,
			"receivedAt": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// ---------- files ----------

func (s *server) despatchFileHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	despatchID := chi.URLParam(r, "despatchId")
	fileType := chi.URLParam(r, "fileType")

	d, err := s.pool.GetDespatch(r.Context(), companyID, despatchID)
	if err != nil {
		s.log.Error("get despatch for file", "error", err, "despatchId", despatchID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Guía no encontrada")
		return
	}

	var r2Key *string
	switch fileType {
	case "xml":
		r2Key = d.R2XmlKey
	case "signed_xml":
		r2Key = d.R2SignedXmlKey
	case "zip":
		r2Key = d.R2ZipKey
	case "cdr":
		r2Key = d.R2CdrKey
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
		s.log.Error("presign despatch file", "error", err, "key", *r2Key)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccess(w, map[string]string{"url": url, "fileType": fileType})
}
