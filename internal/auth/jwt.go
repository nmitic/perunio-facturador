package auth

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// ErrTokenExpired is returned when the JWT exp claim is in the past.
var ErrTokenExpired = errors.New("token expired")

// ErrInvalidToken is returned when the JWT signature or structure is invalid.
var ErrInvalidToken = errors.New("invalid token")

// ParseAndVerify decodes and validates a JWT signed with HS256 using the
// shared JWT secret. Returns the typed payload on success.
func ParseAndVerify(tokenString, secret string) (*JWTPayload, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&jwtClaims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(secret), nil
		},
		jwt.WithValidMethods([]string{"HS256"}),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}
	claims, ok := token.Claims.(*jwtClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	return &claims.JWTPayload, nil
}

// jwtClaims wraps JWTPayload so we can implement jwt.Claims via embedding.
type jwtClaims struct {
	JWTPayload
}

func (c jwtClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	if c.ExpiresAt == 0 {
		return nil, nil
	}
	return jwt.NewNumericDate(unixToTime(c.ExpiresAt)), nil
}

func (c jwtClaims) GetIssuedAt() (*jwt.NumericDate, error) {
	if c.IssuedAt == 0 {
		return nil, nil
	}
	return jwt.NewNumericDate(unixToTime(c.IssuedAt)), nil
}

func (c jwtClaims) GetNotBefore() (*jwt.NumericDate, error) { return nil, nil }
func (c jwtClaims) GetIssuer() (string, error)              { return "", nil }
func (c jwtClaims) GetSubject() (string, error)             { return c.UserID, nil }
func (c jwtClaims) GetAudience() (jwt.ClaimStrings, error)  { return nil, nil }
