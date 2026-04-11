package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/perunio/perunio-facturador/internal/auth"
)

// ErrTenantContextMissing means the request reached a DB helper without going
// through the JWT middleware. Always a programming error — fail loudly.
var ErrTenantContextMissing = errors.New("tenant context missing — auth middleware did not run")

// WithTenant runs fn inside a transaction with the RLS variable
// app.current_tenant_id set from the authenticated user's tenantId.
//
// Mirrors DatabaseHelpers.withTenant in perunio-backend/src/db/index.ts. Every
// SELECT/INSERT/UPDATE/DELETE against an RLS-protected table must go through
// here so PostgreSQL can enforce tenant isolation.
func (p *Pool) WithTenant(ctx context.Context, fn func(pgx.Tx) error) error {
	tenantID, ok := auth.TenantIDFromContext(ctx)
	if !ok {
		return ErrTenantContextMissing
	}
	return pgx.BeginTxFunc(ctx, p.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			"SELECT set_config('app.current_tenant_id', $1, true)", tenantID); err != nil {
			return err
		}
		return fn(tx)
	})
}

// WithExplicitTenant is the background-job equivalent of WithTenant for code
// paths that don't have an authenticated request context (none today, kept for
// parity with the Node.js helper).
func (p *Pool) WithExplicitTenant(ctx context.Context, tenantID string, fn func(pgx.Tx) error) error {
	return pgx.BeginTxFunc(ctx, p.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			"SELECT set_config('app.current_tenant_id', $1, true)", tenantID); err != nil {
			return err
		}
		return fn(tx)
	})
}
