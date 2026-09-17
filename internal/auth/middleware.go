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
	IsSessionAlive(ctx context.Context, sid, tenantID string) (bool, error)
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
			clearAuthCookie(w, r)
			switch err {
			case ErrTokenExpired:
				writeAuthError(w, http.StatusUnauthorized, "Token expirado", "TOKEN_EXPIRED")
			default:
				writeAuthError(w, http.StatusUnauthorized, "Token inválido", "INVALID_TOKEN")
			}
			return
		}

		// Is the session this token descends from still alive?
		//
		// This replaced a token_blacklist lookup, and it answers a strictly larger
		// question. The blacklist could only ever speak for the one jti it was
		// handed, and only logout ever wrote to it — so a password change or an
		// admin action revoked the user everywhere except here, and this service
		// kept honouring their token until it expired. The session row covers every
		// token descended from the login, which is what makes revocation terminal.
		//
		// sid is REQUIRED, unlike the jti check it replaces. The Node backend now
		// rejects any token without one (see the !payload.sid branch in
		// perunio-backend/src/auth/jwt.ts), so a token missing it cannot have come
		// from a live login and there is no transition period to tolerate.
		if payload.SID == "" {
			clearSessionCookies(w, r)
			writeAuthError(w, http.StatusUnauthorized, "Tu sesión fue cerrada", "SESSION_REVOKED")
			return
		}

		alive, err := m.users.IsSessionAlive(r.Context(), payload.SID, payload.TenantID)
		if err != nil {
			// Deliberately NOT treated as "revoked": we could not ask, which is not
			// the same as an answer. A 500 keeps the client retrying rather than
			// signing out every user in the building on a database blip.
			writeAuthError(w, http.StatusInternalServerError, "Verificación de sesión fallida", "AUTH_FAILED")
			return
		}
		if !alive {
			clearSessionCookies(w, r)
			writeAuthError(w, http.StatusUnauthorized, "Tu sesión fue cerrada", "SESSION_REVOKED")
			return
		}

		// User must still exist and be active.
		user, err := m.users.GetUserByID(r.Context(), payload.UserID)
		if err != nil {
			writeAuthError(w, http.StatusInternalServerError, "Verificación de sesión fallida", "AUTH_FAILED")
			return
		}
		if user == nil || !user.IsActive {
			clearAuthCookie(w, r)
			writeAuthError(w, http.StatusUnauthorized, "Cuenta de usuario no encontrada o inactiva", "USER_INACTIVE")
			return
		}

		// Token-version check: tokens issued before the current tokenVersion
		// (set on password change) are invalid.
		if user.TokenVersion != nil && payload.IssuedAt > 0 {
			tokenIssuedAt := time.Unix(payload.IssuedAt, 0)
			if tokenIssuedAt.Before(*user.TokenVersion) {
				clearAuthCookie(w, r)
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

// cookieDomain is the domain the Node backend scopes the auth cookies to in
// production, so app., admin. and facturador. all share one session.
//
// Hardcoded, and it is the only hardcoded hostname in this service — everything
// else comes from config. It lives here because it is not really configuration:
// it must equal what perunio-backend/src/auth/jwt.ts uses, and a value that has
// to match another repo exactly is better as a named constant pointing at its
// twin than as an env var two services could be given different answers for.
const cookieDomain = ".perunio.pe"

// expireCookie builds the deletion twin of a cookie the Node backend set.
//
// A browser only drops a cookie when the deletion matches the original's name,
// path AND domain. Node sets these from one shared object (baseCookieOptions in
// perunio-backend/src/auth/jwt.ts): in production Domain=.perunio.pe, Secure,
// SameSite=None; in dev no domain at all and SameSite=Strict.
//
// Which of the two applies is derived from the request rather than from a config
// flag, because this service has no environment variable saying which it is and
// adding one would mean another place that has to agree about cookies — the
// drift the Node side consolidated away. TLS is the honest signal: the
// production cookie is Secure, so it only ever rides on a TLS request anyway.
func expireCookie(r *http.Request, name, path string) *http.Cookie {
	c := &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     path,
		MaxAge:   -1,
		HttpOnly: true,
	}
	if isTLS(r) {
		c.Secure = true
		c.SameSite = http.SameSiteNoneMode
		c.Domain = cookieDomain
	} else {
		c.SameSite = http.SameSiteStrictMode
	}
	return c
}

// isTLS reports whether the ORIGINAL request was https. r.TLS is nil behind a
// terminating proxy (Railway), so the forwarded header is what carries it.
func isTLS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return r.Header.Get("X-Forwarded-Proto") == "https"
}

// clearAuthCookie removes the access-token cookie only.
//
// The Node backend also sets a `refresh_token` cookie (Path=/api/auth), and this
// deliberately leaves it alone. Its call sites reject the ACCESS token — expired
// or malformed — and the common one, expiry, is precisely when the browser is
// supposed to go and refresh. Clearing the refresh cookie there would turn a
// routine renewal into a forced login.
//
// When the SESSION is gone, that reasoning inverts — use clearSessionCookies.
func clearAuthCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, expireCookie(r, "auth_token", "/"))
}

// clearSessionCookies removes both cookies, for the case where the session
// itself is dead rather than merely the access token.
//
// This is the one rejection where clearing the refresh cookie is right. A
// revoked session's refresh token is dead too, so leaving it behind invites the
// browser to refresh-loop against a session that will never come back: every
// attempt spends the rate limit and every attempt fails. Dropping it sends the
// user to /login on the first try, which is where they are going regardless.
func clearSessionCookies(w http.ResponseWriter, r *http.Request) {
	clearAuthCookie(w, r)
	http.SetCookie(w, expireCookie(r, "refresh_token", "/api/auth"))
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
