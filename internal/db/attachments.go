package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/perunio/perunio-facturador/internal/auth"
	"github.com/perunio/perunio-facturador/internal/model"
)

// attachmentColumns is the shared projection for a comprobante attachment row.
// r2_key is deliberately excluded — it's an internal storage detail, never sent
// on the wire (see GetAttachmentKey for the download/delete path).
const attachmentColumns = `
	id, document_id, file_name, mime_type, file_size, created_at`

func scanAttachment(row pgx.Row, a *model.ComprobanteAttachment) error {
	return row.Scan(&a.ID, &a.DocumentID, &a.FileName, &a.MimeType, &a.FileSize, &a.CreatedAt)
}

// ListAttachments returns every attachment for a document, scoped to the company
// (and the tenant, via RLS), newest first.
func (p *Pool) ListAttachments(ctx context.Context, companyID, docID string) ([]model.ComprobanteAttachment, error) {
	var out []model.ComprobanteAttachment
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+attachmentColumns+`
			FROM comprobante_attachments
			WHERE document_id = $1
			  AND document_id IN (SELECT id FROM issued_documents WHERE company_id = $2)
			ORDER BY created_at DESC`, docID, companyID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a model.ComprobanteAttachment
			if err := scanAttachment(rows, &a); err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

// CreateAttachmentInput is the payload for persisting one attachment's metadata.
// The bytes have already been written to R2 at R2Key by the handler.
type CreateAttachmentInput struct {
	FileName   string
	MimeType   string
	FileSize   int64
	R2Key      string
	UploadedBy *string
}

// CreateAttachment persists attachment metadata. The INSERT is guarded by an
// EXISTS check so an attachment can never be tied to a document outside the
// company/tenant. Returns ErrDocumentNotFound when the document doesn't match.
func (p *Pool) CreateAttachment(ctx context.Context, companyID, docID string, in CreateAttachmentInput) (*model.ComprobanteAttachment, error) {
	tenantID, ok := auth.TenantIDFromContext(ctx)
	if !ok {
		return nil, ErrTenantContextMissing
	}

	var a model.ComprobanteAttachment
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO comprobante_attachments
				(tenant_id, document_id, file_name, mime_type, file_size, r2_key, uploaded_by)
			SELECT $1, $2, $3, $4, $5, $6, $7
			WHERE EXISTS (SELECT 1 FROM issued_documents WHERE id = $2 AND company_id = $8)
			RETURNING `+attachmentColumns,
			tenantID, docID, in.FileName, in.MimeType, in.FileSize, in.R2Key, in.UploadedBy, companyID)
		if err := scanAttachment(row, &a); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrDocumentNotFound
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// GetAttachmentKey returns the stored filename and R2 key for one attachment,
// scoped to the document and company. Used by the download and delete paths.
// Returns ErrDocumentNotFound when nothing matched.
func (p *Pool) GetAttachmentKey(ctx context.Context, companyID, docID, attachmentID string) (fileName, r2Key string, err error) {
	err = p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT file_name, r2_key
			FROM comprobante_attachments
			WHERE id = $1 AND document_id = $2
			  AND document_id IN (SELECT id FROM issued_documents WHERE company_id = $3)`,
			attachmentID, docID, companyID)
		if scanErr := row.Scan(&fileName, &r2Key); scanErr != nil {
			if errors.Is(scanErr, pgx.ErrNoRows) {
				return ErrDocumentNotFound
			}
			return scanErr
		}
		return nil
	})
	return fileName, r2Key, err
}

// DeleteAttachment removes an attachment row, scoped to the document and company.
// Returns ErrDocumentNotFound when nothing matched. The R2 object is removed by
// the handler.
func (p *Pool) DeleteAttachment(ctx context.Context, companyID, docID, attachmentID string) error {
	return p.WithTenant(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			DELETE FROM comprobante_attachments
			WHERE id = $1 AND document_id = $2
			  AND document_id IN (SELECT id FROM issued_documents WHERE company_id = $3)`,
			attachmentID, docID, companyID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrDocumentNotFound
		}
		return nil
	})
}
