package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/perunio/perunio-facturador/internal/model"
)

// DocumentListFilter narrows ListIssuedDocuments. Empty fields are ignored.
type DocumentListFilter struct {
	DocType           string
	Status            string
	CustomerDocNumber string
	Page              int
	Limit             int
}

// DocumentListResult is a paginated slice of issued documents plus the total
// count for the same filter.
type DocumentListResult struct {
	Documents []model.IssuedDocument `json:"documents"`
	Total     int                    `json:"total"`
}

const issuedDocumentColumns = `
	id, tenant_id, company_id, series_id, doc_type, series, correlative, status,
	issue_date, issue_time, due_date,
	currency_code, operation_type,
	customer_doc_type, customer_doc_number, customer_name, customer_address,
	subtotal, total_igv, total_isc, total_other_taxes, total_discount, total_amount,
	tax_inclusive_amount, notes,
	reference_doc_type, reference_doc_series, reference_doc_correlative,
	credit_debit_reason_code, credit_debit_reason_desc,
	sunat_response_code, sunat_response_description, sunat_ticket,
	r2_xml_key, r2_signed_xml_key, r2_zip_key, r2_cdr_key, r2_pdf_key,
	qr_data,
	sent_at, accepted_at, created_at, updated_at
`

func scanIssuedDocument(row pgx.Row, d *model.IssuedDocument) error {
	return row.Scan(
		&d.ID, &d.TenantID, &d.CompanyID, &d.SeriesID, &d.DocType, &d.Series, &d.Correlative, &d.Status,
		&d.IssueDate, &d.IssueTime, &d.DueDate,
		&d.CurrencyCode, &d.OperationType,
		&d.CustomerDocType, &d.CustomerDocNumber, &d.CustomerName, &d.CustomerAddress,
		&d.Subtotal, &d.TotalIgv, &d.TotalIsc, &d.TotalOtherTaxes, &d.TotalDiscount, &d.TotalAmount,
		&d.TaxInclusiveAmount, &d.Notes,
		&d.ReferenceDocType, &d.ReferenceDocSeries, &d.ReferenceDocCorrelative,
		&d.CreditDebitReasonCode, &d.CreditDebitReasonDesc,
		&d.SunatResponseCode, &d.SunatResponseDescription, &d.SunatTicket,
		&d.R2XmlKey, &d.R2SignedXmlKey, &d.R2ZipKey, &d.R2CdrKey, &d.R2PdfKey,
		&d.QrData,
		&d.SentAt, &d.AcceptedAt, &d.CreatedAt, &d.UpdatedAt,
	)
}

// ListIssuedDocuments returns a paginated slice of issued documents for the
// company plus the total count, scoped to the tenant from context.
func (p *Pool) ListIssuedDocuments(ctx context.Context, companyID string, filter DocumentListFilter) (DocumentListResult, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	limit := filter.Limit
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	args := []any{companyID}
	conditions := []string{"company_id = $1"}
	if filter.DocType != "" {
		args = append(args, filter.DocType)
		conditions = append(conditions, fmt.Sprintf("doc_type = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	if filter.CustomerDocNumber != "" {
		args = append(args, filter.CustomerDocNumber)
		conditions = append(conditions, fmt.Sprintf("customer_doc_number = $%d", len(args)))
	}
	where := strings.Join(conditions, " AND ")

	var result DocumentListResult
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		// Total count
		countSQL := "SELECT count(*)::int FROM issued_documents WHERE " + where
		if err := tx.QueryRow(ctx, countSQL, args...).Scan(&result.Total); err != nil {
			return err
		}

		// Page of rows
		listArgs := append([]any{}, args...)
		listArgs = append(listArgs, limit, offset)
		listSQL := "SELECT " + issuedDocumentColumns +
			" FROM issued_documents WHERE " + where +
			fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)

		rows, err := tx.Query(ctx, listSQL, listArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var d model.IssuedDocument
			if err := scanIssuedDocument(rows, &d); err != nil {
				return err
			}
			result.Documents = append(result.Documents, d)
		}
		return rows.Err()
	})
	return result, err
}

// GetIssuedDocument returns a single document scoped to the company. Returns
// nil when not found.
func (p *Pool) GetIssuedDocument(ctx context.Context, companyID, docID string) (*model.IssuedDocument, error) {
	var doc *model.IssuedDocument
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			"SELECT "+issuedDocumentColumns+
				" FROM issued_documents WHERE company_id = $1 AND id = $2 LIMIT 1",
			companyID, docID,
		)
		var got model.IssuedDocument
		if err := scanIssuedDocument(row, &got); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		doc = &got
		return nil
	})
	return doc, err
}

// IssuedDocumentResult captures the outcome of a full issue pipeline run so
// it can be persisted in a single UPDATE. Any nil pointer means "leave
// existing column value unchanged".
type IssuedDocumentResult struct {
	Status                   string
	SunatResponseCode        *string
	SunatResponseDescription *string
	R2XmlKey                 *string
	R2SignedXmlKey           *string
	R2ZipKey                 *string
	R2CdrKey                 *string
	R2PdfKey                 *string
	QRData                   *string
	MarkSent                 bool
	MarkAccepted             bool
}

// ApplyIssueResult writes the pipeline outcome back to issued_documents and
// returns the refreshed row. Must be called inside the request's tenant
// context.
func (p *Pool) ApplyIssueResult(ctx context.Context, docID string, res IssuedDocumentResult) (*model.IssuedDocument, error) {
	var doc model.IssuedDocument
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		set := []string{"status = $1", "updated_at = now()"}
		args := []any{res.Status}
		add := func(col string, val any) {
			args = append(args, val)
			set = append(set, fmt.Sprintf("%s = $%d", col, len(args)))
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
		if res.R2ZipKey != nil {
			add("r2_zip_key", *res.R2ZipKey)
		}
		if res.R2CdrKey != nil {
			add("r2_cdr_key", *res.R2CdrKey)
		}
		if res.R2PdfKey != nil {
			add("r2_pdf_key", *res.R2PdfKey)
		}
		if res.QRData != nil {
			add("qr_data", *res.QRData)
		}
		if res.MarkSent {
			set = append(set, "sent_at = COALESCE(sent_at, now())")
		}
		if res.MarkAccepted {
			set = append(set, "accepted_at = now()")
		}
		args = append(args, docID)
		sql := fmt.Sprintf(
			"UPDATE issued_documents SET %s WHERE id = $%d RETURNING %s",
			strings.Join(set, ", "), len(args), issuedDocumentColumns,
		)
		return scanIssuedDocument(tx.QueryRow(ctx, sql, args...), &doc)
	})
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// GetIssuedDocumentItems returns the line items for a document, ordered by
// line_number.
func (p *Pool) GetIssuedDocumentItems(ctx context.Context, docID string) ([]model.IssuedDocumentItem, error) {
	var out []model.IssuedDocumentItem
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, document_id, line_number, description, quantity, unit_code,
			       unit_price, unit_price_with_tax, tax_exemption_reason_code,
			       igv_amount, isc_amount, isc_tier_range, discount_amount, line_total, price_type_code,
			       created_at
			FROM issued_document_items
			WHERE document_id = $1
			ORDER BY line_number
		`, docID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var it model.IssuedDocumentItem
			if err := rows.Scan(&it.ID, &it.DocumentID, &it.LineNumber, &it.Description,
				&it.Quantity, &it.UnitCode, &it.UnitPrice, &it.UnitPriceWithTax,
				&it.TaxExemptionReasonCode, &it.IgvAmount, &it.IscAmount, &it.IscTierRange,
				&it.DiscountAmount, &it.LineTotal, &it.PriceTypeCode, &it.CreatedAt); err != nil {
				return err
			}
			out = append(out, it)
		}
		return rows.Err()
	})
	return out, err
}
