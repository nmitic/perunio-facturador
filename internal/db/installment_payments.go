package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/perunio/perunio-facturador/internal/auth"
	"github.com/perunio/perunio-facturador/internal/model"
)

// installmentPaymentColumns is the shared projection for a payment row. amount is
// cast to text so it round-trips as a SUNAT-precision decimal string.
const installmentPaymentColumns = `
	id, document_id, cuota_numero, amount::text, paid_at, method, reference, notes, created_at, updated_at`

func scanInstallmentPayment(row pgx.Row, p *model.InstallmentPayment) error {
	return row.Scan(&p.ID, &p.DocumentID, &p.CuotaNumero, &p.Amount, &p.PaidAt,
		&p.Method, &p.Reference, &p.Notes, &p.CreatedAt, &p.UpdatedAt)
}

// ListInstallmentPayments returns every recorded payment for a document, scoped
// to the company (and the tenant, via RLS), ordered by cuota then payment date.
func (p *Pool) ListInstallmentPayments(ctx context.Context, companyID, docID string) ([]model.InstallmentPayment, error) {
	var out []model.InstallmentPayment
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+installmentPaymentColumns+`
			FROM document_installment_payments
			WHERE document_id = $1
			  AND document_id IN (SELECT id FROM issued_documents WHERE company_id = $2)
			ORDER BY cuota_numero, paid_at, created_at`, docID, companyID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var pmt model.InstallmentPayment
			if err := scanInstallmentPayment(rows, &pmt); err != nil {
				return err
			}
			out = append(out, pmt)
		}
		return rows.Err()
	})
	return out, err
}

// CreateInstallmentPaymentInput is the payload for recording one payment.
type CreateInstallmentPaymentInput struct {
	CuotaNumero int
	Amount      string
	PaidAt      string // YYYY-MM-DD
	Method      *string
	Reference   *string
	Notes       *string
}

// ErrDocumentNotFound is returned when the target document does not exist for the
// company/tenant, so no payment could be attached to it.
var ErrDocumentNotFound = errors.New("document not found")

// CreateInstallmentPayment records a payment against a document's cuota. The
// INSERT is guarded by an EXISTS check so a payment can never be attached to a
// document outside the company/tenant.
func (p *Pool) CreateInstallmentPayment(ctx context.Context, companyID, docID string, in CreateInstallmentPaymentInput) (*model.InstallmentPayment, error) {
	tenantID, ok := auth.TenantIDFromContext(ctx)
	if !ok {
		return nil, ErrTenantContextMissing
	}

	var pmt model.InstallmentPayment
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO document_installment_payments
				(tenant_id, document_id, cuota_numero, amount, paid_at, method, reference, notes)
			SELECT $1, $2, $3, $4, $5, $6, $7, $8
			WHERE EXISTS (SELECT 1 FROM issued_documents WHERE id = $2 AND company_id = $9)
			RETURNING `+installmentPaymentColumns,
			tenantID, docID, in.CuotaNumero, in.Amount, in.PaidAt,
			in.Method, in.Reference, in.Notes, companyID)
		if err := scanInstallmentPayment(row, &pmt); err != nil {
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
	return &pmt, nil
}

// DeleteInstallmentPayment removes a recorded payment, scoped to the document and
// company. Returns ErrDocumentNotFound when nothing matched.
func (p *Pool) DeleteInstallmentPayment(ctx context.Context, companyID, docID, paymentID string) error {
	return p.WithTenant(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			DELETE FROM document_installment_payments
			WHERE id = $1 AND document_id = $2
			  AND document_id IN (SELECT id FROM issued_documents WHERE company_id = $3)`,
			paymentID, docID, companyID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrDocumentNotFound
		}
		return nil
	})
}
