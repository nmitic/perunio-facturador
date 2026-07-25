package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/perunio/perunio-facturador/internal/model"
)

// DocumentListFilter narrows ListIssuedDocuments. Empty fields are ignored.
type DocumentListFilter struct {
	DocType string
	Status  string
	// Customer is a free-text search matched case-insensitively against both the
	// customer name and the customer doc number (RUC/DNI). Empty = all.
	Customer string
	// Payment narrows by derived payment status: "pagado" | "parcial" |
	// "pendiente" | "vencido" (any past-due cuota with balance). Empty = all.
	Payment string
	Page    int
	Limit   int
}

// paymentRollupJoin is a LATERAL subquery over `issued_documents d` that derives
// a unified payment status per row. For contado docs it reflects paid_at; for
// crédito docs it rolls up the cuotas schedule against recorded installment
// payments (mirrors GetInstallmentsReport so historial and reports agree).
// Exposes pay.payment_status ("pagado"|"parcial"|"pendiente") and pay.overdue.
// Adds no query args, so it doesn't shift positional parameters.
const paymentRollupJoin = `
	LEFT JOIN LATERAL (
		SELECT
			CASE
				WHEN d.forma_pago = 'credito' AND d.cuotas IS NOT NULL THEN
					CASE WHEN r.fully_paid THEN 'pagado'
					     WHEN r.any_paid   THEN 'parcial'
					     ELSE 'pendiente' END
				WHEN d.paid_at IS NOT NULL THEN 'pagado'
				-- A contado doc that hasn't been marked paid has no payment signal:
				-- NULL, not 'pendiente', so "por cobrar" (pendiente) matches only
				-- crédito docs with an outstanding balance (mirrors the badges,
				-- which show nothing for an un-marked contado doc).
				ELSE NULL
			END AS payment_status,
			COALESCE(r.overdue, false) AS overdue
		FROM (
			SELECT
				(COALESCE(SUM(cq.monto - cq.pagado), 0) <= 0.005) AS fully_paid,
				(COALESCE(SUM(cq.pagado), 0) > 0.005) AS any_paid,
				COALESCE(BOOL_OR(cq.monto - cq.pagado > 0.005 AND cq.vence < current_date), false) AS overdue
			FROM (
				SELECT c.numero,
				       c.monto::numeric AS monto,
				       c."fechaVencimiento"::date AS vence,
				       COALESCE(SUM(pmt.amount), 0) AS pagado
				FROM jsonb_to_recordset(COALESCE(d.cuotas, '[]'::jsonb))
				     AS c(numero int, monto text, "fechaVencimiento" text)
				LEFT JOIN document_installment_payments pmt
				     ON pmt.document_id = d.id AND pmt.cuota_numero = c.numero
				GROUP BY c.numero, c.monto, c."fechaVencimiento"
			) cq
		) r
	) pay ON true`

// DocumentListResult is a paginated slice of issued documents plus the total
// count for the same filter.
type DocumentListResult struct {
	Documents []model.IssuedDocument `json:"documents"`
	Total     int                    `json:"total"`
}

// docScope restricts a query over issued_documents to one company AND its
// *current* SUNAT environment. `$1` must be the company id. `alias` is the
// table alias in the query (""for an unaliased `FROM issued_documents`, "d" in a
// join) so the column references never go ambiguous against a joined table that
// also has a company_id (e.g. productos). Sandbox/test documents (issued while
// the company was in "beta") stay in the table but disappear from the historial
// and every report once the company switches to "production", and vice-versa —
// a correlated subquery so switching needs no row rewrite. Shared by
// ListIssuedDocuments and the reports so the historial and the reports can never
// disagree about which documents exist.
func docScope(alias string) string {
	if alias != "" {
		alias += "."
	}
	return fmt.Sprintf(
		"%[1]scompany_id = $1 AND %[1]ssunat_environment = (SELECT sunat_environment FROM companies WHERE id = $1)",
		alias,
	)
}

const issuedDocumentColumns = `
	id, tenant_id, company_id, series_id, doc_type, series, correlative, status,
	sunat_environment,
	issue_date, issue_time, due_date,
	currency_code, exchange_rate, operation_type,
	customer_doc_type, customer_doc_number, customer_name, customer_address,
	subtotal, total_igv, total_isc, total_other_taxes, total_discount, global_discount, total_amount,
	tax_inclusive_amount, notes,
	forma_pago, cuotas,
	paid_at, payment_method, payment_reference, payment_notes,
	reference_doc_type, reference_doc_series, reference_doc_correlative,
	credit_debit_reason_code, credit_debit_reason_desc,
	detraccion_codigo, detraccion_porcentaje, detraccion_monto, detraccion_cuenta_bn,
	sunat_response_code, sunat_response_description, sunat_ticket, sunat_observations,
	r2_xml_key, r2_signed_xml_key, r2_zip_key, r2_cdr_key, r2_pdf_key,
	qr_data,
	sent_at, accepted_at, created_at, updated_at,
	-- Provenance: which kind of schedule emitted this comprobante. Stamped once at
	-- draft creation on the row (issued_documents.origin), not derived — so it
	-- survives deletion of the schedule that produced it, which cascade-deletes the
	-- comprobante_schedule_runs rows the old lookup depended on.
	--
	-- 'manual' collapses to NULL on the wire so the historial reports a hand-issued
	-- comprobante as a nil ScheduleOrigin and the UI renders no badge.
	NULLIF(origin, 'manual') AS schedule_origin
`

// documentScanDest returns the ordered Scan destinations for issuedDocumentColumns.
// Exposed so callers that SELECT issuedDocumentColumns plus extra trailing columns
// (e.g. the historial's derived payment rollup) can append their own destinations
// without duplicating this ~45-field list. cuotasRaw/observationsRaw receive the
// raw jsonb, decoded by finishDocumentScan.
func documentScanDest(d *model.IssuedDocument, cuotasRaw, observationsRaw *[]byte) []any {
	return []any{
		&d.ID, &d.TenantID, &d.CompanyID, &d.SeriesID, &d.DocType, &d.Series, &d.Correlative, &d.Status,
		&d.SunatEnvironment,
		&d.IssueDate, &d.IssueTime, &d.DueDate,
		&d.CurrencyCode, &d.ExchangeRate, &d.OperationType,
		&d.CustomerDocType, &d.CustomerDocNumber, &d.CustomerName, &d.CustomerAddress,
		&d.Subtotal, &d.TotalIgv, &d.TotalIsc, &d.TotalOtherTaxes, &d.TotalDiscount, &d.GlobalDiscount, &d.TotalAmount,
		&d.TaxInclusiveAmount, &d.Notes,
		&d.FormaPago, cuotasRaw,
		&d.PaidAt, &d.PaymentMethod, &d.PaymentReference, &d.PaymentNotes,
		&d.ReferenceDocType, &d.ReferenceDocSeries, &d.ReferenceDocCorrelative,
		&d.CreditDebitReasonCode, &d.CreditDebitReasonDesc,
		&d.DetraccionCodigo, &d.DetraccionPorcentaje, &d.DetraccionMonto, &d.DetraccionCuentaBN,
		&d.SunatResponseCode, &d.SunatResponseDescription, &d.SunatTicket, observationsRaw,
		&d.R2XmlKey, &d.R2SignedXmlKey, &d.R2ZipKey, &d.R2CdrKey, &d.R2PdfKey,
		&d.QrData,
		&d.SentAt, &d.AcceptedAt, &d.CreatedAt, &d.UpdatedAt,
		&d.ScheduleOrigin,
	}
}

// finishDocumentScan decodes the raw jsonb captured by documentScanDest.
func finishDocumentScan(d *model.IssuedDocument, cuotasRaw, observationsRaw []byte) error {
	if len(cuotasRaw) > 0 {
		if err := json.Unmarshal(cuotasRaw, &d.Cuotas); err != nil {
			return fmt.Errorf("unmarshal cuotas: %w", err)
		}
	}
	if len(observationsRaw) > 0 {
		if err := json.Unmarshal(observationsRaw, &d.SunatObservations); err != nil {
			return fmt.Errorf("unmarshal sunat_observations: %w", err)
		}
	}
	return nil
}

func scanIssuedDocument(row pgx.Row, d *model.IssuedDocument) error {
	var cuotasRaw, observationsRaw []byte
	if err := row.Scan(documentScanDest(d, &cuotasRaw, &observationsRaw)...); err != nil {
		return err
	}
	return finishDocumentScan(d, cuotasRaw, observationsRaw)
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
	// Company + current-environment scope, shared with every report (see
	// docScope) so the historial and the reports agree on which documents exist.
	conditions := []string{docScope("d")}
	if filter.DocType != "" {
		args = append(args, filter.DocType)
		conditions = append(conditions, fmt.Sprintf("d.doc_type = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		conditions = append(conditions, fmt.Sprintf("d.status = $%d", len(args)))
	}
	if filter.Customer != "" {
		// Free-text search over name + doc number. LIKE metacharacters in the raw
		// input are escaped so a typed % or _ matches literally (ESCAPE '\').
		esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(filter.Customer)
		args = append(args, "%"+esc+"%")
		conditions = append(conditions, fmt.Sprintf(
			`(d.customer_name ILIKE $%[1]d ESCAPE '\' OR d.customer_doc_number ILIKE $%[1]d ESCAPE '\')`,
			len(args),
		))
	}
	// Derived payment filter (references the paymentRollupJoin columns). "vencido"
	// matches any past-due balance; the rest match the derived status directly.
	if filter.Payment == "vencido" {
		conditions = append(conditions, "pay.overdue")
	} else if filter.Payment != "" {
		args = append(args, filter.Payment)
		conditions = append(conditions, fmt.Sprintf("pay.payment_status = $%d", len(args)))
	}
	where := strings.Join(conditions, " AND ")
	// The list always needs the payment rollup (it projects payment_status/overdue
	// for the badges). The count only needs it when a payment filter is active —
	// otherwise WHERE never references `pay`, so we omit the per-row lateral from
	// the count path entirely.
	listFrom := "issued_documents d" + paymentRollupJoin
	countFrom := "issued_documents d"
	if filter.Payment != "" {
		countFrom += paymentRollupJoin
	}

	var result DocumentListResult
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		// Total count
		countSQL := "SELECT count(*)::int FROM " + countFrom + " WHERE " + where
		if err := tx.QueryRow(ctx, countSQL, args...).Scan(&result.Total); err != nil {
			return err
		}

		// Page of rows, with the two derived payment columns appended.
		listArgs := append([]any{}, args...)
		listArgs = append(listArgs, limit, offset)
		// NULLS LAST matches idx_issued_documents_company_env_created (created_at is
		// NOT NULL, so the ordering is identical either way) so the index provides
		// the order and the scan can stop after LIMIT rows instead of sorting the
		// whole company/environment set.
		listSQL := "SELECT " + issuedDocumentColumns + ", pay.payment_status, pay.overdue" +
			" FROM " + listFrom + " WHERE " + where +
			fmt.Sprintf(" ORDER BY d.created_at DESC NULLS LAST LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)

		rows, err := tx.Query(ctx, listSQL, listArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var d model.IssuedDocument
			var cuotasRaw, observationsRaw []byte
			var paymentStatus *string // NULL for an un-marked contado doc
			var overdue bool
			dest := documentScanDest(&d, &cuotasRaw, &observationsRaw)
			dest = append(dest, &paymentStatus, &overdue)
			if err := rows.Scan(dest...); err != nil {
				return err
			}
			if err := finishDocumentScan(&d, cuotasRaw, observationsRaw); err != nil {
				return err
			}
			d.PaymentStatus = paymentStatus
			d.PaymentOverdue = overdue
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
	SunatObservations        []model.Observation
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
		if len(res.SunatObservations) > 0 {
			obsJSON, err := json.Marshal(res.SunatObservations)
			if err != nil {
				return fmt.Errorf("marshal sunat_observations: %w", err)
			}
			add("sunat_observations", obsJSON)
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
		if err := scanIssuedDocument(tx.QueryRow(ctx, sql, args...), &doc); err != nil {
			return err
		}

		// A SUNAT-accepted Nota de Crédito motivo 01 (Anulación de la operación)
		// over a boleta fully annuls it — boletas have no Comunicación de Baja, so
		// this NC is their anulación. Flip the referenced boleta to 'voided' so it
		// shows as anulada and can't be annulled again. Mirrors the RA path, which
		// voids its target on acceptance.
		if res.MarkAccepted && doc.DocType == model.DocTypeNotaCredito &&
			doc.CreditDebitReasonCode != nil && *doc.CreditDebitReasonCode == "01" &&
			doc.ReferenceDocType != nil && *doc.ReferenceDocType == model.DocTypeBoleta &&
			doc.ReferenceDocSeries != nil && doc.ReferenceDocCorrelative != nil {
			if _, err := tx.Exec(ctx,
				`UPDATE issued_documents SET status = 'voided', updated_at = now()
				 WHERE company_id = $1 AND doc_type = $2 AND series = $3 AND correlative = $4
				   AND status IN ('accepted', 'accepted_with_observations')`,
				doc.CompanyID, *doc.ReferenceDocType, *doc.ReferenceDocSeries, *doc.ReferenceDocCorrelative,
			); err != nil {
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

// SetDocumentPdfKey persists the R2 key of a client-rendered PDF on an issued
// document. It touches only r2_pdf_key (and updated_at), leaving status and the
// other artifact keys untouched — unlike ApplyIssueResult, which rewrites
// status. Must be called inside the request's tenant context.
func (p *Pool) SetDocumentPdfKey(ctx context.Context, docID, key string) error {
	return p.WithTenant(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			"UPDATE issued_documents SET r2_pdf_key = $1, updated_at = now() WHERE id = $2",
			key, docID)
		return err
	})
}

// SetDocumentPayment marks a document as manually paid. It touches only the
// payment_* columns (and updated_at), leaving status and everything else
// untouched. paidAt is required; method/reference/notes are optional record-
// keeping (nil clears the respective column). Must run inside the request's
// tenant context.
func (p *Pool) SetDocumentPayment(ctx context.Context, docID, paidAt string, method, reference, notes *string) error {
	return p.WithTenant(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE issued_documents
			SET paid_at = $1, payment_method = $2, payment_reference = $3, payment_notes = $4,
			    updated_at = now()
			WHERE id = $5`,
			paidAt, method, reference, notes, docID)
		return err
	})
}

// ClearDocumentPayment reverts a document to unpaid, nulling every payment_*
// column. Must run inside the request's tenant context.
func (p *Pool) ClearDocumentPayment(ctx context.Context, docID string) error {
	return p.WithTenant(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE issued_documents
			SET paid_at = NULL, payment_method = NULL, payment_reference = NULL, payment_notes = NULL,
			    updated_at = now()
			WHERE id = $1`,
			docID)
		return err
	})
}

// GetIssuedDocumentItems returns the line items for a document, ordered by
// line_number.
func (p *Pool) GetIssuedDocumentItems(ctx context.Context, docID string) ([]model.IssuedDocumentItem, error) {
	var out []model.IssuedDocumentItem
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, document_id, line_number, producto_id, description, quantity, unit_code,
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
			if err := rows.Scan(&it.ID, &it.DocumentID, &it.LineNumber, &it.ProductoID, &it.Description,
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
