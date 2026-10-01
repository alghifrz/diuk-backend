package auth

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

const (
	headerAuthorization = "Authorization"
	headerBusinessID    = "X-Business-ID"
	bearerPrefix        = "Bearer "
)

// Errors returned while reading request headers.
var (
	ErrMissingToken      = errors.New("missing authorization header")
	ErrMalformedHeader   = errors.New("malformed authorization header")
	ErrMissingBusinessID = errors.New("missing business id")
	ErrInvalidBusinessID = errors.New("invalid business id")
)

// BearerToken extracts the JWT from an Authorization header. The scheme must
// be Bearer and the token must be a single non-empty value.
func BearerToken(header string) (string, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", ErrMissingToken
	}

	if len(header) < len(bearerPrefix) || !strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		return "", ErrMalformedHeader
	}

	token := strings.TrimSpace(header[len(bearerPrefix):])
	if token == "" || strings.ContainsAny(token, " \t") {
		return "", ErrMalformedHeader
	}

	return token, nil
}

// ParseBusinessID reads a business UUID from X-Business-ID.
func ParseBusinessID(header string) (uuid.UUID, error) {
	value := strings.TrimSpace(header)
	if value == "" {
		return uuid.Nil, ErrMissingBusinessID
	}

	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, ErrInvalidBusinessID
	}

	return id, nil
}
