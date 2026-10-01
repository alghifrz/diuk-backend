package ai

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

var (
	ErrInvalid               = errors.New("invalid request")
	ErrDisabled              = errors.New("ai is disabled")
	ErrProviderNotConfigured = errors.New("ai provider is not configured")
	ErrProvider              = errors.New("ai provider error")
	ErrInvalidResponse       = errors.New("ai response is invalid")
	ErrToolNotFound          = errors.New("ai tool not found")
	ErrToolArguments         = errors.New("ai tool arguments are invalid")
	ErrToolExecution         = errors.New("ai tool execution failed")
	ErrToolLimit             = errors.New("ai tool limit exceeded")
	ErrContext               = errors.New("ai context error")
	ErrPromptNotFound        = errors.New("prompt not found")
	ErrSettingsNotFound      = errors.New("ai settings not found")
	ErrPromptNotDraft        = errors.New("only draft prompts can be edited")
	ErrConversationNotFound  = errors.New("conversation not found")
	ErrHumanHandling         = errors.New("conversation is handled by a human")
	ErrQuotaExceeded         = errors.New("ai quota exceeded")
)

func generationCategory(err error) string {
	if err == nil {
		return "success"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout"
	}
	if errors.Is(err, ErrProviderNotConfigured) || errors.Is(err, ErrDisabled) {
		return "configuration"
	}
	if errors.Is(err, ErrInvalidResponse) {
		return "invalid_response"
	}
	if errors.Is(err, ErrProvider) {
		return "provider_5xx"
	}
	if errors.Is(err, ErrQuotaExceeded) {
		return "quota"
	}
	return "unknown"
}

type validationError struct {
	msg string
}

func (e validationError) Error() string { return e.msg }
func (e validationError) Unwrap() error { return ErrInvalid }

func invalid(msg string) error { return validationError{msg: msg} }

func writeError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{"code": code, "message": message},
	})
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrDisabled):
		writeError(c, http.StatusConflict, "AI_DISABLED", "ai is disabled")
	case errors.Is(err, ErrProviderNotConfigured):
		writeError(c, http.StatusConflict, "AI_PROVIDER_NOT_CONFIGURED", "ai provider is not configured")
	case errors.Is(err, ErrProvider):
		writeError(c, http.StatusBadGateway, "AI_PROVIDER_ERROR", "ai provider error")
	case errors.Is(err, ErrInvalidResponse):
		writeError(c, http.StatusBadGateway, "AI_INVALID_RESPONSE", "ai response is invalid")
	case errors.Is(err, ErrToolNotFound):
		writeError(c, http.StatusBadRequest, "AI_TOOL_NOT_FOUND", "ai tool not found")
	case errors.Is(err, ErrToolArguments):
		writeError(c, http.StatusBadRequest, "AI_TOOL_INVALID_ARGUMENTS", "ai tool arguments are invalid")
	case errors.Is(err, ErrToolExecution):
		writeError(c, http.StatusConflict, "AI_TOOL_EXECUTION_FAILED", "ai tool execution failed")
	case errors.Is(err, ErrToolLimit):
		writeError(c, http.StatusConflict, "AI_TOOL_LIMIT_EXCEEDED", "ai tool limit exceeded")
	case errors.Is(err, ErrContext):
		writeError(c, http.StatusConflict, "AI_CONTEXT_ERROR", "ai context error")
	case errors.Is(err, ErrPromptNotDraft):
		writeError(c, http.StatusConflict, "PROMPT_NOT_DRAFT", "only draft prompts can be edited")
	case errors.Is(err, ErrInvalid):
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, ErrPromptNotFound):
		writeError(c, http.StatusNotFound, "AI_PROMPT_NOT_FOUND", "prompt not found")
	case errors.Is(err, ErrConversationNotFound):
		writeError(c, http.StatusNotFound, "CONVERSATION_NOT_FOUND", "conversation not found")
	case errors.Is(err, ErrHumanHandling):
		writeError(c, http.StatusConflict, "HUMAN_HANDLING_ACTIVE", "conversation is handled by a human")
	case errors.Is(err, ErrQuotaExceeded):
		writeError(c, http.StatusConflict, "AI_QUOTA_EXCEEDED", "AI is unavailable due to the account usage limit")
	case errors.Is(err, ErrSettingsNotFound):
		writeError(c, http.StatusNotFound, "AI_SETTINGS_NOT_FOUND", "ai settings not found")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL", "internal server error")
	}
}

func isClientError(err error) bool {
	return errors.Is(err, ErrInvalid) ||
		errors.Is(err, ErrDisabled) ||
		errors.Is(err, ErrProviderNotConfigured) ||
		errors.Is(err, ErrProvider) ||
		errors.Is(err, ErrInvalidResponse) ||
		errors.Is(err, ErrToolNotFound) ||
		errors.Is(err, ErrToolArguments) ||
		errors.Is(err, ErrToolExecution) ||
		errors.Is(err, ErrToolLimit) ||
		errors.Is(err, ErrContext) ||
		errors.Is(err, ErrPromptNotFound) ||
		errors.Is(err, ErrSettingsNotFound) ||
		errors.Is(err, ErrPromptNotDraft) ||
		errors.Is(err, ErrConversationNotFound) ||
		errors.Is(err, ErrHumanHandling) ||
		errors.Is(err, ErrQuotaExceeded)
}
