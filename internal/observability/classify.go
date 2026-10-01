package observability

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/alghifrz/diuk-backend/internal/ai"
	"github.com/alghifrz/diuk-backend/internal/channel"
	"github.com/alghifrz/diuk-backend/internal/channel/whatsapp"
	"github.com/alghifrz/diuk-backend/internal/job"
)

const (
	CategoryValidation     = "validation"
	CategoryAuthentication = "authentication"
	CategoryAuthorization  = "authorization"
	CategoryNotFound       = "not_found"
	CategoryConflict       = "conflict"
	CategoryDependency     = "dependency"
	CategoryTimeout        = "timeout"
	CategoryDatabase       = "database"
	CategoryQueue          = "queue"
	CategoryProvider       = "provider"
	CategoryInternal       = "internal"
)

func HTTPStatusCategory(status int) string {
	switch {
	case status == http.StatusUnauthorized:
		return CategoryAuthentication
	case status == http.StatusForbidden:
		return CategoryAuthorization
	case status == http.StatusNotFound:
		return CategoryNotFound
	case status == http.StatusConflict:
		return CategoryConflict
	case status == http.StatusBadRequest:
		return CategoryValidation
	case status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout:
		return CategoryTimeout
	case status == http.StatusBadGateway || status == http.StatusServiceUnavailable:
		return CategoryDependency
	case status >= 500:
		return CategoryInternal
	default:
		return CategoryInternal
	}
}

func ProviderErrorCategory(err error) string {
	if err == nil {
		return "success"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout"
	}
	if errors.Is(err, whatsapp.ErrRateLimited) || errors.Is(err, channel.ErrRateLimited) {
		return "rate_limited"
	}
	if errors.Is(err, ai.ErrProviderNotConfigured) || errors.Is(err, ai.ErrDisabled) {
		return "configuration"
	}
	if errors.Is(err, ai.ErrInvalidResponse) || errors.Is(err, whatsapp.ErrInvalidPayload) {
		return "invalid_response"
	}
	if errors.Is(err, whatsapp.ErrRejected) || errors.Is(err, channel.ErrProviderRejected) {
		return "provider_4xx"
	}
	if errors.Is(err, whatsapp.ErrUnavailable) || errors.Is(err, channel.ErrProviderUnavailable) || errors.Is(err, ai.ErrProvider) {
		return "provider_5xx"
	}
	if errors.Is(err, job.ErrQueueUnavailable) {
		return "unknown"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "http 429"):
		return "rate_limited"
	case strings.Contains(msg, "http 4"):
		return "provider_4xx"
	case strings.Contains(msg, "http 5"):
		return "provider_5xx"
	case strings.Contains(msg, "timeout"):
		return "timeout"
	default:
		return "unknown"
	}
}
