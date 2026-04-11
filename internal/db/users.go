package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/perunio/perunio-facturador/internal/auth"
)

// GetUserByID looks up a user by primary key. Used by the JWT middleware to
// verify the user still exists and is active. Runs without an RLS context
// because the users table is global (not tenant-scoped).
func (p *Pool) GetUserByID(ctx context.Context, userID string) (*auth.UserRecord, error) {
	row := p.pool.QueryRow(ctx,
		`SELECT id, is_active, token_version FROM users WHERE id = $1 LIMIT 1`,
		userID,
	)
	var u auth.UserRecord
	if err := row.Scan(&u.ID, &u.IsActive, &u.TokenVersion); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// IsTokenBlacklisted checks whether a JWT JTI has been revoked. Runs inside a
// tenant-scoped tx because token_blacklist has RLS enabled.
func (p *Pool) IsTokenBlacklisted(ctx context.Context, jti, tenantID string) (bool, error) {
	var blacklisted bool
	err := p.WithExplicitTenant(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT 1 FROM token_blacklist WHERE jti = $1 LIMIT 1`,
			jti,
		)
		var x int
		if err := row.Scan(&x); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		blacklisted = true
		return nil
	})
	return blacklisted, err
}
