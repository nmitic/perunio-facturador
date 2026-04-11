package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// userStore is the dependency the middleware needs from the db package.
// Defined as an interface here so the auth package stays import-free of db.
type userStore interface {
	GetUserByID(ctx context.Context, userID string) (*UserRecord, error)
	IsTokenBlacklisted(ctx context.Context, jti, tenantID string) (bool, error)
}

// UserRecord is what the middleware needs from the users table.
type UserRecord struct {
	ID           string
	IsActive     bool
	TokenVersion *time.Time
}

// Middleware authenticates requests by verifying the JWT in the auth_token
// cookie (fallback Authorization: Bearer header), looking up the user, and
// stashing the payload in the request context.
//
// Mirrors the authenticate function in perunio-backend/src/auth/jwt.ts so the
// same cookie works seamlessly between Node.js and Go services.
type Middleware struct {
	secret string
	users  userStore
}

// NewMiddleware constructs the auth middleware. The secret is the shared JWT
// HMAC key from awssecrets.
func NewMiddleware(secret string, users userStore) *Middleware {
	return &Middleware{secret: secret, users: users}
}

// Authenticate is the chi-compatible middleware handler.
func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if token == "" {
			writeAuthError(w, http.StatusUnauthorized, "Autenticación requerida", "AUTH_REQUIRED")
			return
		}

		payload, err := ParseAndVerify(token, m.secret)
		if err != nil {
			clearAuthCookie(w)
			switch err {
			case ErrTokenExpired:
				writeAuthError(w, http.StatusUnauthorized, "Token expirado", "TOKEN_EXPIRED")
			default:
				writeAuthError(w, http.StatusUnauthorized, "Token inválido", "INVALID_TOKEN")
			}
			return
		}

		// Token blacklist check (only when JTI is present — older tokens lack it).
		if payload.JTI != "" {
			blacklisted, err := m.users.IsTokenBlacklisted(r.Context(), payload.JTI, payload.TenantID)
			if err != nil {
				writeAuthError(w, http.StatusInternalServerError, "Verificación de sesión fallida", "AUTH_FAILED")
				return
			}
			if blacklisted {
				clearAuthCookie(w)
				writeAuthError(w, http.StatusUnauthorized, "Token ha sido revocado", "TOKEN_REVOKED")
				return
			}
		}

		// User must still exist and be active.
		user, err := m.users.GetUserByID(r.Context(), payload.UserID)
		if err != nil {
			writeAuthError(w, http.StatusInternalServerError, "Verificación de sesión fallida", "AUTH_FAILED")
			return
		}
		if user == nil || !user.IsActive {
			clearAuthCookie(w)
			writeAuthError(w, http.StatusUnauthorized, "Cuenta de usuario no encontrada o inactiva", "USER_INACTIVE")
			return
		}

		// Token-version check: tokens issued before the current tokenVersion
		// (set on password change) are invalid.
		if user.TokenVersion != nil && payload.IssuedAt > 0 {
			tokenIssuedAt := time.Unix(payload.IssuedAt, 0)
			if tokenIssuedAt.Before(*user.TokenVersion) {
				clearAuthCookie(w)
				writeAuthError(w, http.StatusUnauthorized, "Token invalidado por cambio de contraseña", "TOKEN_INVALIDATED")
				return
			}
		}

		ctx := WithUser(r.Context(), payload)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func extractToken(r *http.Request) string {
	if c, err := r.Cookie("auth_token"); err == nil && c.Value != "" {
		return c.Value
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

func clearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}

func writeAuthError(w http.ResponseWriter, status int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": false,
		"error":   message,
		"code":    code,
	})
}
