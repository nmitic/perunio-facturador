package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/perunio/perunio-facturador/internal/auth"
	"github.com/perunio/perunio-facturador/internal/model"
)

// CreateVoidRequest creates a voided_documents row linked to the given docs.
// Validates every doc is accepted (or accepted_with_observations) and within
// the 7-day void window. Mirrors FacturadorService.createVoidRequest.
func (p *Pool) CreateVoidRequest(ctx context.Context, companyID, voidID, voidDate string, documentIDs []string, reason string) (*model.VoidedDocument, error) {
	tenantID, ok := auth.TenantIDFromContext(ctx)
	if !ok {
		return nil, ErrTenantContextMissing
	}
	if len(documentIDs) == 0 {
		return nil, fmt.Errorf("documentIDs is empty")
	}

	type docRow struct {
		id          string
		docType     string
		series      string
		correlative int
		status      string
		issueDate   time.Time
	}

	var voidDoc model.VoidedDocument
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, doc_type, series, correlative, status, issue_date
			FROM issued_documents
			WHERE company_id = $1 AND id = ANY($2)
		`, companyID, documentIDs)
		if err != nil {
			return err
		}
		var docs []docRow
		for rows.Next() {
			var d docRow
			if err := rows.Scan(&d.id, &d.docType, &d.series, &d.correlative, &d.status, &d.issueDate); err != nil {
				rows.Close()
				return err
			}
			docs = append(docs, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(docs) != len(documentIDs) {
			return ErrNotFound
		}

		now := time.Now()
		for _, d := range docs {
			if d.status != "accepted" && d.status != "accepted_with_observations" {
				return fmt.Errorf("%w: %s-%d", ErrInvalidDocStatus, d.series, d.correlative)
			}
			if now.Sub(d.issueDate).Hours()/24 > 7 {
				return fmt.Errorf("%w: %s-%d", ErrVoidWindowExpired, d.series, d.correlative)
			}
		}

		// Insert the void row.
		if err := tx.QueryRow(ctx,
			"INSERT INTO voided_documents (tenant_id, company_id, void_id, void_date)"+
				" VALUES ($1, $2, $3, $4) RETURNING "+voidedDocumentColumns,
			tenantID, companyID, voidID, voidDate,
		).Scan(
			&voidDoc.ID, &voidDoc.TenantID, &voidDoc.CompanyID, &voidDoc.VoidID, &voidDoc.VoidDate, &voidDoc.Status,
			&voidDoc.SunatTicket, &voidDoc.SunatResponseCode, &voidDoc.SunatResponseDescription,
			&voidDoc.R2XmlKey, &voidDoc.R2SignedXmlKey, &voidDoc.R2CdrKey,
			&voidDoc.SentAt, &voidDoc.AcceptedAt, &voidDoc.CreatedAt, &voidDoc.UpdatedAt,
		); err != nil {
			return err
		}

		// Link items.
		linkRows := make([][]any, 0, len(docs))
		for _, d := range docs {
			linkRows = append(linkRows, []any{voidDoc.ID, d.id, d.docType, d.series, d.correlative, reason})
		}
		if _, err := tx.CopyFrom(ctx,
			pgx.Identifier{"voided_document_items"},
			[]string{"void_id", "document_id", "doc_type", "series", "correlative", "reason"},
			pgx.CopyFromRows(linkRows),
		); err != nil {
			return err
		}

		// Mark each doc as voided.
		if _, err := tx.Exec(ctx,
			`UPDATE issued_documents SET status = 'voided', updated_at = now() WHERE id = ANY($1)`,
			documentIDs,
		); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &voidDoc, nil
}

// VoidIssueResult captures the outcome of sending a void communication.
// All pointer fields are optional.
type VoidIssueResult struct {
	Status                   string
	SunatTicket              *string
	SunatResponseCode        *string
	SunatResponseDescription *string
	R2XmlKey                 *string
	R2SignedXmlKey           *string
	R2CdrKey                 *string
	MarkSent                 bool
	MarkAccepted             bool
}

// ApplyVoidResult writes the pipeline/poll outcome back to voided_documents
// and returns the refreshed row.
func (p *Pool) ApplyVoidResult(ctx context.Context, voidID string, res VoidIssueResult) (*model.VoidedDocument, error) {
	var v model.VoidedDocument
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		set := []string{"status = $1", "updated_at = now()"}
		args := []any{res.Status}
		add := func(col string, val any) {
			args = append(args, val)
			set = append(set, fmt.Sprintf("%s = $%d", col, len(args)))
		}
		if res.SunatTicket != nil {
			add("sunat_ticket", *res.SunatTicket)
		}
		if res.SunatResponseCode != nil {
			add("sunat_response_code", *res.SunatResponseCode)
		}
		if res.SunatResponseDescription != nil {
			add("sunat_response_description", *res.SunatResponseDescription)
		}
		if res.R2XmlKey != nil {
			add("r2_xml_key", *res.R2XmlKey)
		}
		if res.R2SignedXmlKey != nil {
			add("r2_signed_xml_key", *res.R2SignedXmlKey)
		}
		if res.R2CdrKey != nil {
			add("r2_cdr_key", *res.R2CdrKey)
		}
		if res.MarkSent {
			set = append(set, "sent_at = COALESCE(sent_at, now())")
		}
		if res.MarkAccepted {
			set = append(set, "accepted_at = now()")
		}
		args = append(args, voidID)
		sql := fmt.Sprintf(
			"UPDATE voided_documents SET %s WHERE id = $%d RETURNING %s",
			strings.Join(set, ", "), len(args), voidedDocumentColumns,
		)
		return scanVoidedDocument(tx.QueryRow(ctx, sql, args...), &v)
	})
	if err != nil {
		return nil, err
	}
	return &v, nil
}

const voidedDocumentColumns = `
	id, tenant_id, company_id, void_id, void_date, status,
	sunat_ticket, sunat_response_code, sunat_response_description,
	r2_xml_key, r2_signed_xml_key, r2_cdr_key,
	sent_at, accepted_at, created_at, updated_at
`

func scanVoidedDocument(row pgx.Row, v *model.VoidedDocument) error {
	return row.Scan(
		&v.ID, &v.TenantID, &v.CompanyID, &v.VoidID, &v.VoidDate, &v.Status,
		&v.SunatTicket, &v.SunatResponseCode, &v.SunatResponseDescription,
		&v.R2XmlKey, &v.R2SignedXmlKey, &v.R2CdrKey,
		&v.SentAt, &v.AcceptedAt, &v.CreatedAt, &v.UpdatedAt,
	)
}

// ListVoidedDocuments returns every void communication for the company,
// newest first.
func (p *Pool) ListVoidedDocuments(ctx context.Context, companyID string) ([]model.VoidedDocument, error) {
	var out []model.VoidedDocument
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			"SELECT "+voidedDocumentColumns+
				" FROM voided_documents WHERE company_id = $1 ORDER BY void_date DESC",
			companyID,
		)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v model.VoidedDocument
			if err := scanVoidedDocument(rows, &v); err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

// GetVoidedDocument fetches a single void communication scoped to the company.
// Returns nil when not found.
func (p *Pool) GetVoidedDocument(ctx context.Context, companyID, voidID string) (*model.VoidedDocument, error) {
	var out *model.VoidedDocument
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			"SELECT "+voidedDocumentColumns+
				" FROM voided_documents WHERE company_id = $1 AND id = $2 LIMIT 1",
			companyID, voidID,
		)
		var got model.VoidedDocument
		if err := scanVoidedDocument(row, &got); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		out = &got
		return nil
	})
	return out, err
}

// GetVoidedDocumentItems returns the documents linked to a void communication.
func (p *Pool) GetVoidedDocumentItems(ctx context.Context, voidID string) ([]model.VoidedDocumentItem, error) {
	var out []model.VoidedDocumentItem
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, void_id, document_id, doc_type, series, correlative, reason, created_at
			FROM voided_document_items
			WHERE void_id = $1
			ORDER BY series, correlative
		`, voidID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var it model.VoidedDocumentItem
			if err := rows.Scan(&it.ID, &it.VoidID, &it.DocumentID, &it.DocType,
				&it.Series, &it.Correlative, &it.Reason, &it.CreatedAt); err != nil {
				return err
			}
			out = append(out, it)
		}
		return rows.Err()
	})
	return out, err
}
