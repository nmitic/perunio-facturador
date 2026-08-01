package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ReportFilter is the common date-range + currency filter every money report
// takes. Dates are YYYY-MM-DD (inclusive). Currency is a 3-letter code; totals
// are always scoped to a single currency because summing PEN and USD is
// meaningless.
type ReportFilter struct {
	From     string
	To       string
	Currency string
}

// acceptedStatuses are the only statuses that count as real, SUNAT-accepted
// operations for the money reports. Drafts, pending/sending, rejected and voided
// documents are not revenue.
const acceptedStatuses = `('accepted','accepted_with_observations')`

// moneyScope is the WHERE body shared by every money report: the company +
// current-environment scope, restricted to accepted documents in the date range
// and currency. `$1` company, `$2` from, `$3` to, `$4` currency. `alias` is the
// issued_documents alias in the query.
func moneyScope(alias string) string {
	a := alias
	if a != "" {
		a += "."
	}
	return fmt.Sprintf(
		"%[1]s AND %[2]sstatus IN %[3]s AND %[2]sissue_date BETWEEN $2 AND $3 AND %[2]scurrency_code = $4",
		docScope(alias), a, acceptedStatuses,
	)
}

// signedAmount returns a SQL expression that applies the report sign convention
// to `col`: notas de crédito (doc_type 07) count negative, everything else
// (facturas 01, boletas 03, notas de débito 08) positive. `alias` qualifies both
// doc_type and the column.
func signedAmount(alias, col string) string {
	a := alias
	if a != "" {
		a += "."
	}
	return fmt.Sprintf("CASE WHEN %[1]sdoc_type = '07' THEN -%[1]s%[2]s ELSE %[1]s%[2]s END", a, col)
}

// ─── Sales ────────────────────────────────────────────────────────────────────

// SalesSummary is the high-level ventas overview for a period.
type SalesSummary struct {
	Currency     string `json:"currency"`
	Revenue      string `json:"revenue"` // net of notas de crédito
	Igv          string `json:"igv"`     // net of notas de crédito
	Facturas     int    `json:"facturas"`
	Boletas      int    `json:"boletas"`
	NotasCredito int    `json:"notasCredito"`
	NotasDebito  int    `json:"notasDebito"`
	AverageSale  string `json:"averageSale"` // avg factura/boleta gross
}

// GetSalesSummary aggregates the ventas overview in one pass.
func (p *Pool) GetSalesSummary(ctx context.Context, companyID string, f ReportFilter) (SalesSummary, error) {
	out := SalesSummary{Currency: f.Currency}
	sqlStr := fmt.Sprintf(`
		SELECT
			COALESCE(SUM(%[1]s),0)::numeric(14,2)::text AS revenue,
			COALESCE(SUM(%[2]s),0)::numeric(14,2)::text AS igv,
			COUNT(*) FILTER (WHERE doc_type = '01')::int AS facturas,
			COUNT(*) FILTER (WHERE doc_type = '03')::int AS boletas,
			COUNT(*) FILTER (WHERE doc_type = '07')::int AS notas_credito,
			COUNT(*) FILTER (WHERE doc_type = '08')::int AS notas_debito,
			COALESCE(
				SUM(total_amount) FILTER (WHERE doc_type IN ('01','03'))
				/ NULLIF(COUNT(*) FILTER (WHERE doc_type IN ('01','03')), 0), 0
			)::numeric(14,2)::text AS average_sale
		FROM issued_documents
		WHERE %[3]s`,
		signedAmount("", "total_amount"), signedAmount("", "total_igv"), moneyScope(""))

	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, sqlStr, companyID, f.From, f.To, f.Currency).Scan(
			&out.Revenue, &out.Igv, &out.Facturas, &out.Boletas,
			&out.NotasCredito, &out.NotasDebito, &out.AverageSale,
		)
	})
	return out, err
}

// SalesSeriesPoint is one time bucket in a sales-over-time series.
type SalesSeriesPoint struct {
	Bucket string `json:"bucket"` // YYYY-MM-DD (start of the bucket)
	Total  string `json:"total"`
	Igv    string `json:"igv"`
	Count  int    `json:"count"`
}

// GetSalesSeries returns net sales bucketed by day, month or year. `bucket` must
// be one of "day", "month", "year" (validated by the caller).
func (p *Pool) GetSalesSeries(ctx context.Context, companyID, bucket string, f ReportFilter) ([]SalesSeriesPoint, error) {
	sqlStr := fmt.Sprintf(`
		SELECT to_char(date_trunc($5, issue_date), 'YYYY-MM-DD') AS bucket,
		       COALESCE(SUM(%[1]s),0)::numeric(14,2)::text AS total,
		       COALESCE(SUM(%[2]s),0)::numeric(14,2)::text AS igv,
		       COUNT(*)::int AS cnt
		FROM issued_documents
		WHERE %[3]s
		GROUP BY 1
		ORDER BY 1`,
		signedAmount("", "total_amount"), signedAmount("", "total_igv"), moneyScope(""))

	out := []SalesSeriesPoint{}
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sqlStr, companyID, f.From, f.To, f.Currency, bucket)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var pt SalesSeriesPoint
			if err := rows.Scan(&pt.Bucket, &pt.Total, &pt.Igv, &pt.Count); err != nil {
				return err
			}
			out = append(out, pt)
		}
		return rows.Err()
	})
	return out, err
}

// ─── Customers ──────────────────────────────────────────────────────────────

// CustomerRankingRow is one customer in the ranking.
type CustomerRankingRow struct {
	DocNumber string `json:"docNumber"`
	Name      string `json:"name"`
	Revenue   string `json:"revenue"`
	Documents int    `json:"documents"`
	Average   string `json:"average"`
}

// GetCustomerRanking ranks customers by revenue, document count or average
// invoice. `orderBy` must be one of "revenue", "count", "average".
func (p *Pool) GetCustomerRanking(ctx context.Context, companyID, orderBy string, limit int, f ReportFilter) ([]CustomerRankingRow, error) {
	order := map[string]string{
		"revenue": "revenue DESC",
		"count":   "documents DESC",
		"average": "average DESC",
	}[orderBy]
	if order == "" {
		order = "revenue DESC"
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	sqlStr := fmt.Sprintf(`
		SELECT customer_doc_number,
		       MAX(customer_name) AS name,
		       COALESCE(SUM(%[1]s),0)::numeric(14,2)::text AS revenue,
		       COUNT(*)::int AS documents,
		       COALESCE(SUM(%[1]s) / NULLIF(COUNT(*),0),0)::numeric(14,2)::text AS average
		FROM issued_documents
		WHERE %[2]s
		GROUP BY customer_doc_number
		ORDER BY %[3]s
		LIMIT $5`,
		signedAmount("", "total_amount"), moneyScope(""), order)

	out := []CustomerRankingRow{}
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sqlStr, companyID, f.From, f.To, f.Currency, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c CustomerRankingRow
			if err := rows.Scan(&c.DocNumber, &c.Name, &c.Revenue, &c.Documents, &c.Average); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// ─── Products ───────────────────────────────────────────────────────────────

// ProductRankingRow is one product (or unmatched description) in the ranking.
type ProductRankingRow struct {
	Name     string `json:"name"`
	Quantity string `json:"quantity"`
	Revenue  string `json:"revenue"`
}

// GetProductRanking ranks products by quantity sold or revenue. Lines are
// grouped by their producto link when present, falling back to the normalized
// description for manual/legacy lines. Gratuito lines (price_type_code 02) count
// toward quantity but not revenue. `orderBy` must be "quantity" or "revenue".
func (p *Pool) GetProductRanking(ctx context.Context, companyID, orderBy string, limit int, f ReportFilter) ([]ProductRankingRow, error) {
	order := map[string]string{
		"quantity": "quantity DESC",
		"revenue":  "revenue DESC",
	}[orderBy]
	if order == "" {
		order = "quantity DESC"
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	sqlStr := fmt.Sprintf(`
		SELECT COALESCE(MAX(p.nombre), MAX(i.description)) AS name,
		       COALESCE(SUM(%[1]s),0)::numeric(16,6)::text AS quantity,
		       COALESCE(SUM(%[2]s) FILTER (WHERE i.price_type_code IS DISTINCT FROM '02'),0)::numeric(14,2)::text AS revenue
		FROM issued_document_items i
		JOIN issued_documents d ON d.id = i.document_id
		LEFT JOIN productos p ON p.id = i.producto_id
		WHERE %[3]s
		GROUP BY COALESCE(i.producto_id::text, lower(trim(i.description)))
		ORDER BY %[4]s
		LIMIT $5`,
		signedItemAmount("d", "i", "quantity"), signedItemAmount("d", "i", "line_total"), moneyScope("d"), order)

	out := []ProductRankingRow{}
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sqlStr, companyID, f.From, f.To, f.Currency, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var pr ProductRankingRow
			if err := rows.Scan(&pr.Name, &pr.Quantity, &pr.Revenue); err != nil {
				return err
			}
			out = append(out, pr)
		}
		return rows.Err()
	})
	return out, err
}

// ─── Tax ──────────────────────────────────────────────────────────────────

// TaxBucket is the base + IGV for one afectación category.
type TaxBucket struct {
	Category string `json:"category"` // gravado|exonerado|inafecto|exportacion|gratuito
	Base     string `json:"base"`
	Igv      string `json:"igv"`
}

// TaxBreakdown backs the three tax reports (IGV summary, sales by IGV type, tax
// base). Base+IGV per afectación category come from the line items; the tax
// totals (IGV/ISC/other/total) come from the authoritative document totals.
type TaxBreakdown struct {
	Currency    string      `json:"currency"`
	Buckets     []TaxBucket `json:"buckets"`
	TotalIgv    string      `json:"totalIgv"`
	TotalIsc    string      `json:"totalIsc"`
	TotalOther  string      `json:"totalOther"`
	TotalAmount string      `json:"totalAmount"`
}

// GetTaxBreakdown assembles the per-category base/IGV split and the document tax
// totals for a period.
func (p *Pool) GetTaxBreakdown(ctx context.Context, companyID string, f ReportFilter) (TaxBreakdown, error) {
	out := TaxBreakdown{Currency: f.Currency, Buckets: []TaxBucket{}}

	// Category classification: gratuito lines (price_type_code 02) first, then by
	// the first char of the Cat.07 code — 1 gravado, 2 exonerado, 3 inafecto,
	// 4 exportación. A gratuito line's net base is reconstructed from the stored
	// referential inclusive unit price (unit_price_with_tax) since line_total is
	// forced to 0 for gratuito.
	bucketsSQL := fmt.Sprintf(`
		SELECT
			CASE
				WHEN i.price_type_code = '02' THEN 'gratuito'
				WHEN left(coalesce(i.tax_exemption_reason_code,'10'),1) = '2' THEN 'exonerado'
				WHEN left(coalesce(i.tax_exemption_reason_code,'10'),1) = '3' THEN 'inafecto'
				WHEN left(coalesce(i.tax_exemption_reason_code,'10'),1) = '4' THEN 'exportacion'
				ELSE 'gravado'
			END AS category,
			COALESCE(SUM(
				CASE WHEN i.price_type_code = '02'
					THEN %[1]s
					ELSE %[2]s
				END
			),0)::numeric(14,2)::text AS base,
			COALESCE(SUM(CASE WHEN i.price_type_code = '02' THEN 0 ELSE %[3]s END),0)::numeric(14,2)::text AS igv
		FROM issued_document_items i
		JOIN issued_documents d ON d.id = i.document_id
		WHERE %[4]s
		GROUP BY 1`,
		// gratuito base reconstruction (signed by doc type)
		signedGratuitoBase("d", "i"),
		// non-gratuito base = line_total (signed, column on the items table)
		signedItemAmount("d", "i", "line_total"),
		// igv (signed), only for non-gratuito
		signedItemAmount("d", "i", "igv_amount"),
		moneyScope("d"))

	totalsSQL := fmt.Sprintf(`
		SELECT
			COALESCE(SUM(%[1]s),0)::numeric(14,2)::text,
			COALESCE(SUM(%[2]s),0)::numeric(14,2)::text,
			COALESCE(SUM(%[3]s),0)::numeric(14,2)::text,
			COALESCE(SUM(%[4]s),0)::numeric(14,2)::text
		FROM issued_documents
		WHERE %[5]s`,
		signedAmount("", "total_igv"),
		signedAmount("", "COALESCE(total_isc,0)"),
		signedAmount("", "COALESCE(total_other_taxes,0)"),
		signedAmount("", "total_amount"),
		moneyScope(""))

	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, bucketsSQL, companyID, f.From, f.To, f.Currency)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b TaxBucket
			if err := rows.Scan(&b.Category, &b.Base, &b.Igv); err != nil {
				return err
			}
			out.Buckets = append(out.Buckets, b)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return tx.QueryRow(ctx, totalsSQL, companyID, f.From, f.To, f.Currency).Scan(
			&out.TotalIgv, &out.TotalIsc, &out.TotalOther, &out.TotalAmount,
		)
	})
	return out, err
}

// signedItemAmount is signedAmount for an item column `col` whose sign is driven
// by the parent document's doc_type. `dAlias` is the documents alias, `iAlias`
// the items alias.
func signedItemAmount(dAlias, iAlias, col string) string {
	return fmt.Sprintf("CASE WHEN %s.doc_type = '07' THEN -%s.%s ELSE %s.%s END",
		dAlias, iAlias, col, iAlias, col)
}

// signedGratuitoBase reconstructs a gratuito line's net referential base
// (unit_price_with_tax*qty − igv − isc), signed by the parent document's type.
func signedGratuitoBase(dAlias, iAlias string) string {
	base := fmt.Sprintf(
		"(COALESCE(%[1]s.unit_price_with_tax,0) * %[1]s.quantity - %[1]s.igv_amount - COALESCE(%[1]s.isc_amount,0))",
		iAlias)
	return fmt.Sprintf("CASE WHEN %s.doc_type = '07' THEN -%s ELSE %s END", dAlias, base, base)
}

// NoteRow is one reason-code bucket in a notas report.
type NoteRow struct {
	ReasonCode string `json:"reasonCode"`
	Count      int    `json:"count"`
	Amount     string `json:"amount"`
}

// GetNotesBreakdown groups notas de crédito (07) or débito (08) by reason code.
// `docType` must be "07" or "08". Amounts are gross (unsigned) — this report is
// about the notes themselves, not their effect on revenue.
func (p *Pool) GetNotesBreakdown(ctx context.Context, companyID, docType string, f ReportFilter) ([]NoteRow, error) {
	sqlStr := fmt.Sprintf(`
		SELECT COALESCE(credit_debit_reason_code,'') AS code,
		       COUNT(*)::int AS cnt,
		       COALESCE(SUM(total_amount),0)::numeric(14,2)::text AS amount
		FROM issued_documents
		WHERE %s AND doc_type = $5
		GROUP BY 1
		ORDER BY 1`, moneyScope(""))

	out := []NoteRow{}
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sqlStr, companyID, f.From, f.To, f.Currency, docType)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var n NoteRow
			if err := rows.Scan(&n.ReasonCode, &n.Count, &n.Amount); err != nil {
				return err
			}
			out = append(out, n)
		}
		return rows.Err()
	})
	return out, err
}

// ─── Installments (cuotas) ──────────────────────────────────────────────────

// InstallmentRow is one credit document's installment rollup.
type InstallmentRow struct {
	DocumentID   string  `json:"documentId"`
	DocType      string  `json:"docType"`
	Series       string  `json:"series"`
	Correlative  int     `json:"correlative"`
	CustomerName string  `json:"customerName"`
	Currency     string  `json:"currency"`
	TotalAmount  string  `json:"totalAmount"`
	NumCuotas    int     `json:"numCuotas"`
	Paid         string  `json:"paid"`
	Balance      string  `json:"balance"`
	NextDueDate  *string `json:"nextDueDate"`
	Status       string  `json:"status"` // pagada|parcial|pendiente
	Overdue      bool    `json:"overdue"`
}

// GetInstallmentsReport rolls up cuotas + recorded payments per credit document.
// `statusFilter` optionally narrows to "paid"|"partial"|"pending"; "overdue"
// narrows to documents with any past-due balance. Empty = all.
func (p *Pool) GetInstallmentsReport(ctx context.Context, companyID, statusFilter string, f ReportFilter) ([]InstallmentRow, error) {
	sqlStr := fmt.Sprintf(`
		WITH cuotas AS (
			-- monto/fechaVencimiento are extracted as text and cast explicitly:
			-- the cuotas jsonb stores monto as a JSON *string* ("123.45"), so a
			-- direct numeric/date recordset column would depend on jsonb_to_recordset
			-- coercing a JSON string, which text extraction + ::cast avoids.
			SELECT d.id AS document_id, d.doc_type, d.series, d.correlative,
			       d.customer_name, d.currency_code, d.total_amount,
			       c.numero, c.monto::numeric AS monto, c."fechaVencimiento"::date AS vence,
			       COALESCE(SUM(pmt.amount),0) AS pagado
			FROM issued_documents d
			CROSS JOIN LATERAL jsonb_to_recordset(d.cuotas)
			     AS c(numero int, monto text, "fechaVencimiento" text)
			LEFT JOIN document_installment_payments pmt
			     ON pmt.document_id = d.id AND pmt.cuota_numero = c.numero
			WHERE d.forma_pago = 'credito' AND d.cuotas IS NOT NULL AND %s
			GROUP BY d.id, d.doc_type, d.series, d.correlative, d.customer_name,
			         d.currency_code, d.total_amount, c.numero, c.monto, c."fechaVencimiento"
		)
		SELECT document_id, doc_type, series, correlative, customer_name, currency_code,
		       total_amount::numeric(14,2)::text,
		       COUNT(*)::int AS num_cuotas,
		       COALESCE(SUM(pagado),0)::numeric(14,2)::text AS paid,
		       COALESCE(SUM(monto - pagado),0)::numeric(14,2)::text AS balance,
		       to_char(MIN(vence) FILTER (WHERE monto - pagado > 0.005), 'YYYY-MM-DD') AS next_due,
		       (COALESCE(SUM(monto - pagado),0) <= 0.005) AS fully_paid,
		       (COALESCE(SUM(pagado),0) > 0.005) AS any_paid,
		       BOOL_OR(monto - pagado > 0.005 AND vence < current_date) AS overdue
		FROM cuotas
		GROUP BY document_id, doc_type, series, correlative, customer_name, currency_code, total_amount
		ORDER BY overdue DESC, next_due NULLS LAST`, moneyScope("d"))

	rowsOut := []InstallmentRow{}
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sqlStr, companyID, f.From, f.To, f.Currency)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r InstallmentRow
			var fullyPaid, anyPaid bool
			if err := rows.Scan(&r.DocumentID, &r.DocType, &r.Series, &r.Correlative,
				&r.CustomerName, &r.Currency, &r.TotalAmount, &r.NumCuotas,
				&r.Paid, &r.Balance, &r.NextDueDate, &fullyPaid, &anyPaid, &r.Overdue); err != nil {
				return err
			}
			switch {
			case fullyPaid:
				r.Status = "pagada"
			case anyPaid:
				r.Status = "parcial"
			default:
				r.Status = "pendiente"
			}
			rowsOut = append(rowsOut, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	// Status filter applied in Go — the per-company/period set is small, and the
	// derived status is simpler to express here than as SQL HAVING.
	if statusFilter == "" {
		return rowsOut, nil
	}
	filtered := rowsOut[:0]
	for _, r := range rowsOut {
		keep := false
		switch statusFilter {
		case "paid":
			keep = r.Status == "pagada"
		case "partial":
			keep = r.Status == "parcial"
		case "pending":
			keep = r.Status == "pendiente"
		case "overdue":
			keep = r.Overdue
		default:
			keep = true
		}
		if keep {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

// ─── Anticipos ──────────────────────────────────────────────────────────────

// AnticipoReportRow is one accepted anticipo comprobante (operation_type 0104)
// with its derived applied state. Status is "aplicado" when an accepted
// document carries this anticipo in its `anticipos` jsonb — matched by
// sourceDocumentId (historial picks) or by docId string (manual entries) — and
// "pendiente" otherwise. The AppliedBy* fields describe that regularización.
type AnticipoReportRow struct {
	DocumentID        string  `json:"documentId"`
	DocType           string  `json:"docType"`
	Series            string  `json:"series"`
	Correlative       int     `json:"correlative"`
	IssueDate         string  `json:"issueDate"`
	CustomerDocNumber string  `json:"customerDocNumber"`
	CustomerName      string  `json:"customerName"`
	Currency          string  `json:"currency"`
	TotalAmount       string  `json:"totalAmount"`
	Status            string  `json:"status"` // "pendiente" | "aplicado"
	AppliedByID       *string `json:"appliedById"`
	AppliedByCode     *string `json:"appliedByCode"` // "F001-00000050"
	AppliedByDate     *string `json:"appliedByDate"` // YYYY-MM-DD

	// The venta con anticipos (deal) this row belongs to, so the report can group
	// without a second round-trip. All nil for anticipos emitted before the deal
	// concept existed, or emitted outside one — those render under "Sin agrupar".
	// VentaRegularizado is the total already documented by the deal's accepted
	// factura(s) final — its saldo, since a regularización deducts the anticipos.
	// It closes the group: cobrado + regularizado = acordado, so a regularizada
	// venta stops reporting a saldo por cobrar. "0.00" while none exists.
	VentaAnticipoID   *string `json:"ventaAnticipoId"`
	VentaNombre       *string `json:"ventaNombre"`
	VentaAcordado     *string `json:"ventaAcordado"`
	VentaRegularizado *string `json:"ventaRegularizado"`
}

// GetAnticiposReport lists the accepted anticipo comprobantes in the period
// with their applied state. `statusFilter` optionally narrows to
// "pendiente"|"aplicado"; `customerDoc` optionally narrows to one customer's
// anticipos (used by the emission form to suggest pending anticipos). Empty =
// all.
func (p *Pool) GetAnticiposReport(ctx context.Context, companyID, statusFilter, customerDoc string, f ReportFilter) ([]AnticipoReportRow, error) {
	args := []any{companyID, f.From, f.To, f.Currency}
	customerCond := ""
	if customerDoc != "" {
		args = append(args, customerDoc)
		customerCond = fmt.Sprintf(" AND d.customer_doc_number = $%d", len(args))
	}
	// The applying regularización: any accepted doc in the same company +
	// environment whose anticipos array references this row. sourceDocumentId is
	// the robust link (historial picks); the docId string match covers manual
	// entries. Newest first so a re-emission after a voided attempt wins.
	sqlStr := fmt.Sprintf(`
		SELECT d.id, d.doc_type, d.series, d.correlative,
		       to_char(d.issue_date, 'YYYY-MM-DD'),
		       d.customer_doc_number, d.customer_name, d.currency_code,
		       d.total_amount::numeric(14,2)::text,
		       app.id, app.code, app.issue_date,
		       d.venta_anticipo_id, va.nombre, va.monto_acordado::numeric(14,2)::text,
		       fin.regularizado
		FROM issued_documents d
		LEFT JOIN ventas_anticipo va ON va.id = d.venta_anticipo_id
		LEFT JOIN LATERAL (
			SELECT COALESCE(SUM(f.total_amount), 0)::numeric(14,2)::text AS regularizado
			FROM issued_documents f
			WHERE f.venta_anticipo_id = va.id
			  AND f.operation_type IS DISTINCT FROM '0104'
			  AND f.status IN %s
		) fin ON va.id IS NOT NULL
		LEFT JOIN LATERAL (
			SELECT r.id,
			       r.series || '-' || lpad(r.correlative::text, 8, '0') AS code,
			       to_char(r.issue_date, 'YYYY-MM-DD') AS issue_date
			FROM issued_documents r
			CROSS JOIN LATERAL jsonb_array_elements(r.anticipos) AS a
			WHERE r.company_id = d.company_id
			  AND r.sunat_environment = d.sunat_environment
			  AND r.status IN %s
			  AND (a->>'sourceDocumentId' = d.id::text
			       OR a->>'docId' = d.series || '-' || lpad(d.correlative::text, 8, '0'))
			ORDER BY r.issue_date DESC, r.created_at DESC
			LIMIT 1
		) app ON true
		WHERE %s AND d.operation_type = '0104'%s
		ORDER BY (app.id IS NOT NULL), d.issue_date DESC, d.created_at DESC`,
		acceptedStatuses, acceptedStatuses, moneyScope("d"), customerCond)

	out := []AnticipoReportRow{}
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sqlStr, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r AnticipoReportRow
			if err := rows.Scan(&r.DocumentID, &r.DocType, &r.Series, &r.Correlative,
				&r.IssueDate, &r.CustomerDocNumber, &r.CustomerName, &r.Currency,
				&r.TotalAmount, &r.AppliedByID, &r.AppliedByCode, &r.AppliedByDate,
				&r.VentaAnticipoID, &r.VentaNombre, &r.VentaAcordado, &r.VentaRegularizado); err != nil {
				return err
			}
			if r.AppliedByID != nil {
				r.Status = "aplicado"
			} else {
				r.Status = "pendiente"
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	if statusFilter == "" {
		return out, nil
	}
	filtered := out[:0]
	for _, r := range out {
		if r.Status == statusFilter {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

// ─── SUNAT submissions ──────────────────────────────────────────────────────

// SunatStatusRow is the document count for one SUNAT status.
type SunatStatusRow struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

// GetSunatSubmissions counts documents by SUNAT status in the period. Unlike the
// money reports this includes every status (drafts, rejected, voided, …) and is
// currency-agnostic — it reports delivery outcome, not revenue.
func (p *Pool) GetSunatSubmissions(ctx context.Context, companyID string, f ReportFilter) ([]SunatStatusRow, error) {
	sqlStr := fmt.Sprintf(`
		SELECT status, COUNT(*)::int
		FROM issued_documents
		WHERE %s AND issue_date BETWEEN $2 AND $3
		GROUP BY status
		ORDER BY status`, docScope(""))

	out := []SunatStatusRow{}
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sqlStr, companyID, f.From, f.To)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s SunatStatusRow
			if err := rows.Scan(&s.Status, &s.Count); err != nil {
				return err
			}
			out = append(out, s)
		}
		return rows.Err()
	})
	return out, err
}
