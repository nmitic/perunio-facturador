package db

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/nmitic/perunio-sunat-catalogs/sunat"

	"github.com/perunio/perunio-facturador/internal/model"
)

// reversedByNota is the condition under which comprobante `d` has been undone
// in full by an accepted nota de crédito: one whose motivo reverses the whole
// comprobante (Cat.09 full-reversal — 01 anulación de la operación, 02
// anulación por error en el RUC, 06 devolución total).
//
// The nota has to be looked up because annulling a FACTURA by nota de crédito
// leaves it 'accepted' — only a boleta is flipped to 'voided' that way (see
// ApplyIssueResult), and a Comunicación de Baja voids either.
func reversedByNota(d string) string {
	return fmt.Sprintf(`EXISTS (
			SELECT 1 FROM issued_documents nc
			WHERE nc.company_id = %[1]s.company_id
			  AND nc.sunat_environment = %[1]s.sunat_environment
			  AND nc.doc_type = '%[3]s'
			  AND nc.status IN %[2]s
			  AND nc.credit_debit_reason_code IN %[4]s
			  AND nc.reference_doc_type = %[1]s.doc_type::text
			  AND nc.reference_doc_series = %[1]s.series
			  AND nc.reference_doc_correlative = %[1]s.correlative
		)`,
		d, acceptedStatuses, sunat.Cat01NotaCredito, sqlStringList(sunat.Cat09FullReversalCodes))
}

// standsAccepted is the condition under which comprobante `d` is in force:
// SUNAT accepted it and no nota de crédito has undone it since.
func standsAccepted(d string) string {
	return fmt.Sprintf("%[1]s.status IN %[2]s AND NOT %[3]s", d, acceptedStatuses, reversedByNota(d))
}

// deductsAnticipos is the condition under which comprobante `r` still deducts
// the anticipos listed in its `anticipos` array: it stands accepted. Without
// the nota check, an anticipo whose factura final was annulled would read
// "aplicado" forever and could never be deducted by the corrected one.
//
// The anticipos report and the issue pipeline share it, so "aplicado" on screen
// and "ya fue aplicado" on emission can never disagree.
func deductsAnticipos(r string) string {
	return standsAccepted(r)
}

// sqlStringList renders codes as a SQL list literal, ('01', '02'). Only for
// catálogo constants, never for user input.
func sqlStringList(codes []string) string {
	return "('" + strings.Join(codes, "', '") + "')"
}

// anticipoRefersTo is the condition under which element `a` of a comprobante's
// `anticipos` jsonb array points at issued document `d`. sourceDocumentId is
// the robust link (historial picks). The docId match covers anticipos typed by
// hand, compared on serie + correlative so "F001-42" and "F001-00000042" are the
// same anticipo.
func anticipoRefersTo(a, d string) string {
	return fmt.Sprintf(`(%[1]s->>'sourceDocumentId' = %[2]s.id::text
		OR (split_part(%[1]s->>'docId', '-', 1) = %[2]s.series
		    AND ltrim(split_part(%[1]s->>'docId', '-', 2), '0') = %[2]s.correlative::text))`,
		a, d)
}

// AnticipoSource looks up the comprobante an applied anticipo points at — by
// sourceDocumentId when it was picked from the historial, else by its
// SERIE-CORRELATIVO — together with any OTHER comprobante that already deducts
// it. excludeDocID is the comprobante being issued, which may already list the
// anticipo from an earlier, rejected attempt.
//
// found is false when Perunio did not issue the anticipo (one emitted elsewhere
// and typed by hand): there is then nothing to check it against.
//
// Reversed and Cancelacion say whether the anticipo's money has left the sale:
// a nota de crédito undid it, or its venta was cancelled and the user recorded
// the money as returned or retained.
func (p *Pool) AnticipoSource(ctx context.Context, companyID, environment, excludeDocID string, a model.Anticipo) (src model.AnticipoSource, found bool, err error) {
	series, correlative, ok := splitDocID(a.DocID)
	if !ok && a.SourceDocumentID == nil {
		return src, false, nil
	}
	docType := sunat.Cat01Factura
	if a.DocTypeCode == "03" { // Cat.12 boleta de anticipo
		docType = sunat.Cat01Boleta
	}
	sourceID := ""
	if a.SourceDocumentID != nil {
		sourceID = *a.SourceDocumentID
	}

	err = p.WithTenant(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT d.status, d.total_amount::numeric(14,2)::text, d.currency_code, d.customer_doc_number,
			       COALESCE((
			           SELECT r.series || '-' || lpad(r.correlative::text, 8, '0')
			           FROM issued_documents r
			           CROSS JOIN LATERAL jsonb_array_elements(r.anticipos) AS a
			           WHERE r.company_id = d.company_id
			             AND r.sunat_environment = d.sunat_environment
			             AND r.id::text <> $7
			             AND `+deductsAnticipos("r")+`
			             AND `+anticipoRefersTo("a", "d")+`
			           ORDER BY r.issue_date DESC, r.created_at DESC
			           LIMIT 1
			       ), ''),
			       `+reversedByNota("d")+`,
			       COALESCE((
			           SELECT va.cancelacion ->> d.id::text
			           FROM ventas_anticipo va
			           WHERE va.id = d.venta_anticipo_id AND va.cancelada
			       ), '')
			FROM issued_documents d
			WHERE d.company_id = $1 AND d.sunat_environment = $2
			  AND (d.id::text = $3
			       OR (d.doc_type = $4 AND d.series = $5 AND d.correlative = $6))
			ORDER BY (d.id::text = $3) DESC
			LIMIT 1`,
			companyID, environment, sourceID, docType, series, correlative, excludeDocID,
		).Scan(&src.Status, &src.TotalAmount, &src.CurrencyCode, &src.CustomerDocNumber, &src.AppliedBy,
			&src.Reversed, &src.Cancelacion)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		return nil
	})
	return src, found, err
}

// splitDocID parses a SERIE-CORRELATIVO ("F001-00000042") into its serie and
// correlative number.
func splitDocID(docID string) (series string, correlative int, ok bool) {
	series, num, found := strings.Cut(strings.TrimSpace(docID), "-")
	if !found || series == "" {
		return "", 0, false
	}
	n, err := strconv.Atoi(num)
	if err != nil || n <= 0 {
		return "", 0, false
	}
	return strings.ToUpper(series), n, true
}
