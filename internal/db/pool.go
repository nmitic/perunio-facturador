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

// New opens a connection pool against the given DATABASE_URL. Pings the
// database before returning so misconfiguration fails fast at startup.
func New(ctx context.Context, databaseURL string) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = 20

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
