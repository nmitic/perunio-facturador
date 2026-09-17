package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// The first tests in this package, and they exist because the revocation check
// moved from a deny-list to an allow-list. That inversion is easy to get subtly
// backwards — a `!alive` written as `alive`, or a DB error folded in with a dead
// session — and either mistake compiles, passes review, and silently authorises
// sessions the user already ended. Every case below pins one direction of it.
//
// No database: the userStore interface is the seam, so a fake answers for it.
// This matches the repo's rule that no test touches a live pool (see
// internal/db/sql_shape_test.go).

const testSecret = "test-secret-not-a-real-key"

// fakeStore is a userStore whose answers each test sets directly.
type fakeStore struct {
	alive    bool
	aliveErr error
	user     *UserRecord
	userErr  error

	// What the middleware actually asked, so a test can pin that the session is
	// looked up by sid and not by something else that happens to be a uuid.
	gotSID      string
	gotTenantID string
}

func (f *fakeStore) GetUserByID(_ context.Context, _ string) (*UserRecord, error) {
	return f.user, f.userErr
}

func (f *fakeStore) IsSessionAlive(_ context.Context, sid, tenantID string) (bool, error) {
	f.gotSID = sid
	f.gotTenantID = tenantID
	return f.alive, f.aliveErr
}

// activeUser is the boring case: the user exists, is active, and has never had
// their tokens invalidated.
func activeUser() *UserRecord {
	return &UserRecord{ID: "user-1", IsActive: true}
}

// signToken mints a token the middleware will accept the signature of, so each
// test can vary exactly one claim.
func signToken(t *testing.T, p JWTPayload) string {
	t.Helper()
	if p.ExpiresAt == 0 {
		p.ExpiresAt = time.Now().Add(time.Hour).Unix()
	}
	if p.IssuedAt == 0 {
		p.IssuedAt = time.Now().Add(-time.Minute).Unix()
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims{JWTPayload: p}).
		SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("signing the test token failed: %v", err)
	}
	return signed
}

// run drives one request through the middleware and reports what came back,
// plus whether the protected handler behind it ever ran.
func run(t *testing.T, store userStore, token string) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	reached := false
	h := NewMiddleware(testSecret, store).Authenticate(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			reached = true
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/facturador/anything", nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, reached
}

// errorCode pulls the `code` out of the JSON error envelope. That code is a
// cross-repo contract: the frontend maps it to which banner the user sees.
func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response was not the JSON error envelope: %v (%q)", err, rec.Body.String())
	}
	if body.Success {
		t.Fatal("an error response must carry success:false")
	}
	return body.Code
}

// cookieByName finds a Set-Cookie the response emitted, or nil.
func cookieByName(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range (&http.Response{Header: rec.Header()}).Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// A live session is the only case that gets through, and it must look the
// session up by sid — the whole point of the change is that revocation targets
// a session rather than one token, so a lookup keyed on anything else would
// still be revoking per-token while appearing to work.
func TestAuthenticateAllowsALiveSession(t *testing.T) {
	store := &fakeStore{alive: true, user: activeUser()}
	token := signToken(t, JWTPayload{
		JTI: "jti-1", SID: "session-1", UserID: "user-1", TenantID: "tenant-1",
	})

	rec, reached := run(t, store, token)

	if !reached {
		t.Fatalf("a live session must reach the handler; got %d %s", rec.Code, rec.Body.String())
	}
	if store.gotSID != "session-1" {
		t.Fatalf("session looked up by %q, want the sid claim %q", store.gotSID, "session-1")
	}
	if store.gotTenantID != "tenant-1" {
		t.Fatalf("session scoped to tenant %q, want %q", store.gotTenantID, "tenant-1")
	}
}

// The inversion itself. Under the old blacklist a missing row meant "carry on";
// here it means the session is gone, and this is the test that fails loudly if
// anyone restores the old polarity.
func TestAuthenticateRejectsARevokedSession(t *testing.T) {
	store := &fakeStore{alive: false, user: activeUser()}
	token := signToken(t, JWTPayload{
		JTI: "jti-1", SID: "session-1", UserID: "user-1", TenantID: "tenant-1",
	})

	rec, reached := run(t, store, token)

	if reached {
		t.Fatal("a revoked session must never reach the handler")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if code := errorCode(t, rec); code != "SESSION_REVOKED" {
		t.Fatalf("code = %q, want SESSION_REVOKED — the frontend maps it to the 'revoked' banner", code)
	}
}

// Both cookies, not just the access one. A revoked session's refresh token is
// dead too, so leaving it behind lets the browser refresh-loop against a session
// that can never come back.
func TestRevokedSessionClearsBothCookies(t *testing.T) {
	store := &fakeStore{alive: false, user: activeUser()}
	token := signToken(t, JWTPayload{SID: "session-1", UserID: "user-1", TenantID: "tenant-1"})

	rec, _ := run(t, store, token)

	for _, name := range []string{"auth_token", "refresh_token"} {
		c := cookieByName(rec, name)
		if c == nil {
			t.Fatalf("%s was not cleared", name)
		}
		if c.MaxAge >= 0 {
			t.Fatalf("%s MaxAge = %d, want negative so the browser drops it", name, c.MaxAge)
		}
	}

	// Path has to match what Node set it with or the browser keeps the original.
	if c := cookieByName(rec, "refresh_token"); c.Path != "/api/auth" {
		t.Fatalf("refresh_token Path = %q, want /api/auth to match the Node backend", c.Path)
	}
}

// A routine expiry must NOT clear the refresh cookie: that is precisely when the
// browser is supposed to go and renew, and clearing it turns every token expiry
// into a forced login. This is the counterpart to the test above, and the pair
// is what keeps the two paths from being collapsed into one.
func TestExpiredTokenLeavesTheRefreshCookie(t *testing.T) {
	store := &fakeStore{alive: true, user: activeUser()}
	token := signToken(t, JWTPayload{
		SID: "session-1", UserID: "user-1", TenantID: "tenant-1",
		IssuedAt:  time.Now().Add(-2 * time.Hour).Unix(),
		ExpiresAt: time.Now().Add(-time.Hour).Unix(),
	})

	rec, reached := run(t, store, token)

	if reached {
		t.Fatal("an expired token must not reach the handler")
	}
	if code := errorCode(t, rec); code != "TOKEN_EXPIRED" {
		t.Fatalf("code = %q, want TOKEN_EXPIRED", code)
	}
	if c := cookieByName(rec, "refresh_token"); c != nil {
		t.Fatal("an expired ACCESS token must leave the refresh cookie alone — otherwise every renewal becomes a forced login")
	}
}

// sid is required. The Node backend rejects any token without one, so a token
// missing it cannot have come from a live login — and treating it as valid here
// would leave this service accepting exactly the pre-session credentials the
// whole rework exists to retire.
func TestAuthenticateRejectsATokenWithNoSID(t *testing.T) {
	store := &fakeStore{alive: true, user: activeUser()}
	token := signToken(t, JWTPayload{JTI: "jti-1", UserID: "user-1", TenantID: "tenant-1"})

	rec, reached := run(t, store, token)

	if reached {
		t.Fatal("a token with no sid must never reach the handler")
	}
	if code := errorCode(t, rec); code != "SESSION_REVOKED" {
		t.Fatalf("code = %q, want SESSION_REVOKED", code)
	}
	if store.gotSID != "" {
		t.Fatal("a token with no sid must be rejected before the session is looked up")
	}
}

// "I could not ask" is not "the session is dead". A database blip must not sign
// out every user in the building — it must surface as a retryable 500, which is
// the same call the pre-existing GetUserByID failure path makes.
func TestSessionLookupFailureIs500NotALogout(t *testing.T) {
	store := &fakeStore{aliveErr: errors.New("connection refused"), user: activeUser()}
	token := signToken(t, JWTPayload{SID: "session-1", UserID: "user-1", TenantID: "tenant-1"})

	rec, reached := run(t, store, token)

	if reached {
		t.Fatal("a failed session lookup must not reach the handler")
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 — a DB error is not a revocation", rec.Code)
	}
	if c := cookieByName(rec, "auth_token"); c != nil {
		t.Fatal("a DB error must not clear cookies: the session may well be fine")
	}
}

// The session is checked BEFORE the user lookup, so a revoked session costs one
// query rather than two. Ordering is behaviour here, not taste: the cheap
// rejection has to come first or every revoked token still pays for a user read.
func TestRevokedSessionSkipsTheUserLookup(t *testing.T) {
	store := &fakeStore{
		alive:   false,
		userErr: errors.New("GetUserByID must not be reached"),
	}
	token := signToken(t, JWTPayload{SID: "session-1", UserID: "user-1", TenantID: "tenant-1"})

	rec, _ := run(t, store, token)

	// If the user lookup ran, its error would have produced a 500 instead.
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 — the session must be checked before the user is read", rec.Code)
	}
}
