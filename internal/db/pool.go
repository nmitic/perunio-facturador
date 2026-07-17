// Package db provides PostgreSQL access for the facturador service.
//
// Connects to the same database as perunio-backend and respects the same Row
// Level Security policies. All tenant-scoped reads/writes must go through
// WithTenant so the RLS variable app.current_tenant_id is set correctly.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool wraps a pgxpool.Pool with the tenant-aware helpers used across the service.
type Pool struct {
	pool *pgxpool.Pool
}

// New opens a connection pool against the given DATABASE_URL, sized for request
// traffic. Pings the database before returning so misconfiguration fails fast at
// startup.
func New(ctx context.Context, databaseURL string) (*Pool, error) {
	return newPool(ctx, databaseURL, 20)
}

// NewAdmin opens a deliberately small pool for the BYPASSRLS role.
//
// Its only caller is the comprobante scheduler, which runs two indexed queries a
// minute and is not latency-sensitive — sizing it like the request pool would
// reserve 20 connection slots to do almost nothing. Mirrors the `max: 5` on
// adminDb in perunio-backend/src/db/admin.ts.
func NewAdmin(ctx context.Context, databaseURL string) (*Pool, error) {
	return newPool(ctx, databaseURL, 5)
}

func newPool(ctx context.Context, databaseURL string, maxConns int32) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = maxConns

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &Pool{pool: pool}, nil
}

// Close releases all connections in the pool.
func (p *Pool) Close() {
	p.pool.Close()
}

// Raw returns the underlying pgxpool for queries that don't need a tenant tx.
func (p *Pool) Raw() *pgxpool.Pool {
	return p.pool
}
