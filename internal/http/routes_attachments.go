package http

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/perunio/perunio-facturador/internal/auth"
	"github.com/perunio/perunio-facturador/internal/db"
	"github.com/perunio/perunio-facturador/internal/model"
	"github.com/perunio/perunio-facturador/internal/r2"
)

// maxAttachmentBytes caps a single supporting-document upload.
const maxAttachmentBytes = 10 << 20 // 10 MiB

// allowedAttachmentExts is the allow-list of supporting-document file types.
// Extension-based (case-insensitive) — the primary guard alongside the size cap.
var allowedAttachmentExts = map[string]bool{
	".pdf":  true,
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".webp": true,
	".gif":  true,
	".xlsx": true,
	".xls":  true,
	".docx": true,
	".doc":  true,
	".csv":  true,
	".txt":  true,
	".xml":  true,
	".zip":  true,
}

// randomUUID returns an RFC 4122 v4 UUID string, used as the attachment id (and
// therefore its R2 object key) — generated before upload so every persisted row
// points at a real object.
func randomUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// listAttachmentsHandler returns the supporting documents attached to a comprobante.
func (s *Server) listAttachmentsHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	docID := chi.URLParam(r, "docId")

	items, err := s.pool.ListAttachments(r.Context(), companyID, docID)
	if err != nil {
		s.log.Error("list attachments", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if items == nil {
		items = []model.ComprobanteAttachment{}
	}
	writeSuccess(w, items)
}

// uploadAttachmentHandler stores a supporting document (multipart/form-data,
// field "file") in R2 and persists its metadata. Validation the PDF path lacks:
// a 10 MiB size cap and an extension allow-list.
func (s *Server) uploadAttachmentHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	docID := chi.URLParam(r, "docId")

	payload, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "Autenticación requerida")
		return
	}

	// Cap the whole request body before parsing, so an oversized upload is
	// rejected without buffering it all.
	r.Body = http.MaxBytesReader(w, r.Body, maxAttachmentBytes)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "No se pudo leer el archivo (campo 'file')")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedAttachmentExts[ext] {
		writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_TYPE", "Tipo de archivo no permitido")
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "No se pudo leer el archivo")
		return
	}
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, "EMPTY_BODY", "El archivo está vacío")
		return
	}

	// Prefer the multipart part's declared content type; fall back to the
	// extension. Used both for the R2 object and the stored metadata.
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = mime.TypeByExtension(ext)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	attachmentID, err := randomUUID()
	if err != nil {
		s.log.Error("generate attachment id", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}

	// Upload to R2 first so a persisted row always points at a real object.
	key := r2.AttachmentKey(payload.TenantID, companyID, docID, attachmentID, strings.TrimPrefix(ext, "."))
	if err := s.r2.UploadAttachment(r.Context(), key, contentType, data); err != nil {
		s.log.Error("upload attachment", "error", err, "key", key)
		writeError(w, http.StatusInternalServerError, "R2_UPLOAD_ERROR", "No se pudo guardar el archivo")
		return
	}

	att, err := s.pool.CreateAttachment(r.Context(), companyID, docID, db.CreateAttachmentInput{
		FileName:   header.Filename,
		MimeType:   contentType,
		FileSize:   int64(len(data)),
		R2Key:      key,
		UploadedBy: &payload.UserID,
	})
	if err != nil {
		// Row wasn't written — drop the just-uploaded object so it doesn't orphan.
		if delErr := s.r2.DeleteDocumentFile(r.Context(), key); delErr != nil {
			s.log.Error("cleanup orphan attachment", "error", delErr, "key", key)
		}
		if errors.Is(err, db.ErrDocumentNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Documento no encontrado")
			return
		}
		s.log.Error("create attachment", "error", err, "docId", docID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccessStatus(w, http.StatusCreated, att)
}

// downloadAttachmentHandler returns a presigned R2 URL for one attachment,
// carrying the original filename as an attachment download.
func (s *Server) downloadAttachmentHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	docID := chi.URLParam(r, "docId")
	attachmentID := chi.URLParam(r, "attachmentId")

	fileName, key, err := s.pool.GetAttachmentKey(r.Context(), companyID, docID, attachmentID)
	if err != nil {
		if errors.Is(err, db.ErrDocumentNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Archivo no encontrado")
			return
		}
		s.log.Error("get attachment key", "error", err, "attachmentId", attachmentID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}

	url, err := s.r2.DocumentPresignedURL(r.Context(), key, fileName, 0)
	if err != nil {
		s.log.Error("presign attachment", "error", err, "key", key)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	writeSuccess(w, map[string]string{"url": url})
}

// deleteAttachmentHandler removes an attachment (row + R2 object). The DB row is
// dropped first so the list is immediately consistent; a residual R2 object (if
// its delete fails) is harmless and swept by the per-document prefix cleanup.
func (s *Server) deleteAttachmentHandler(w http.ResponseWriter, r *http.Request) {
	companyID := chi.URLParam(r, "companyId")
	docID := chi.URLParam(r, "docId")
	attachmentID := chi.URLParam(r, "attachmentId")

	_, key, err := s.pool.GetAttachmentKey(r.Context(), companyID, docID, attachmentID)
	if err != nil {
		if errors.Is(err, db.ErrDocumentNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Archivo no encontrado")
			return
		}
		s.log.Error("get attachment for delete", "error", err, "attachmentId", attachmentID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}

	if err := s.pool.DeleteAttachment(r.Context(), companyID, docID, attachmentID); err != nil {
		if errors.Is(err, db.ErrDocumentNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Archivo no encontrado")
			return
		}
		s.log.Error("delete attachment", "error", err, "attachmentId", attachmentID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}
	if err := s.r2.DeleteDocumentFile(r.Context(), key); err != nil {
		s.log.Error("delete attachment object", "error", err, "key", key)
	}
	writeSuccess(w, map[string]string{"message": "Archivo eliminado"})
}
