// Package auth verifies Supabase JWTs and resolves the request tenant context.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

// Errors returned by JWT verification. Middleware maps all of them to HTTP 401
// without exposing the underlying reason to the client.
var (
	ErrInvalidToken   = errors.New("invalid token")
	ErrExpiredToken   = errors.New("token expired")
	ErrMalformedToken = errors.New("malformed token")
	ErrMissingSubject = errors.New("missing subject")
)

const (
	jwksPath           = "/auth/v1/.well-known/jwks.json"
	issuerPath         = "/auth/v1"
	expectedAudience   = "authenticated"
	jwksHTTPTimeout    = 10 * time.Second
	jwksRefreshEvery   = time.Hour
	jwksUnknownKIDRate = 5 * time.Minute
)

// Verifier cryptographically verifies a Supabase access token and returns the
// auth user id from the `sub` claim.
type Verifier interface {
	Verify(token string) (uuid.UUID, error)
}

// JWKSVerifier validates tokens against the Supabase JWKS endpoint. Keys are
// cached and refreshed periodically, and again when an unknown kid appears so
// key rotation does not take the API down.
type JWKSVerifier struct {
	keys   keyfunc.Keyfunc
	issuer string
	cancel context.CancelFunc
}

// NewVerifier builds a JWKS-backed verifier from a Supabase project URL.
// The JWKS URL and expected issuer are derived from that URL.
func NewVerifier(parent context.Context, supabaseURL string) (*JWKSVerifier, error) {
	jwksURL, issuer, err := Endpoints(supabaseURL)
	if err != nil {
		return nil, err
	}
	return newVerifier(parent, jwksURL, issuer, nil)
}

func newVerifier(parent context.Context, jwksURL, issuer string, client *http.Client) (*JWKSVerifier, error) {
	ctx, cancel := context.WithCancel(parent)

	failFast := false
	keys, err := keyfunc.NewDefaultOverrideCtx(ctx, []string{jwksURL}, keyfunc.Override{
		Client:                    client,
		HTTPTimeout:               jwksHTTPTimeout,
		RefreshInterval:           jwksRefreshEvery,
		RefreshUnknownKID:         rate.NewLimiter(rate.Every(jwksUnknownKIDRate), 1),
		NoErrorReturnFirstHTTPReq: &failFast,
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("load supabase jwks: %w", err)
	}

	return &JWKSVerifier{keys: keys, issuer: issuer, cancel: cancel}, nil
}

// Endpoints derives the JWKS URL and JWT issuer from a Supabase project URL.
func Endpoints(supabaseURL string) (jwksURL, issuer string, err error) {
	raw := strings.TrimSpace(supabaseURL)
	if raw == "" {
		return "", "", fmt.Errorf("supabase url is required")
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", fmt.Errorf("invalid supabase url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", fmt.Errorf("invalid supabase url")
	}

	base := &url.URL{Scheme: parsed.Scheme, Host: parsed.Host}
	return base.JoinPath(jwksPath).String(), base.JoinPath(issuerPath).String(), nil
}

// Verify checks the token signature, expiration, issuer, audience and
// algorithm, then returns the UUID in the `sub` claim.
func (v *JWKSVerifier) Verify(token string) (uuid.UUID, error) {
	parsed, err := jwt.Parse(token, v.keys.Keyfunc,
		jwt.WithValidMethods([]string{jwt.SigningMethodES256.Alg(), jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(expectedAudience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if err != nil {
		return uuid.Nil, mapJWTError(err)
	}
	if !parsed.Valid {
		return uuid.Nil, ErrInvalidToken
	}

	sub, err := parsed.Claims.GetSubject()
	if err != nil || strings.TrimSpace(sub) == "" {
		return uuid.Nil, ErrMissingSubject
	}

	id, err := uuid.Parse(sub)
	if err != nil {
		return uuid.Nil, ErrMissingSubject
	}

	return id, nil
}

// Close stops the background JWKS refresh goroutine.
func (v *JWKSVerifier) Close() {
	if v.cancel != nil {
		v.cancel()
	}
}

func mapJWTError(err error) error {
	switch {
	case errors.Is(err, jwt.ErrTokenExpired):
		return ErrExpiredToken
	case errors.Is(err, jwt.ErrTokenMalformed):
		return ErrMalformedToken
	default:
		return ErrInvalidToken
	}
}
