package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/perunio/perunio-facturador/internal/auth"
	"github.com/perunio/perunio-facturador/internal/model"
)

// CreateSeriesInput is the payload for creating a new document_series row.
type CreateSeriesInput struct {
	DocType     string
	Series      string
	Description *string
}

// UpdateSeriesInput is the payload for PUT /series/:companyId/:seriesId.
// Nil fields are left unchanged.
type UpdateSeriesInput struct {
	Description *string
	IsActive    *bool
}

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
		row := tx.QueryRow(ctx, `
			INSERT INTO document_series (tenant_id, company_id, doc_type, series, description)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING `+seriesColumns,
			tenantID, companyID, in.DocType, in.Series, in.Description)
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
// Returns ErrNotFound when the row doesn't exist.
func (p *Pool) UpdateSeries(ctx context.Context, companyID, seriesID string, in UpdateSeriesInput) (*model.Series, error) {
	var s model.Series
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE document_series
			SET description = COALESCE($3, description),
			    is_active   = COALESCE($4, is_active),
			    updated_at  = now()
			WHERE company_id = $1 AND id = $2
			RETURNING `+seriesColumns,
			companyID, seriesID, in.Description, in.IsActive)
		if err := scanSeries(row, &s); err != nil {
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
	return &s, nil
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
