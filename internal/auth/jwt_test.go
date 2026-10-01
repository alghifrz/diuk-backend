package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const testKeyID = "test-key"

func TestEndpoints(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantJWKS  string
		wantIss   string
		wantError bool
	}{
		{
			name:     "strips trailing slash and path",
			input:    "https://example.supabase.co/rest/v1/",
			wantJWKS: "https://example.supabase.co/auth/v1/.well-known/jwks.json",
			wantIss:  "https://example.supabase.co/auth/v1",
		},
		{
			name:     "plain project url",
			input:    "https://example.supabase.co",
			wantJWKS: "https://example.supabase.co/auth/v1/.well-known/jwks.json",
			wantIss:  "https://example.supabase.co/auth/v1",
		},
		{name: "empty", input: "  ", wantError: true},
		{name: "missing host", input: "https://", wantError: true},
		{name: "wrong scheme", input: "ftp://example.supabase.co", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jwksURL, issuer, err := Endpoints(tt.input)
			if tt.wantError {
				if err == nil {
					t.Fatal("Endpoints() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Endpoints() error = %v", err)
			}
			if jwksURL != tt.wantJWKS {
				t.Errorf("jwks URL = %q, want %q", jwksURL, tt.wantJWKS)
			}
			if issuer != tt.wantIss {
				t.Errorf("issuer = %q, want %q", issuer, tt.wantIss)
			}
		})
	}
}

func TestVerifyAcceptsValidToken(t *testing.T) {
	env := newJWTTestEnv(t)
	token := env.sign(t, jwt.RegisteredClaims{
		Subject:   env.subject.String(),
		Issuer:    env.issuer,
		Audience:  jwt.ClaimStrings{expectedAudience},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})

	got, err := env.verifier.Verify(token)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if got != env.subject {
		t.Errorf("Verify() = %s, want %s", got, env.subject)
	}
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	env := newJWTTestEnv(t)
	token := env.sign(t, jwt.RegisteredClaims{
		Subject:   env.subject.String(),
		Issuer:    env.issuer,
		Audience:  jwt.ClaimStrings{expectedAudience},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Hour)),
	})

	_, err := env.verifier.Verify(token)
	if err != ErrExpiredToken {
		t.Fatalf("Verify() error = %v, want ErrExpiredToken", err)
	}
}

func TestVerifyRejectsInvalidSignature(t *testing.T) {
	env := newJWTTestEnv(t)
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.RegisteredClaims{
		Subject:   env.subject.String(),
		Issuer:    env.issuer,
		Audience:  jwt.ClaimStrings{expectedAudience},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})
	token.Header["kid"] = testKeyID
	signed, err := token.SignedString(other)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_, err = env.verifier.Verify(signed)
	if err != ErrInvalidToken {
		t.Fatalf("Verify() error = %v, want ErrInvalidToken", err)
	}
}

func TestVerifyRejectsMalformedToken(t *testing.T) {
	env := newJWTTestEnv(t)
	_, err := env.verifier.Verify("not-a-jwt")
	if err != ErrMalformedToken {
		t.Fatalf("Verify() error = %v, want ErrMalformedToken", err)
	}
}

func TestVerifyRejectsMissingSubject(t *testing.T) {
	env := newJWTTestEnv(t)
	token := env.sign(t, jwt.RegisteredClaims{
		Issuer:    env.issuer,
		Audience:  jwt.ClaimStrings{expectedAudience},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})

	_, err := env.verifier.Verify(token)
	if err != ErrMissingSubject {
		t.Fatalf("Verify() error = %v, want ErrMissingSubject", err)
	}
}

func TestVerifyRejectsNonUUIDSubject(t *testing.T) {
	env := newJWTTestEnv(t)
	token := env.sign(t, jwt.RegisteredClaims{
		Subject:   "not-a-uuid",
		Issuer:    env.issuer,
		Audience:  jwt.ClaimStrings{expectedAudience},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})

	_, err := env.verifier.Verify(token)
	if err != ErrMissingSubject {
		t.Fatalf("Verify() error = %v, want ErrMissingSubject", err)
	}
}

func TestVerifyRejectsWrongIssuer(t *testing.T) {
	env := newJWTTestEnv(t)
	token := env.sign(t, jwt.RegisteredClaims{
		Subject:   env.subject.String(),
		Issuer:    "https://other.supabase.co/auth/v1",
		Audience:  jwt.ClaimStrings{expectedAudience},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})

	_, err := env.verifier.Verify(token)
	if err != ErrInvalidToken {
		t.Fatalf("Verify() error = %v, want ErrInvalidToken", err)
	}
}

func TestVerifyRejectsWrongAudience(t *testing.T) {
	env := newJWTTestEnv(t)
	token := env.sign(t, jwt.RegisteredClaims{
		Subject:   env.subject.String(),
		Issuer:    env.issuer,
		Audience:  jwt.ClaimStrings{"anon"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})

	_, err := env.verifier.Verify(token)
	if err != ErrInvalidToken {
		t.Fatalf("Verify() error = %v, want ErrInvalidToken", err)
	}
}

func TestVerifyRejectsUnexpectedAlgorithm(t *testing.T) {
	env := newJWTTestEnv(t)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   env.subject.String(),
		Issuer:    env.issuer,
		Audience:  jwt.ClaimStrings{expectedAudience},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})
	token.Header["kid"] = testKeyID
	signed, err := token.SignedString([]byte("not-a-jwks-key"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_, err = env.verifier.Verify(signed)
	if err != ErrInvalidToken {
		t.Fatalf("Verify() error = %v, want ErrInvalidToken", err)
	}
}

type jwtTestEnv struct {
	issuer   string
	subject  uuid.UUID
	priv     *ecdsa.PrivateKey
	verifier *JWKSVerifier
}

func newJWTTestEnv(t *testing.T) *jwtTestEnv {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	jwk, err := jwkset.NewJWKFromKey(priv.Public(), jwkset.JWKOptions{
		Metadata: jwkset.JWKMetadataOptions{KID: testKeyID, ALG: jwkset.AlgES256, USE: jwkset.UseSig},
	})
	if err != nil {
		t.Fatalf("create jwk: %v", err)
	}

	store := jwkset.NewMemoryStorage()
	if err := store.KeyWrite(context.Background(), jwk); err != nil {
		t.Fatalf("store jwk: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := store.JSONPublic(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	t.Cleanup(server.Close)

	issuer := "https://example.supabase.co/auth/v1"
	verifier, err := newVerifier(context.Background(), server.URL, issuer, server.Client())
	if err != nil {
		t.Fatalf("newVerifier: %v", err)
	}
	t.Cleanup(verifier.Close)

	return &jwtTestEnv{
		issuer:   issuer,
		subject:  uuid.New(),
		priv:     priv,
		verifier: verifier,
	}
}

func (env *jwtTestEnv) sign(t *testing.T, claims jwt.RegisteredClaims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = testKeyID
	signed, err := token.SignedString(env.priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}
