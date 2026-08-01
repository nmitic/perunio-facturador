package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/perunio/perunio-facturador/internal/auth"
	"github.com/perunio/perunio-facturador/internal/model"
)

// CreateVentaAnticipoInput is the payload for creating a venta con anticipos.
type CreateVentaAnticipoInput struct {
	Nombre            string
	Descripcion       *string
	CustomerDocType   string
	CustomerDocNumber string
	CustomerName      string
	CurrencyCode      string
	MontoAcordado     string
	FormData          map[string]any
}

// UpdateVentaAnticipoInput is the payload for PUT /ventas-anticipo/:companyId/:ventaId.
// Nil fields are left unchanged.
type UpdateVentaAnticipoInput struct {
	Nombre        *string
	Descripcion   *string
	MontoAcordado *string
	FormData      map[string]any
	Cancelada     *bool
}

// VentaAnticipoFilter narrows a ventas listing. Zero values mean "no filter".
type VentaAnticipoFilter struct {
	// Estado is one of abierta/regularizada/cancelada. Applied in Go against the
	// derived value rather than in SQL — the derivation already ships in the row
	// and the result sets here are small (one per deal, not per comprobante).
	Estado      string
	CustomerDoc string
}

const ventaAnticipoColumns = `v.id, v.tenant_id, v.company_id, v.sunat_environment,
	v.nombre, v.descripcion, v.customer_doc_type, v.customer_doc_number, v.customer_name,
	v.currency_code, v.monto_acordado, v.form_data, v.cancelada, v.created_at, v.updated_at`

// ventaAnticipoDerived aggregates the comprobantes pointing at a venta.
//
// Deliberately NOT stored: cobrado, the factura final and therefore the estado
// all follow from the historial, so voiding or rejecting a comprobante corrects
// the deal on the next read instead of leaving a stale counter behind. Same
// reasoning as the derived "aplicado" status in reports.go.
//
// Only accepted comprobantes count. `operation_type = '0104'` separates the
// advances from the factura final that closes the deal.
//
// cobrado sums BOTH: an anticipo's total_amount is what was collected in
// advance, and the factura final's total_amount is the saldo (it already
// deducts the anticipos through cbc:PrepaidAmount), so together they add up to
// exactly the monto acordado — the deal reads 100% cobrado / saldo 0 once it is
// regularizada, instead of hanging at "everything but the last payment".
const ventaAnticipoDerived = `
	LEFT JOIN LATERAL (
		SELECT
			COALESCE(SUM(d.total_amount), 0)                                          AS cobrado,
			COUNT(*) FILTER (WHERE d.operation_type = '0104')::int                    AS anticipo_count,
			MAX(d.id::text)   FILTER (WHERE d.operation_type IS DISTINCT FROM '0104') AS final_id,
			MAX(d.series || '-' || lpad(d.correlative::text, 8, '0'))
				FILTER (WHERE d.operation_type IS DISTINCT FROM '0104')               AS final_code
		FROM issued_documents d
		WHERE d.venta_anticipo_id = v.id
		  AND d.status IN ('accepted', 'accepted_with_observations')
	) agg ON true`

// ventaAnticipoDerivedColumns is the derived block every venta query selects.
// The saldo is computed in SQL so Postgres' exact numeric arithmetic does the
// money subtraction rather than float64 in Go.
const ventaAnticipoDerivedColumns = `agg.cobrado,
	(v.monto_acordado - agg.cobrado) AS saldo,
	agg.anticipo_count, agg.final_id, agg.final_code`

func scanVentaAnticipo(row pgx.Row, v *model.VentaAnticipo) error {
	if err := row.Scan(
		&v.ID, &v.TenantID, &v.CompanyID, &v.SunatEnvironment,
		&v.Nombre, &v.Descripcion, &v.CustomerDocType, &v.CustomerDocNumber, &v.CustomerName,
		&v.CurrencyCode, &v.MontoAcordado, &v.FormData, &v.Cancelada, &v.CreatedAt, &v.UpdatedAt,
		&v.Cobrado, &v.Saldo, &v.AnticipoCount, &v.FinalDocumentID, &v.FinalDocumentCode,
	); err != nil {
		return err
	}
	v.Estado = deriveVentaEstado(v.Cancelada, v.FinalDocumentID)
	return nil
}

// deriveVentaEstado resolves a venta's lifecycle from the only two things that
// can decide it: the user's cancellation, and whether an accepted factura final
// exists. A venta with everything collected but no factura final is still
// abierta — the money is in, but the sale has not been invoiced.
func deriveVentaEstado(cancelada bool, finalDocumentID *string) model.VentaAnticipoEstado {
	switch {
	case cancelada:
		return model.VentaCancelada
	case finalDocumentID != nil:
		return model.VentaRegularizada
	default:
		return model.VentaAbierta
	}
}

// CreateVentaAnticipo inserts a new ventas_anticipo row, stamped with the
// company's current SUNAT environment so beta deals never mix with production.
func (p *Pool) CreateVentaAnticipo(ctx context.Context, companyID string, in CreateVentaAnticipoInput) (*model.VentaAnticipo, error) {
	tenantID, ok := auth.TenantIDFromContext(ctx)
	if !ok {
		return nil, ErrTenantContextMissing
	}
	var v model.VentaAnticipo
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			WITH ins AS (
				INSERT INTO ventas_anticipo (
					tenant_id, company_id, sunat_environment, nombre, descripcion,
					customer_doc_type, customer_doc_number, customer_name,
					currency_code, monto_acordado, form_data)
				VALUES ($1, $2,
					(SELECT sunat_environment FROM companies WHERE id = $2),
					$3, $4, $5, $6, $7, $8, $9, $10)
				RETURNING *
			)
			SELECT `+ventaAnticipoColumns+`, `+ventaAnticipoDerivedColumns+`
			FROM ins v`+ventaAnticipoDerived,
			tenantID, companyID, in.Nombre, in.Descripcion,
			in.CustomerDocType, in.CustomerDocNumber, in.CustomerName,
			in.CurrencyCode, in.MontoAcordado, in.FormData)
		return scanVentaAnticipo(row, &v)
	})
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// GetVentaAnticipo returns one venta with its derived totals.
func (p *Pool) GetVentaAnticipo(ctx context.Context, companyID, ventaID string) (*model.VentaAnticipo, error) {
	var v model.VentaAnticipo
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			"SELECT "+ventaAnticipoColumns+", "+ventaAnticipoDerivedColumns+`
			FROM ventas_anticipo v`+ventaAnticipoDerived+`
			WHERE v.company_id = $1 AND v.id = $2`,
			companyID, ventaID)
		if err := scanVentaAnticipo(row, &v); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// ListVentasAnticipo returns the company's ventas con anticipos, newest first,
// scoped to the company's current SUNAT environment like the historial.
func (p *Pool) ListVentasAnticipo(ctx context.Context, companyID string, f VentaAnticipoFilter) ([]model.VentaAnticipo, error) {
	out := []model.VentaAnticipo{}
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		args := []any{companyID}
		q := "SELECT " + ventaAnticipoColumns + ", " + ventaAnticipoDerivedColumns + `
		FROM ventas_anticipo v` + ventaAnticipoDerived + `
		WHERE ` + docScope("v")
		if f.CustomerDoc != "" {
			args = append(args, f.CustomerDoc)
			q += fmt.Sprintf(" AND v.customer_doc_number = $%d", len(args))
		}
		q += " ORDER BY v.created_at DESC"

		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v model.VentaAnticipo
			if err := scanVentaAnticipo(rows, &v); err != nil {
				return err
			}
			if f.Estado != "" && string(v.Estado) != f.Estado {
				continue
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

// UpdateVentaAnticipo applies non-nil fields from in. Returns ErrNotFound when
// the row doesn't exist.
func (p *Pool) UpdateVentaAnticipo(ctx context.Context, companyID, ventaID string, in UpdateVentaAnticipoInput) (*model.VentaAnticipo, error) {
	var v model.VentaAnticipo
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			WITH upd AS (
				UPDATE ventas_anticipo
				SET nombre         = COALESCE($3, nombre),
				    descripcion    = COALESCE($4, descripcion),
				    monto_acordado = COALESCE($5, monto_acordado),
				    form_data      = COALESCE($6, form_data),
				    cancelada      = COALESCE($7, cancelada),
				    updated_at     = now()
				WHERE company_id = $1 AND id = $2
				RETURNING *
			)
			SELECT `+ventaAnticipoColumns+`, `+ventaAnticipoDerivedColumns+`
			FROM upd v`+ventaAnticipoDerived,
			companyID, ventaID, in.Nombre, in.Descripcion, in.MontoAcordado, in.FormData, in.Cancelada)
		if err := scanVentaAnticipo(row, &v); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// DeleteVentaAnticipo removes a venta. Returns ErrVentaHasDocuments when any
// comprobante already points at it — those are real emitted documents, so the
// deal must be cancelled (cancelada) rather than erased.
func (p *Pool) DeleteVentaAnticipo(ctx context.Context, companyID, ventaID string) error {
	return p.WithTenant(ctx, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx,
			`SELECT count(*)::int FROM issued_documents WHERE venta_anticipo_id = $1`, ventaID,
		).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return ErrVentaHasDocuments
		}
		tag, err := tx.Exec(ctx,
			`DELETE FROM ventas_anticipo WHERE company_id = $1 AND id = $2`, companyID, ventaID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}
