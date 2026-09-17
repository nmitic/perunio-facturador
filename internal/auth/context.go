// Package auth handles JWT verification and the Authenticate middleware.
//
// Mirrors perunio-backend/src/auth/jwt.ts: same cookie name (auth_token), same
// HS256 algorithm, same blacklist + token-version checks. The two services
// share JWT_SECRET via awssecrets so a cookie issued by either backend is
// accepted by the other.
package auth

import "context"

type contextKey int

const (
	userKey contextKey = iota
)

// JWTPayload mirrors the JWT payload created by perunio-backend/src/auth/jwt.ts.
// JSON tags match the Node.js field names exactly.
type JWTPayload struct {
	JTI string `json:"jti"`
	// SID is the user_sessions row this token descends from. Unlike JTI it is
	// stable across a refresh, so revocation can target a session rather than a
	// single token.
	//
	// Optional, and it must stay optional: tokens minted before sessions existed
	// carry no sid and have to keep verifying until they expire. Parsed but not
	// yet acted on — the blacklist and token-version checks in middleware.go are
	// still what enforce revocation here.
	SID                       string `json:"sid,omitempty"`
	UserID                    string `json:"userId"`
	TenantID                  string `json:"tenantId"`
	Email                     string `json:"email"`
	Role                      string `json:"role"`
	SubscriptionTier          string `json:"subscriptionTier"`
	SubscriptionTierExpiresAt string `json:"subscriptionTierExpiresAt,omitempty"`
	TenantRole                string `json:"tenantRole,omitempty"`
	IssuedAt                  int64  `json:"iat,omitempty"`
	ExpiresAt                 int64  `json:"exp,omitempty"`
}

// WithUser returns a new context carrying the JWT payload.
func WithUser(ctx context.Context, p *JWTPayload) context.Context {
	return context.WithValue(ctx, userKey, p)
}

// UserFromContext extracts the JWT payload set by the Authenticate middleware.
func UserFromContext(ctx context.Context) (*JWTPayload, bool) {
	p, ok := ctx.Value(userKey).(*JWTPayload)
	return p, ok
}

// TenantIDFromContext returns the tenant id of the authenticated user.
func TenantIDFromContext(ctx context.Context) (string, bool) {
	p, ok := UserFromContext(ctx)
	if !ok {
		return "", false
	}
	return p.TenantID, true
}
