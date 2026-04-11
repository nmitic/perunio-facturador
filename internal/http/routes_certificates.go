package http

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/perunio/perunio-facturador/internal/auth"
	facturadorCrypto "github.com/perunio/perunio-facturador/internal/crypto"
	"github.com/perunio/perunio-facturador/internal/db"
	"github.com/perunio/perunio-facturador/internal/model"
	"github.com/perunio/perunio-facturador/internal/r2"
)

// listCertificatesHandler returns every certificate for a company.
func (s *server) listCertificatesHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")

	certs, err := s.pool.ListCertificates(r.Context(), companyID)
	if err != nil {
		s.log.Error("list certificates", "error", err, "companyId", companyID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if certs == nil {
		certs = []model.Certificate{}
	}
	writeSuccess(w, certs)
}

// getCertificateHandler returns a single certificate by id.
func (s *server) getCertificateHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	certID := chi.URLParam(r, "certId")

	cert, err := s.pool.GetCertificate(r.Context(), companyID, certID)
	if err != nil {
		s.log.Error("get certificate", "error", err, "certId", certID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if cert == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Certificado no encontrado")
		return
	}
	writeSuccess(w, cert)
}

type uploadCertificateRequest struct {
	Label             string `json:"label"`
	Password          string `json:"password"`
	CertificateBase64 string `json:"certificateBase64"`
}

// uploadCertificateHandler accepts a base64 PFX + password, dedupes by
// fingerprint, uploads to R2, encrypts the password, and inserts the row.
// Mirrors certificates.routes.ts POST /:companyId.
func (s *server) uploadCertificateHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	tenantID, ok := auth.TenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "No autenticado")
		return
	}

	var req uploadCertificateRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Datos inválidos")
		return
	}
	if req.Label == "" || len(req.Label) > 255 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "label requerido (1-255 chars)")
		return
	}
	if req.Password == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "password requerido")
		return
	}
	if req.CertificateBase64 == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Archivo de certificado requerido")
		return
	}

	certBytes, err := base64.StdEncoding.DecodeString(req.CertificateBase64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "certificateBase64 inválido")
		return
	}
	if len(certBytes) > 50*1024 {
		writeError(w, http.StatusBadRequest, "FILE_TOO_LARGE", "El certificado no puede exceder 50KB")
		return
	}

	sum := sha256.Sum256(certBytes)
	fingerprint := hex.EncodeToString(sum[:])

	existingID, err := s.pool.FindCertificateByFingerprint(r.Context(), companyID, fingerprint)
	if err != nil {
		s.log.Error("find certificate by fingerprint", "error", err, "companyId", companyID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if existingID != "" {
		writeError(w, http.StatusConflict, "CERTIFICATE_DUPLICATE", "Este certificado ya fue registrado")
		return
	}

	certID := uuid.NewString()
	r2Key := r2.CertificateKey(tenantID, companyID, certID)
	if err := s.r2.UploadCertificate(r.Context(), r2Key, certBytes); err != nil {
		s.log.Error("upload certificate to r2", "error", err, "key", r2Key)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}

	encryptedPassword, err := facturadorCrypto.EncryptAES256GCM(req.Password, s.cfg.EncryptionKey)
	if err != nil {
		// Best-effort cleanup so an orphan R2 object is not left behind.
		_ = s.r2.DeleteCertificate(r.Context(), r2Key)
		s.log.Error("encrypt certificate password", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}

	cert, err := s.pool.CreateCertificate(r.Context(), companyID, db.CreateCertificateInput{
		ID:                certID,
		Label:             req.Label,
		R2CertificateKey:  r2Key,
		EncryptedPassword: encryptedPassword,
		FingerprintSha256: fingerprint,
		FileSizeBytes:     len(certBytes),
	})
	if err != nil {
		_ = s.r2.DeleteCertificate(r.Context(), r2Key)
		s.log.Error("insert certificate", "error", err, "companyId", companyID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccessStatus(w, http.StatusCreated, cert)
}

// activateCertificateHandler sets one certificate active and all others
// inactive for the company in a single transaction.
func (s *server) activateCertificateHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	certID := chi.URLParam(r, "certId")

	if err := s.pool.ActivateCertificate(r.Context(), companyID, certID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Certificado no encontrado")
			return
		}
		s.log.Error("activate certificate", "error", err, "certId", certID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccess(w, map[string]string{"message": "Certificado activado"})
}

// deleteCertificateHandler removes the DB row and then best-effort deletes the
// R2 object. R2 failure is logged but does not fail the request.
func (s *server) deleteCertificateHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	certID := chi.URLParam(r, "certId")

	r2Key, err := s.pool.DeleteCertificate(r.Context(), companyID, certID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Certificado no encontrado")
			return
		}
		s.log.Error("delete certificate", "error", err, "certId", certID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if err := s.r2.DeleteCertificate(r.Context(), r2Key); err != nil {
		s.log.Warn("delete certificate from r2", "error", err, "key", r2Key)
	}
	writeSuccess(w, map[string]string{"message": "Certificado eliminado"})
}
