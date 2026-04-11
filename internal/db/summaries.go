package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/perunio/perunio-facturador/internal/auth"
	"github.com/perunio/perunio-facturador/internal/model"
)

// CreateDailySummary selects every accepted boleta (docType='03') for
// referenceDate that is not already in a summary, inserts a daily_summaries
// row, and links the boletas via daily_summary_items. Mirrors
// FacturadorService.createDailySummary in the Node.js backend.
func (p *Pool) CreateDailySummary(ctx context.Context, companyID, referenceDate, summaryID string) (*model.DailySummary, error) {
	tenantID, ok := auth.TenantIDFromContext(ctx)
	if !ok {
		return nil, ErrTenantContextMissing
	}
	var summary model.DailySummary
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		// Find unsummarized accepted boletas for the date.
		rows, err := tx.Query(ctx, `
			SELECT d.id
			FROM issued_documents d
			LEFT JOIN daily_summary_items dsi ON dsi.document_id = d.id
			WHERE d.company_id = $1
			  AND d.doc_type = '03'
			  AND d.issue_date = $2
			  AND d.status = 'accepted'
			  AND dsi.id IS NULL
		`, companyID, referenceDate)
		if err != nil {
			return err
		}
		var docIDs []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			docIDs = append(docIDs, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(docIDs) == 0 {
			return ErrNoBoletas
		}

		// Insert the summary row.
		if err := tx.QueryRow(ctx,
			"INSERT INTO daily_summaries (tenant_id, company_id, summary_id, reference_date, total_documents)"+
				" VALUES ($1, $2, $3, $4, $5) RETURNING "+dailySummaryColumns,
			tenantID, companyID, summaryID, referenceDate, len(docIDs),
		).Scan(
			&summary.ID, &summary.TenantID, &summary.CompanyID, &summary.SummaryID, &summary.ReferenceDate, &summary.Status,
			&summary.SunatTicket, &summary.SunatResponseCode, &summary.SunatResponseDescription,
			&summary.R2XmlKey, &summary.R2SignedXmlKey, &summary.R2CdrKey,
			&summary.TotalDocuments, &summary.SentAt, &summary.AcceptedAt, &summary.CreatedAt, &summary.UpdatedAt,
		); err != nil {
			return err
		}

		// Link each boleta with condition_code '1' (add).
		linkRows := make([][]any, 0, len(docIDs))
		for _, id := range docIDs {
			linkRows = append(linkRows, []any{summary.ID, id, "1"})
		}
		_, err = tx.CopyFrom(ctx,
			pgx.Identifier{"daily_summary_items"},
			[]string{"summary_id", "document_id", "condition_code"},
			pgx.CopyFromRows(linkRows),
		)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &summary, nil
}


// SummaryIssueResult captures the outcome of sending a daily summary to
// SUNAT. All pointer fields are optional.
type SummaryIssueResult struct {
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

// ApplySummaryResult writes the pipeline/poll outcome back to daily_summaries
// and returns the refreshed row.
func (p *Pool) ApplySummaryResult(ctx context.Context, summaryID string, res SummaryIssueResult) (*model.DailySummary, error) {
	var s model.DailySummary
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
		args = append(args, summaryID)
		sql := fmt.Sprintf(
			"UPDATE daily_summaries SET %s WHERE id = $%d RETURNING %s",
			strings.Join(set, ", "), len(args), dailySummaryColumns,
		)
		return scanDailySummary(tx.QueryRow(ctx, sql, args...), &s)
	})
	if err != nil {
		return nil, err
	}
	return &s, nil
}

const dailySummaryColumns = `
	id, tenant_id, company_id, summary_id, reference_date, status,
	sunat_ticket, sunat_response_code, sunat_response_description,
	r2_xml_key, r2_signed_xml_key, r2_cdr_key,
	total_documents, sent_at, accepted_at, created_at, updated_at
`

func scanDailySummary(row pgx.Row, s *model.DailySummary) error {
	return row.Scan(
		&s.ID, &s.TenantID, &s.CompanyID, &s.SummaryID, &s.ReferenceDate, &s.Status,
		&s.SunatTicket, &s.SunatResponseCode, &s.SunatResponseDescription,
		&s.R2XmlKey, &s.R2SignedXmlKey, &s.R2CdrKey,
		&s.TotalDocuments, &s.SentAt, &s.AcceptedAt, &s.CreatedAt, &s.UpdatedAt,
	)
}

// ListDailySummaries returns every summary row for the company, newest first.
func (p *Pool) ListDailySummaries(ctx context.Context, companyID string) ([]model.DailySummary, error) {
	var out []model.DailySummary
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			"SELECT "+dailySummaryColumns+
				" FROM daily_summaries WHERE company_id = $1 ORDER BY reference_date DESC",
			companyID,
		)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s model.DailySummary
			if err := scanDailySummary(rows, &s); err != nil {
				return err
			}
			out = append(out, s)
		}
		return rows.Err()
	})
	return out, err
}

// GetDailySummary fetches a single summary scoped to the company. Returns nil
// when not found.
func (p *Pool) GetDailySummary(ctx context.Context, companyID, summaryID string) (*model.DailySummary, error) {
	var sum *model.DailySummary
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			"SELECT "+dailySummaryColumns+
				" FROM daily_summaries WHERE company_id = $1 AND id = $2 LIMIT 1",
			companyID, summaryID,
		)
		var got model.DailySummary
		if err := scanDailySummary(row, &got); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		sum = &got
		return nil
	})
	return sum, err
}

// SummaryItemRow is one document linked to a daily summary, joined with the
// minimal issued_document fields the UI needs.
type SummaryItemRow struct {
	ID            string `json:"id"`
	DocumentID    string `json:"documentId"`
	ConditionCode string `json:"conditionCode"`
	DocType       string `json:"docType"`
	Series        string `json:"series"`
	Correlative   int    `json:"correlative"`
}

// SummaryIssueItem carries all the fields needed to emit one line of a
// SUNAT RC (Resumen Diario). It joins daily_summary_items with
// issued_documents so the pipeline doesn't have to re-fetch each boleta.
type SummaryIssueItem struct {
	DocumentID        string
	ConditionCode     string
	DocType           string
	Series            string
	Correlative       int
	CustomerDocType   string
	CustomerDocNumber string
	CurrencyCode      string
	TotalAmount       string
	TotalIgv          string
	TotalIsc          string
	TotalOtherTaxes   string
}

// GetDailySummaryIssueItems returns the fully-joined rows needed to build
// the RC XML for a daily summary.
func (p *Pool) GetDailySummaryIssueItems(ctx context.Context, summaryID string) ([]SummaryIssueItem, error) {
	var out []SummaryIssueItem
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT dsi.document_id, dsi.condition_code,
			       d.doc_type, d.series, d.correlative,
			       d.customer_doc_type, d.customer_doc_number,
			       d.currency_code, d.total_amount, d.total_igv,
			       COALESCE(d.total_isc, '0.00'),
			       COALESCE(d.total_other_taxes, '0.00')
			FROM daily_summary_items dsi
			JOIN issued_documents d ON d.id = dsi.document_id
			WHERE dsi.summary_id = $1
			ORDER BY d.series, d.correlative
		`, summaryID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var it SummaryIssueItem
			if err := rows.Scan(&it.DocumentID, &it.ConditionCode,
				&it.DocType, &it.Series, &it.Correlative,
				&it.CustomerDocType, &it.CustomerDocNumber,
				&it.CurrencyCode, &it.TotalAmount, &it.TotalIgv,
				&it.TotalIsc, &it.TotalOtherTaxes); err != nil {
				return err
			}
			out = append(out, it)
		}
		return rows.Err()
	})
	return out, err
}

// GetDailySummaryItems returns the documents linked to a summary.
func (p *Pool) GetDailySummaryItems(ctx context.Context, summaryID string) ([]SummaryItemRow, error) {
	var out []SummaryItemRow
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT dsi.id, dsi.document_id, dsi.condition_code,
			       d.doc_type, d.series, d.correlative
			FROM daily_summary_items dsi
			JOIN issued_documents d ON d.id = dsi.document_id
			WHERE dsi.summary_id = $1
			ORDER BY d.series, d.correlative
		`, summaryID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var it SummaryItemRow
			if err := rows.Scan(&it.ID, &it.DocumentID, &it.ConditionCode,
				&it.DocType, &it.Series, &it.Correlative); err != nil {
				return err
			}
			out = append(out, it)
		}
		return rows.Err()
	})
	return out, err
}
