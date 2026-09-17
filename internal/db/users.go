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

// IsSessionAlive reports whether the session a token descends from is still
// usable. Runs inside a tenant-scoped tx because user_sessions has RLS enabled.
//
// This replaces the old token_blacklist lookup, and the polarity is inverted in
// a way worth being explicit about: the blacklist was a DENY-list, where finding
// no row meant "fine, carry on". user_sessions is an ALLOW-list, where finding
// no row means the session is gone. The query therefore has to prove the session
// is alive rather than prove it is dead.
//
// That inversion is what makes the check fail closed. The tenantID handed to
// set_config comes from the JWT itself, so a forged tenantId picks its own RLS
// scope and finds nothing there — which under deny-list semantics would have
// read as a valid session. Requiring a present, unrevoked, unexpired row means
// the attacker's empty result denies the request instead of allowing it.
//
// Mirrors SessionService.isSessionAlive in
// perunio-backend/src/auth/session.service.ts, including both expiry columns:
// absolute is the cap that is never extended, idle moves forward on each refresh.
func (p *Pool) IsSessionAlive(ctx context.Context, sid, tenantID string) (bool, error) {
	var alive bool
	err := p.WithExplicitTenant(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT 1 FROM user_sessions
			 WHERE id = $1
			   AND revoked_at IS NULL
			   AND absolute_expires_at > now()
			   AND idle_expires_at > now()
			 LIMIT 1`,
			sid,
		)
		var x int
		if err := row.Scan(&x); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		alive = true
		return nil
	})
	return alive, err
}
