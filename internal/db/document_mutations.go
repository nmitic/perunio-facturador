package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/perunio/perunio-facturador/internal/auth"
	"github.com/perunio/perunio-facturador/internal/model"
)

// CreateDocumentInput is the base payload for creating an issued_documents
// row. String numeric fields match the on-wire format used by the frontend
// and the Node.js backend (they're stored as numeric but transported as
// strings so precision isn't lost).
type CreateDocumentInput struct {
	SeriesID string

	IssueDate string
	IssueTime *string
	DueDate   *string

	CurrencyCode  string
	OperationType *string

	CustomerDocType   string
	CustomerDocNumber string
	CustomerName      string
	CustomerAddress   *string

	Subtotal           string
	TotalIgv           string
	TotalIsc           *string
	TotalOtherTaxes    *string
	TotalDiscount      *string
	TotalAmount        string
	TaxInclusiveAmount *string
	Notes              *string

	ReferenceDocType        *string
	ReferenceDocSeries      *string
	ReferenceDocCorrelative *int
	CreditDebitReasonCode   *string
	CreditDebitReasonDesc   *string

	Items []CreateDocumentItemInput
}

// CreateDocumentItemInput is one line of a new document.
type CreateDocumentItemInput struct {
	LineNumber             int
	Description            string
	Quantity               string
	UnitCode               string
	UnitPrice              string
	UnitPriceWithTax       *string
	TaxExemptionReasonCode *string
	IgvAmount              string
	IscAmount              *string
	DiscountAmount         *string
	LineTotal              string
	PriceTypeCode          *string
}

// UpdateDocumentInput holds the fields the PUT /documents handler allows.
// Only non-nil fields are updated. Items, when non-nil, replace all existing
// line items.
type UpdateDocumentInput struct {
	IssueDate *string
	IssueTime *string
	DueDate   *string

	CurrencyCode  *string
	OperationType *string

	CustomerDocType   *string
	CustomerDocNumber *string
	CustomerName      *string
	CustomerAddress   *string

	Subtotal           *string
	TotalIgv           *string
	TotalIsc           *string
	TotalOtherTaxes    *string
	TotalDiscount      *string
	TotalAmount        *string
	TaxInclusiveAmount *string
	Notes              *string

	ReferenceDocType        *string
	ReferenceDocSeries      *string
	ReferenceDocCorrelative *int
	CreditDebitReasonCode   *string
	CreditDebitReasonDesc   *string

	Items []CreateDocumentItemInput // nil = don't touch; non-nil = replace all
}

// CreateDocumentWithItems atomically allocates the next correlative for the
// given series and inserts the document with its line items. Mirrors
// FacturadorService.createDocumentWithItems in the Node.js backend.
func (p *Pool) CreateDocumentWithItems(ctx context.Context, companyID string, in CreateDocumentInput) (*model.IssuedDocument, error) {
	tenantID, ok := auth.TenantIDFromContext(ctx)
	if !ok {
		return nil, ErrTenantContextMissing
	}

	var doc model.IssuedDocument
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		// 1. Atomically bump the correlative and read back the previous value.
		var correlative int
		var seriesCode, docType string
		err := tx.QueryRow(ctx, `
			UPDATE document_series
			SET next_correlative = next_correlative + 1, updated_at = now()
			WHERE id = $1 AND company_id = $2 AND is_active = true
			RETURNING next_correlative - 1, series, doc_type
		`, in.SeriesID, companyID).Scan(&correlative, &seriesCode, &docType)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrSeriesInactive
			}
			return err
		}

		// 2. Insert the document.
		err = tx.QueryRow(ctx, `
			INSERT INTO issued_documents (
				tenant_id, company_id, series_id, doc_type, series, correlative, status,
				issue_date, issue_time, due_date,
				currency_code, operation_type,
				customer_doc_type, customer_doc_number, customer_name, customer_address,
				subtotal, total_igv, total_isc, total_other_taxes, total_discount, total_amount,
				tax_inclusive_amount, notes,
				reference_doc_type, reference_doc_series, reference_doc_correlative,
				credit_debit_reason_code, credit_debit_reason_desc
			) VALUES (
				$1, $2, $3, $4, $5, $6, 'draft',
				$7, $8, $9,
				$10, $11,
				$12, $13, $14, $15,
				$16, $17, $18, $19, $20, $21,
				$22, $23,
				$24, $25, $26,
				$27, $28
			)
			RETURNING `+issuedDocumentColumns,
			tenantID, companyID, in.SeriesID, docType, seriesCode, correlative,
			in.IssueDate, in.IssueTime, in.DueDate,
			in.CurrencyCode, in.OperationType,
			in.CustomerDocType, in.CustomerDocNumber, in.CustomerName, in.CustomerAddress,
			in.Subtotal, in.TotalIgv, in.TotalIsc, in.TotalOtherTaxes, in.TotalDiscount, in.TotalAmount,
			in.TaxInclusiveAmount, in.Notes,
			in.ReferenceDocType, in.ReferenceDocSeries, in.ReferenceDocCorrelative,
			in.CreditDebitReasonCode, in.CreditDebitReasonDesc,
		).Scan(
			&doc.ID, &doc.TenantID, &doc.CompanyID, &doc.SeriesID, &doc.DocType, &doc.Series, &doc.Correlative, &doc.Status,
			&doc.IssueDate, &doc.IssueTime, &doc.DueDate,
			&doc.CurrencyCode, &doc.OperationType,
			&doc.CustomerDocType, &doc.CustomerDocNumber, &doc.CustomerName, &doc.CustomerAddress,
			&doc.Subtotal, &doc.TotalIgv, &doc.TotalIsc, &doc.TotalOtherTaxes, &doc.TotalDiscount, &doc.TotalAmount,
			&doc.TaxInclusiveAmount, &doc.Notes,
			&doc.ReferenceDocType, &doc.ReferenceDocSeries, &doc.ReferenceDocCorrelative,
			&doc.CreditDebitReasonCode, &doc.CreditDebitReasonDesc,
			&doc.SunatResponseCode, &doc.SunatResponseDescription, &doc.SunatTicket,
			&doc.R2XmlKey, &doc.R2SignedXmlKey, &doc.R2ZipKey, &doc.R2CdrKey, &doc.R2PdfKey,
			&doc.QrData,
			&doc.SentAt, &doc.AcceptedAt, &doc.CreatedAt, &doc.UpdatedAt,
		)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return ErrDuplicate
			}
			return err
		}

		// 3. Insert items (non-empty per request validation).
		if err := insertDocumentItems(ctx, tx, doc.ID, in.Items); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// UpdateDraftDocument updates mutable fields on a draft doc and (optionally)
// replaces its line items. Returns ErrNotFound / ErrNotDraft accordingly.
func (p *Pool) UpdateDraftDocument(ctx context.Context, docID string, in UpdateDocumentInput) (*model.IssuedDocument, error) {
	var doc model.IssuedDocument
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(ctx,
			`SELECT status FROM issued_documents WHERE id = $1 LIMIT 1`, docID,
		).Scan(&status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if status != "draft" {
			return ErrNotDraft
		}

		// Dynamic SET clause from the non-nil fields.
		set, args := buildUpdateSet(in)
		set = append(set, "updated_at = now()")
		args = append(args, docID)
		sql := fmt.Sprintf(
			"UPDATE issued_documents SET %s WHERE id = $%d RETURNING %s",
			strings.Join(set, ", "), len(args), issuedDocumentColumns,
		)

		if err := scanIssuedDocument(tx.QueryRow(ctx, sql, args...), &doc); err != nil {
			return err
		}

		if in.Items != nil {
			if _, err := tx.Exec(ctx,
				`DELETE FROM issued_document_items WHERE document_id = $1`, docID,
			); err != nil {
				return err
			}
			if err := insertDocumentItems(ctx, tx, docID, in.Items); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// DeleteDraftDocument removes a draft row (cascade deletes items). Returns
// ErrNotFound / ErrNotDraft accordingly.
func (p *Pool) DeleteDraftDocument(ctx context.Context, docID string) error {
	return p.WithTenant(ctx, func(tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(ctx,
			`SELECT status FROM issued_documents WHERE id = $1 LIMIT 1`, docID,
		).Scan(&status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if status != "draft" {
			return ErrNotDraft
		}
		_, err := tx.Exec(ctx, `DELETE FROM issued_documents WHERE id = $1`, docID)
		return err
	})
}

// buildUpdateSet emits the SET-clause fragments and positional args matching
// the non-nil fields of UpdateDocumentInput.
func buildUpdateSet(in UpdateDocumentInput) ([]string, []any) {
	var set []string
	var args []any
	add := func(col string, val any) {
		args = append(args, val)
		set = append(set, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	if in.IssueDate != nil {
		add("issue_date", *in.IssueDate)
	}
	if in.IssueTime != nil {
		add("issue_time", *in.IssueTime)
	}
	if in.DueDate != nil {
		add("due_date", *in.DueDate)
	}
	if in.CurrencyCode != nil {
		add("currency_code", *in.CurrencyCode)
	}
	if in.OperationType != nil {
		add("operation_type", *in.OperationType)
	}
	if in.CustomerDocType != nil {
		add("customer_doc_type", *in.CustomerDocType)
	}
	if in.CustomerDocNumber != nil {
		add("customer_doc_number", *in.CustomerDocNumber)
	}
	if in.CustomerName != nil {
		add("customer_name", *in.CustomerName)
	}
	if in.CustomerAddress != nil {
		add("customer_address", *in.CustomerAddress)
	}
	if in.Subtotal != nil {
		add("subtotal", *in.Subtotal)
	}
	if in.TotalIgv != nil {
		add("total_igv", *in.TotalIgv)
	}
	if in.TotalIsc != nil {
		add("total_isc", *in.TotalIsc)
	}
	if in.TotalOtherTaxes != nil {
		add("total_other_taxes", *in.TotalOtherTaxes)
	}
	if in.TotalDiscount != nil {
		add("total_discount", *in.TotalDiscount)
	}
	if in.TotalAmount != nil {
		add("total_amount", *in.TotalAmount)
	}
	if in.TaxInclusiveAmount != nil {
		add("tax_inclusive_amount", *in.TaxInclusiveAmount)
	}
	if in.Notes != nil {
		add("notes", *in.Notes)
	}
	if in.ReferenceDocType != nil {
		add("reference_doc_type", *in.ReferenceDocType)
	}
	if in.ReferenceDocSeries != nil {
		add("reference_doc_series", *in.ReferenceDocSeries)
	}
	if in.ReferenceDocCorrelative != nil {
		add("reference_doc_correlative", *in.ReferenceDocCorrelative)
	}
	if in.CreditDebitReasonCode != nil {
		add("credit_debit_reason_code", *in.CreditDebitReasonCode)
	}
	if in.CreditDebitReasonDesc != nil {
		add("credit_debit_reason_desc", *in.CreditDebitReasonDesc)
	}
	return set, args
}

func insertDocumentItems(ctx context.Context, tx pgx.Tx, docID string, items []CreateDocumentItemInput) error {
	if len(items) == 0 {
		return nil
	}
	const insertSQL = `
		INSERT INTO issued_document_items (
			document_id, line_number, description, quantity, unit_code,
			unit_price, unit_price_with_tax, tax_exemption_reason_code,
			igv_amount, isc_amount, discount_amount, line_total, price_type_code
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`

	batch := &pgx.Batch{}
	for i, it := range items {
		line := it.LineNumber
		if line == 0 {
			line = i + 1
		}
		batch.Queue(insertSQL,
			docID, line, it.Description, it.Quantity, it.UnitCode,
			it.UnitPrice, it.UnitPriceWithTax, it.TaxExemptionReasonCode,
			it.IgvAmount, it.IscAmount, it.DiscountAmount, it.LineTotal, it.PriceTypeCode,
		)
	}
	return tx.SendBatch(ctx, batch).Close()
}
