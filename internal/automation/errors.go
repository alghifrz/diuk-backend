package automation

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

var (
	ErrInvalid            = errors.New("invalid request")
	ErrNotFound           = errors.New("automation not found")
	ErrEventNotFound      = errors.New("automation event not found")
	ErrRunNotFound        = errors.New("automation run not found")
	ErrRunNotClaimed      = errors.New("automation run was not claimed")
	ErrDisabled           = errors.New("automation engine is disabled")
	ErrRecipientMissing   = errors.New("automation recipient is missing")
	ErrSubjectMissing     = errors.New("automation subject is missing")
	ErrSubjectInvalid     = errors.New("automation subject is no longer valid")
	ErrConditionsFailed   = errors.New("automation conditions failed")
	ErrInactiveAutomation = errors.New("automation is not active")
	ErrUnsupportedAction  = errors.New("unsupported automation action")
	ErrInvalidTemplate    = errors.New("invalid automation template")
	ErrBusinessInactive   = errors.New("business is not active")
)

type validationError struct{ msg string }

func (e validationError) Error() string { return e.msg }
func (e validationError) Unwrap() error { return ErrInvalid }

func invalid(msg string) error { return validationError{msg: msg} }

type skippedError struct{ msg string }

func (e skippedError) Error() string { return e.msg }

func skipped(msg string) error { return skippedError{msg: msg} }

func writeError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{"code": code, "message": message},
	})
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, ErrNotFound):
		writeError(c, http.StatusNotFound, "AUTOMATION_NOT_FOUND", "automation not found")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL", "internal server error")
	}
}

func isClientError(err error) bool {
	return errors.Is(err, ErrInvalid) || errors.Is(err, ErrNotFound)
}

func sanitizeRunError(err error) string {
	if err == nil {
		return "automation run failed"
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	for _, secret := range []string{"api_key", "apikey", "access_token", "authorization", "bearer ", "jwt", "sk-"} {
		if strings.Contains(lower, secret) {
			return publicRunMessage(err)
		}
	}
	if len(msg) > 240 {
		msg = msg[:240]
	}
	return msg
}

func publicRunMessage(err error) string {
	switch {
	case errors.Is(err, ErrRecipientMissing):
		return "recipient is missing"
	case errors.Is(err, ErrSubjectMissing), errors.Is(err, ErrSubjectInvalid):
		return "subject is no longer valid"
	case errors.Is(err, ErrInvalidTemplate):
		return "template is invalid"
	case errors.Is(err, ErrUnsupportedAction):
		return "action is not supported"
	case errors.Is(err, ErrInactiveAutomation):
		return "automation is not active"
	case errors.Is(err, ErrBusinessInactive):
		return "business is not active"
	default:
		return "automation run failed"
	}
}

func classifyErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrRecipientMissing):
		return "RECIPIENT_MISSING"
	case errors.Is(err, ErrSubjectMissing), errors.Is(err, ErrSubjectInvalid):
		return "SUBJECT_INVALID"
	case errors.Is(err, ErrInvalidTemplate):
		return "INVALID_TEMPLATE"
	case errors.Is(err, ErrUnsupportedAction):
		return "UNSUPPORTED_ACTION"
	case errors.Is(err, ErrInactiveAutomation):
		return "AUTOMATION_INACTIVE"
	case errors.Is(err, ErrBusinessInactive):
		return "BUSINESS_INACTIVE"
	case errors.Is(err, ErrConditionsFailed):
		return "CONDITIONS_FAILED"
	case errors.Is(err, ErrInvalid):
		return "INVALID"
	default:
		return "RUN_FAILED"
	}
}
