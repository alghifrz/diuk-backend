package knowledge

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

var (
	ErrInvalid                   = errors.New("invalid request")
	ErrDocumentNotFound          = errors.New("knowledge document not found")
	ErrDocumentAlreadyProcessing = errors.New("knowledge document is already processing")
	ErrUnsupportedDocumentType   = errors.New("unsupported document type")
	ErrDocumentEmpty             = errors.New("document has no extractable text")
	ErrDocumentTooLarge          = errors.New("document is too large")
	ErrEmbeddingFailed           = errors.New("embedding request failed")
	ErrEmbeddingNotConfigured    = errors.New("embedding provider is not configured")
	ErrDocumentProcessingFailed  = errors.New("document processing failed")
	ErrStorageFailed             = errors.New("document storage failed")
	ErrQueryTooLong              = errors.New("query is too long")
	ErrQuotaExceeded             = errors.New("knowledge document quota exceeded")
	ErrAnswerFailed              = errors.New("answer generation failed")
)

type validationError struct{ msg string }

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
	case errors.Is(err, ErrDocumentTooLarge):
		writeError(c, http.StatusBadRequest, "DOCUMENT_TOO_LARGE", "document is too large")
	case errors.Is(err, ErrUnsupportedDocumentType):
		writeError(c, http.StatusBadRequest, "UNSUPPORTED_DOCUMENT_TYPE", "unsupported document type")
	case errors.Is(err, ErrDocumentEmpty):
		writeError(c, http.StatusBadRequest, "DOCUMENT_EMPTY", "document has no extractable text")
	case errors.Is(err, ErrQueryTooLong):
		writeError(c, http.StatusBadRequest, "QUERY_TOO_LONG", "query is too long")
	case errors.Is(err, ErrInvalid):
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, ErrDocumentNotFound):
		writeError(c, http.StatusNotFound, "DOCUMENT_NOT_FOUND", "knowledge document not found")
	case errors.Is(err, ErrDocumentAlreadyProcessing):
		writeError(c, http.StatusConflict, "DOCUMENT_ALREADY_PROCESSING", "knowledge document is already processing")
	case errors.Is(err, ErrQuotaExceeded):
		writeError(c, http.StatusConflict, "KNOWLEDGE_QUOTA_EXCEEDED", "knowledge document limit reached")
	case errors.Is(err, ErrEmbeddingNotConfigured):
		writeError(c, http.StatusServiceUnavailable, "EMBEDDING_NOT_CONFIGURED", "knowledge ingestion is not configured")
	case errors.Is(err, ErrStorageFailed):
		writeError(c, http.StatusInternalServerError, "DOCUMENT_STORAGE_FAILED", "document storage failed")
	case errors.Is(err, ErrAnswerFailed):
		writeError(c, http.StatusBadGateway, "ANSWER_FAILED", "could not generate an answer")
	case errors.Is(err, ErrDocumentProcessingFailed), errors.Is(err, ErrEmbeddingFailed):
		writeError(c, http.StatusInternalServerError, "DOCUMENT_PROCESSING_FAILED", "document processing failed")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL", "internal server error")
	}
}

func isClientError(err error) bool {
	return errors.Is(err, ErrInvalid) ||
		errors.Is(err, ErrDocumentTooLarge) ||
		errors.Is(err, ErrUnsupportedDocumentType) ||
		errors.Is(err, ErrDocumentEmpty) ||
		errors.Is(err, ErrQueryTooLong) ||
		errors.Is(err, ErrDocumentNotFound) ||
		errors.Is(err, ErrDocumentAlreadyProcessing) ||
		errors.Is(err, ErrQuotaExceeded)
}

func sanitizeProcessingError(err error) string {
	if err == nil {
		return "document processing failed"
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	for _, secret := range []string{"api_key", "apikey", "access_token", "authorization", "bearer ", "jwt", "sk-"} {
		if strings.Contains(lower, secret) {
			return publicProcessingMessage(err)
		}
	}
	if len(msg) > 240 {
		msg = msg[:240]
	}
	return msg
}

func publicProcessingMessage(err error) string {
	switch {
	case errors.Is(err, ErrUnsupportedDocumentType):
		return "unsupported document type"
	case errors.Is(err, ErrDocumentEmpty):
		return "document has no extractable text"
	case errors.Is(err, ErrEmbeddingNotConfigured):
		return "embedding provider is not configured"
	case errors.Is(err, ErrEmbeddingFailed):
		return "embedding request failed"
	case errors.Is(err, ErrDocumentTooLarge):
		return "document is too large"
	default:
		return "document processing failed"
	}
}
