package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/perunio/perunio-facturador/internal/auth"
	"github.com/perunio/perunio-facturador/internal/model"
)

// CreateSeriesInput is the payload for creating a new document_series row.
//
// NextCorrelative is what a company migrating from another facturador sets: it
// already issued F001-00004312 elsewhere, so its first comprobante here must be
// 4313, not 1. Nil means "brand-new serie" and leaves the column to its default.
// It seeds the production counter only — beta is a sandbox with its own
// independent sequence at SUNAT, and neither counter resets on a switch.
type CreateSeriesInput struct {
	DocType         string
	Series          string
	Description     *string
	NextCorrelative *int
}

// UpdateSeriesInput is the payload for PUT /series/:companyId/:seriesId.
// Nil fields are left unchanged.
//
// NextCorrelative is raise-only and guarded against every number this serie has
// already put on the wire — see UpdateSeries.
type UpdateSeriesInput struct {
	Description     *string
	IsActive        *bool
	NextCorrelative *int
}

// CorrelativeTooLowError reports the smallest correlative the caller may set, so
// the HTTP layer can name it instead of making the user guess. Unwraps to
// ErrCorrelativeTooLow, so errors.Is still works for callers that don't care.
type CorrelativeTooLowError struct {
	// Floor is the highest number already spoken for. A valid new value is
	// strictly greater than this.
	Floor int
}

func (e *CorrelativeTooLowError) Error() string {
	return fmt.Sprintf("correlative must be greater than %d", e.Floor)
}

func (e *CorrelativeTooLowError) Unwrap() error { return ErrCorrelativeTooLow }

func scanSeries(row pgx.Row, s *model.Series) error {
	return row.Scan(&s.ID, &s.TenantID, &s.CompanyID, &s.DocType, &s.Series,
		&s.NextCorrelative, &s.Description, &s.IsActive, &s.CreatedAt, &s.UpdatedAt)
}

const seriesColumns = `id, tenant_id, company_id, doc_type, series, next_correlative,
	description, is_active, created_at, updated_at`

// CreateSeries inserts a new document_series row. Returns ErrDuplicate when
// the (company, docType, series) unique constraint fires.
func (p *Pool) CreateSeries(ctx context.Context, companyID string, in CreateSeriesInput) (*model.Series, error) {
	tenantID, ok := auth.TenantIDFromContext(ctx)
	if !ok {
		return nil, ErrTenantContextMissing
	}
	var s model.Series
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		// Two statements rather than one with a COALESCE fallback: the "start at
		// 1" default belongs to the schema, and spelling it here as well would
		// let the two drift apart silently.
		var row pgx.Row
		if in.NextCorrelative != nil {
			row = tx.QueryRow(ctx, `
				INSERT INTO document_series (tenant_id, company_id, doc_type, series, description, next_correlative)
				VALUES ($1, $2, $3, $4, $5, $6)
				RETURNING `+seriesColumns,
				tenantID, companyID, in.DocType, in.Series, in.Description, *in.NextCorrelative)
		} else {
			row = tx.QueryRow(ctx, `
				INSERT INTO document_series (tenant_id, company_id, doc_type, series, description)
				VALUES ($1, $2, $3, $4, $5)
				RETURNING `+seriesColumns,
				tenantID, companyID, in.DocType, in.Series, in.Description)
		}
		return scanSeries(row, &s)
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return &s, nil
}

// UpdateSeries applies non-nil fields from in to the given series row.
// Returns ErrNotFound when the row doesn't exist, and *CorrelativeTooLowError
// when in.NextCorrelative would reuse a number.
//
// Moving a correlative backwards means re-emitting a número SUNAT has already
// accepted, which it rejects and which is tedious to unwind, so the counter is
// raise-only and must clear everything this serie has already put on the wire.
// The guard runs in the same transaction as the update, with the row locked, so
// a concurrent draft creation (which bumps the same counter, see
// createDocumentDraft) cannot slip between the check and the write.
func (p *Pool) UpdateSeries(ctx context.Context, companyID, seriesID string, in UpdateSeriesInput) (*model.Series, error) {
	var s model.Series
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		var current int
		if in.NextCorrelative != nil {
			if err := tx.QueryRow(ctx, `
				SELECT next_correlative FROM document_series
				WHERE company_id = $1 AND id = $2
				FOR UPDATE`,
				companyID, seriesID,
			).Scan(&current); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrNotFound
				}
				return err
			}

			// The floor is the largest number already spoken for: the counter
			// itself, plus anything actually issued under this serie. Both
			// tables are consulted because GRE despatches share
			// document_series; that table has no environment column, and only
			// production issued_documents draw from next_correlative.
			floor := current - 1
			var issued int
			if err := tx.QueryRow(ctx, `
				SELECT GREATEST(
					COALESCE((SELECT MAX(correlative) FROM issued_documents
					          WHERE series_id = $1 AND sunat_environment = 'production'), 0),
					COALESCE((SELECT MAX(correlative) FROM despatches
					          WHERE series_id = $1), 0)
				)`, seriesID).Scan(&issued); err != nil {
				return err
			}
			if issued > floor {
				floor = issued
			}
			if *in.NextCorrelative <= floor {
				return &CorrelativeTooLowError{Floor: floor}
			}
		}

		row := tx.QueryRow(ctx, `
			UPDATE document_series
			SET description       = COALESCE($3, description),
			    is_active         = COALESCE($4, is_active),
			    next_correlative  = COALESCE($5, next_correlative),
			    updated_at        = now()
			WHERE company_id = $1 AND id = $2
			RETURNING `+seriesColumns,
			companyID, seriesID, in.Description, in.IsActive, in.NextCorrelative)
		if err := scanSeries(row, &s); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		if in.NextCorrelative != nil {
			return auditCorrelativeChange(ctx, tx, seriesID, current, *in.NextCorrelative)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// auditCorrelativeChange records a correlative move in audit_logs. It is the one
// facturador setting whose misuse produces SUNAT rejections weeks later, so the
// before/after pair is worth keeping even though nothing else in this service
// writes to that table yet. Runs inside the caller's transaction, so a failed
// audit rolls the change back with it.
func auditCorrelativeChange(ctx context.Context, tx pgx.Tx, seriesID string, from, to int) error {
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return ErrTenantContextMissing
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, resource_type, resource_id, metadata)
		VALUES ($1, $2, 'series_correlative_set', 'document_series', $3, $4)`,
		user.TenantID, user.UserID, seriesID,
		map[string]any{"from": from, "to": to})
	return err
}

// DeleteSeries removes a series row. Returns ErrNotFound when missing,
// ErrSeriesHasDocuments when any issued_documents reference it.
func (p *Pool) DeleteSeries(ctx context.Context, companyID, seriesID string) error {
	return p.WithTenant(ctx, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx,
			`SELECT count(*)::int FROM issued_documents WHERE series_id = $1`, seriesID,
		).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return ErrSeriesHasDocuments
		}
		tag, err := tx.Exec(ctx,
			`DELETE FROM document_series WHERE company_id = $1 AND id = $2`,
			companyID, seriesID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// ListSeries returns every document_series row for the given company in
// (docType, series) order, scoped to the tenant from context.
func (p *Pool) ListSeries(ctx context.Context, companyID string) ([]model.Series, error) {
	var out []model.Series
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			"SELECT "+seriesColumns+
				" FROM document_series WHERE company_id = $1 ORDER BY doc_type, series",
			companyID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var s model.Series
			if err := scanSeries(rows, &s); err != nil {
				return err
			}
			out = append(out, s)
		}
		return rows.Err()
	})
	return out, err
}
