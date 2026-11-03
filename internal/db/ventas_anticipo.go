package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/nmitic/perunio-sunat-catalogs/sunat"

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
	v.currency_code, v.monto_acordado, v.form_data, v.cancelada, v.cancelacion, v.created_at, v.updated_at`

// ventaAnticipoDerived aggregates the comprobantes pointing at a venta.
//
// Deliberately NOT stored: cobrado, the factura final and therefore the estado
// all follow from the historial, so voiding or rejecting a comprobante corrects
// the deal on the next read instead of leaving a stale counter behind. Same
// reasoning as the derived "aplicado" status in reports.go.
//
// Only comprobantes in force count (standsAccepted): accepted, and not undone
// by a full-reversal nota de crédito since. An anticipo refunded with a nota 06
// stops counting as collected, and a factura final annulled with a nota 01
// stops closing the venta. `operation_type = '0104'` separates the advances
// from the factura final that closes the deal.
//
// cobrado sums BOTH: an anticipo's total_amount is what was collected in
// advance, and the factura final's total_amount is the saldo (it already
// deducts the anticipos through cbc:PrepaidAmount), so together they add up to
// exactly the monto acordado — the deal reads 100% cobrado / saldo 0 once it is
// regularizada, instead of hanging at "everything but the last payment".
//
// detraccion_declarada sums the same way and for the same reason: the SPOT
// obligation arises per payment, over the importe of the comprobante that
// documents it, so each anticipo deposits its own share and the factura final
// deposits on the saldo. Summed across the deal they come to porcentaje × monto
// acordado, which is what makes the figure worth showing.
//
// notas lists every nota de crédito referencing one of the venta's anticipos,
// in any status, for the refunds a cancellation calls for.
var ventaAnticipoDerived = fmt.Sprintf(`
	LEFT JOIN LATERAL (
		SELECT
			COALESCE(SUM(d.total_amount), 0)                                          AS cobrado,
			COALESCE(SUM(d.detraccion_monto), 0)                                      AS detraccion_declarada,
			COUNT(*) FILTER (WHERE d.operation_type = '0104')::int                    AS anticipo_count,
			MAX(d.id::text)   FILTER (WHERE d.operation_type IS DISTINCT FROM '0104') AS final_id,
			MAX(d.series || '-' || lpad(d.correlative::text, 8, '0'))
				FILTER (WHERE d.operation_type IS DISTINCT FROM '0104')               AS final_code
		FROM issued_documents d
		WHERE d.venta_anticipo_id = v.id
		  AND %[1]s
	) agg ON true
	LEFT JOIN LATERAL (
		SELECT COALESCE(json_agg(json_build_object(
			'anticipoId', a.id,
			'id',         nc.id,
			'code',       nc.series || '-' || lpad(nc.correlative::text, 8, '0'),
			'status',     nc.status,
			'reasonCode', COALESCE(nc.credit_debit_reason_code, '')
		) ORDER BY nc.created_at), '[]'::json) AS notas
		FROM issued_documents a
		JOIN issued_documents nc
		  ON nc.company_id = a.company_id
		 AND nc.sunat_environment = a.sunat_environment
		 AND nc.doc_type = '%[2]s'
		 AND nc.reference_doc_type = a.doc_type::text
		 AND nc.reference_doc_series = a.series
		 AND nc.reference_doc_correlative = a.correlative
		WHERE a.venta_anticipo_id = v.id
		  AND a.operation_type = '0104'
	) notas ON true`,
	standsAccepted("d"), sunat.Cat01NotaCredito)

// ventaAnticipoDerivedColumns is the derived block every venta query selects.
// The saldo is computed in SQL so Postgres' exact numeric arithmetic does the
// money subtraction rather than float64 in Go.
const ventaAnticipoDerivedColumns = `agg.cobrado,
	(v.monto_acordado - agg.cobrado) AS saldo,
	agg.detraccion_declarada,
	agg.anticipo_count, agg.final_id, agg.final_code, notas.notas`

func scanVentaAnticipo(row pgx.Row, v *model.VentaAnticipo) error {
	if err := row.Scan(
		&v.ID, &v.TenantID, &v.CompanyID, &v.SunatEnvironment,
		&v.Nombre, &v.Descripcion, &v.CustomerDocType, &v.CustomerDocNumber, &v.CustomerName,
		&v.CurrencyCode, &v.MontoAcordado, &v.FormData, &v.Cancelada, &v.Cancelacion, &v.CreatedAt, &v.UpdatedAt,
		&v.Cobrado, &v.Saldo, &v.DetraccionDeclarada,
		&v.AnticipoCount, &v.FinalDocumentID, &v.FinalDocumentCode, &v.NotasCredito,
	); err != nil {
		return err
	}
	v.Estado = deriveVentaEstado(v.Cancelada, v.FinalDocumentID)
	v.NotasPendientes = countNotasPendientes(v.Cancelacion, v.NotasCredito)
	return nil
}

// countNotasPendientes counts the anticipos a cancellation marked devuelto that
// no accepted full-reversal nota de crédito has undone yet: the refunds still
// to be documented at SUNAT.
func countNotasPendientes(cancelacion map[string]model.CancelacionDestino, notas []model.VentaNotaCredito) int {
	reversed := map[string]bool{}
	for _, n := range notas {
		if sunat.EstadoComprobanteEsAceptado(n.Status) && sunat.Cat09FullReversal(n.ReasonCode) {
			reversed[n.AnticipoID] = true
		}
	}
	pending := 0
	for id, destino := range cancelacion {
		if destino == model.CancelacionDevuelto && !reversed[id] {
			pending++
		}
	}
	return pending
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
				    updated_at     = now()
				WHERE company_id = $1 AND id = $2
				RETURNING *
			)
			SELECT `+ventaAnticipoColumns+`, `+ventaAnticipoDerivedColumns+`
			FROM upd v`+ventaAnticipoDerived,
			companyID, ventaID, in.Nombre, in.Descripcion, in.MontoAcordado, in.FormData)
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

// ventaAnticipoDoc is one anticipo of a venta, as cancelling it needs it.
type ventaAnticipoDoc struct {
	ID       string
	Code     string // SERIE-CORRELATIVO
	Status   string
	Reversed bool // undone by an accepted full-reversal nota de crédito
}

// AnticiposEnEsperaError refuses cancelling a venta while some of its
// anticipos may still be accepted by SUNAT: the cancellation could not say
// what became of their money.
type AnticiposEnEsperaError struct {
	Codes []string
}

func (e *AnticiposEnEsperaError) Error() string {
	return "anticipos awaiting SUNAT: " + strings.Join(e.Codes, ", ")
}

// checkCancelacion decides whether a venta with these anticipos can be
// cancelled with these destinos. An anticipo on its way to SUNAT (pending,
// sending) may still be accepted and blocks it. Every anticipo in force needs a
// destino, and destinos may name nothing else. A draft never reached SUNAT and
// needs none — the issue pipeline refuses to emit into a cancelled venta —
// and neither does a rejected, voided or already reversed one.
func checkCancelacion(anticipos []ventaAnticipoDoc, cancelacion map[string]model.CancelacionDestino) error {
	var waiting []string
	inForce := map[string]bool{}
	for _, a := range anticipos {
		switch {
		case a.Status != sunat.EstadoComprobanteDraft && !sunat.EstadoComprobanteEsFinal(a.Status):
			waiting = append(waiting, a.Code)
		case sunat.EstadoComprobanteEsAceptado(a.Status) && !a.Reversed:
			inForce[a.ID] = true
		}
	}
	if len(waiting) > 0 {
		return &AnticiposEnEsperaError{Codes: waiting}
	}
	if len(cancelacion) != len(inForce) {
		return ErrCancelacionIncompleta
	}
	for id, destino := range cancelacion {
		if !inForce[id] || !destino.Valid() {
			return ErrCancelacionIncompleta
		}
	}
	return nil
}

// CancelVentaAnticipo cancels an abierta venta, recording what became of each
// anticipo's money. Cancelling does not touch SUNAT: an anticipo marked
// devuelto still needs its nota de crédito, which the venta then tracks.
//
// Returns ErrVentaNoAbierta when the venta is already cancelled or closed by a
// factura final, *AnticiposEnEsperaError or ErrCancelacionIncompleta from
// checkCancelacion, and ErrNotFound.
func (p *Pool) CancelVentaAnticipo(ctx context.Context, companyID, ventaID string, cancelacion map[string]model.CancelacionDestino) (*model.VentaAnticipo, error) {
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		var cancelada bool
		err := tx.QueryRow(ctx,
			`SELECT cancelada FROM ventas_anticipo WHERE company_id = $1 AND id = $2 FOR UPDATE`,
			companyID, ventaID).Scan(&cancelada)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if cancelada {
			return ErrVentaNoAbierta
		}

		var closed bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM issued_documents d
				WHERE d.venta_anticipo_id = $1
				  AND d.operation_type IS DISTINCT FROM '0104'
				  AND `+standsAccepted("d")+`
			)`, ventaID).Scan(&closed); err != nil {
			return err
		}
		if closed {
			return ErrVentaNoAbierta
		}

		rows, err := tx.Query(ctx, `
			SELECT d.id::text, d.series || '-' || lpad(d.correlative::text, 8, '0'), d.status,
			       `+reversedByNota("d")+`
			FROM issued_documents d
			WHERE d.venta_anticipo_id = $1 AND d.operation_type = '0104'`, ventaID)
		if err != nil {
			return err
		}
		anticipos, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ventaAnticipoDoc, error) {
			var a ventaAnticipoDoc
			err := row.Scan(&a.ID, &a.Code, &a.Status, &a.Reversed)
			return a, err
		})
		if err != nil {
			return err
		}
		if err := checkCancelacion(anticipos, cancelacion); err != nil {
			return err
		}

		// A venta without anticipos in force stores no destinos: NULL, not {}.
		var stored any
		if len(cancelacion) > 0 {
			stored = cancelacion
		}
		_, err = tx.Exec(ctx, `
			UPDATE ventas_anticipo SET cancelada = true, cancelacion = $3, updated_at = now()
			WHERE company_id = $1 AND id = $2`, companyID, ventaID, stored)
		return err
	})
	if err != nil {
		return nil, err
	}
	return p.GetVentaAnticipo(ctx, companyID, ventaID)
}

// ReopenVentaAnticipo undoes a cancellation and forgets its destinos, as long
// as no nota de crédito has been issued against the venta's anticipos (a
// draft, rejected or voided one aside): once SUNAT has, or may have, the
// refund, the anticipo cannot be revived. Returns ErrVentaHasNotas then, and
// ErrNotFound. Reopening a venta that is not cancelled changes nothing.
func (p *Pool) ReopenVentaAnticipo(ctx context.Context, companyID, ventaID string) (*model.VentaAnticipo, error) {
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		var cancelada bool
		err := tx.QueryRow(ctx,
			`SELECT cancelada FROM ventas_anticipo WHERE company_id = $1 AND id = $2 FOR UPDATE`,
			companyID, ventaID).Scan(&cancelada)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil || !cancelada {
			return err
		}

		var hasNotas bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM issued_documents a
				JOIN issued_documents nc
				  ON nc.company_id = a.company_id
				 AND nc.sunat_environment = a.sunat_environment
				 AND nc.doc_type = $2
				 AND nc.reference_doc_type = a.doc_type::text
				 AND nc.reference_doc_series = a.series
				 AND nc.reference_doc_correlative = a.correlative
				WHERE a.venta_anticipo_id = $1
				  AND a.operation_type = '0104'
				  AND nc.status NOT IN ($3, $4, $5)
			)`, ventaID, sunat.Cat01NotaCredito,
			sunat.EstadoComprobanteDraft, sunat.EstadoComprobanteRejected, sunat.EstadoComprobanteVoided,
		).Scan(&hasNotas); err != nil {
			return err
		}
		if hasNotas {
			return ErrVentaHasNotas
		}

		_, err = tx.Exec(ctx, `
			UPDATE ventas_anticipo SET cancelada = false, cancelacion = NULL, updated_at = now()
			WHERE company_id = $1 AND id = $2`, companyID, ventaID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return p.GetVentaAnticipo(ctx, companyID, ventaID)
}
