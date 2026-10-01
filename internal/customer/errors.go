package customer

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

var (
	ErrInvalid            = errors.New("invalid request")
	ErrCustomerNotFound   = errors.New("customer not found")
	ErrCustomerInactive   = errors.New("customer is not active")
	ErrIdentityNotFound   = errors.New("identity not found")
	ErrIdentityExists     = errors.New("identity already exists")
	ErrInvalidChannel     = errors.New("identity channel is invalid")
	ErrTagNotFound        = errors.New("tag not found")
	ErrTagInactive        = errors.New("tag is not active")
	ErrTagNameExists      = errors.New("tag name already exists")
	ErrAssignmentExists   = errors.New("customer already has this tag")
	ErrAssignmentNotFound = errors.New("customer tag assignment not found")
)

type validationError struct {
	msg string
}

func (e validationError) Error() string { return e.msg }
func (e validationError) Unwrap() error { return ErrInvalid }

func invalid(msg string) error {
	return validationError{msg: msg}
}

func writeError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidChannel):
		writeError(c, http.StatusBadRequest, "INVALID_IDENTITY_CHANNEL", "identity channel is invalid")
	case errors.Is(err, ErrInvalid):
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, ErrCustomerNotFound):
		writeError(c, http.StatusNotFound, "CUSTOMER_NOT_FOUND", "customer not found")
	case errors.Is(err, ErrIdentityNotFound):
		writeError(c, http.StatusNotFound, "IDENTITY_NOT_FOUND", "identity not found")
	case errors.Is(err, ErrTagNotFound):
		writeError(c, http.StatusNotFound, "TAG_NOT_FOUND", "tag not found")
	case errors.Is(err, ErrAssignmentNotFound):
		writeError(c, http.StatusNotFound, "CUSTOMER_TAG_NOT_FOUND", "customer tag assignment not found")
	case errors.Is(err, ErrCustomerInactive):
		writeError(c, http.StatusConflict, "CUSTOMER_INACTIVE", "customer is not active")
	case errors.Is(err, ErrTagInactive):
		writeError(c, http.StatusConflict, "TAG_INACTIVE", "tag is not active")
	case errors.Is(err, ErrIdentityExists):
		writeError(c, http.StatusConflict, "IDENTITY_ALREADY_EXISTS", "identity already exists")
	case errors.Is(err, ErrTagNameExists):
		writeError(c, http.StatusConflict, "TAG_NAME_ALREADY_EXISTS", "tag name already exists")
	case errors.Is(err, ErrAssignmentExists):
		writeError(c, http.StatusConflict, "CUSTOMER_TAG_ALREADY_EXISTS", "customer already has this tag")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL", "internal server error")
	}
}

func isClientError(err error) bool {
	return errors.Is(err, ErrInvalid) ||
		errors.Is(err, ErrInvalidChannel) ||
		errors.Is(err, ErrCustomerNotFound) ||
		errors.Is(err, ErrCustomerInactive) ||
		errors.Is(err, ErrIdentityNotFound) ||
		errors.Is(err, ErrIdentityExists) ||
		errors.Is(err, ErrTagNotFound) ||
		errors.Is(err, ErrTagInactive) ||
		errors.Is(err, ErrTagNameExists) ||
		errors.Is(err, ErrAssignmentExists) ||
		errors.Is(err, ErrAssignmentNotFound)
}
