package auth

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		want    string
		wantErr error
	}{
		{name: "missing", header: "", wantErr: ErrMissingToken},
		{name: "whitespace only", header: "   ", wantErr: ErrMissingToken},
		{name: "malformed", header: "Bearer", wantErr: ErrMalformedHeader},
		{name: "non bearer scheme", header: "Basic abc", wantErr: ErrMalformedHeader},
		{name: "empty token", header: "Bearer   ", wantErr: ErrMalformedHeader},
		{name: "token with space", header: "Bearer abc def", wantErr: ErrMalformedHeader},
		{name: "valid", header: "Bearer tok-en", want: "tok-en"},
		{name: "case insensitive scheme", header: "bearer tok-en", want: "tok-en"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BearerToken(tt.header)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("BearerToken() error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("BearerToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseBusinessID(t *testing.T) {
	id := uuid.New()

	tests := []struct {
		name    string
		header  string
		want    uuid.UUID
		wantErr error
	}{
		{name: "missing", header: "", wantErr: ErrMissingBusinessID},
		{name: "whitespace", header: "  ", wantErr: ErrMissingBusinessID},
		{name: "invalid uuid", header: "not-a-uuid", wantErr: ErrInvalidBusinessID},
		{name: "valid", header: id.String(), want: id},
		{name: "valid with space", header: " " + id.String() + " ", want: id},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBusinessID(tt.header)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ParseBusinessID() error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseBusinessID() = %s, want %s", got, tt.want)
			}
		})
	}
}
