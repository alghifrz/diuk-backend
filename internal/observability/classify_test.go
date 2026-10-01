package observability

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/alghifrz/diuk-backend/internal/ai"
	"github.com/alghifrz/diuk-backend/internal/channel/whatsapp"
)

func TestProviderErrorCategory(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{nil, "success"},
		{context.DeadlineExceeded, "timeout"},
		{whatsapp.ErrRateLimited, "rate_limited"},
		{ai.ErrProviderNotConfigured, "configuration"},
		{ai.ErrInvalidResponse, "invalid_response"},
		{whatsapp.ErrRejected, "provider_4xx"},
		{whatsapp.ErrUnavailable, "provider_5xx"},
		{errors.New("mystery"), "unknown"},
	}
	for _, tt := range tests {
		if got := ProviderErrorCategory(tt.err); got != tt.want {
			t.Errorf("%v = %s, want %s", tt.err, got, tt.want)
		}
	}
}

func TestHTTPStatusCategory(t *testing.T) {
	if HTTPStatusCategory(http.StatusUnauthorized) != CategoryAuthentication {
		t.Fatal("401")
	}
	if HTTPStatusCategory(http.StatusForbidden) != CategoryAuthorization {
		t.Fatal("403")
	}
	if HTTPStatusCategory(http.StatusBadRequest) != CategoryValidation {
		t.Fatal("400")
	}
}
