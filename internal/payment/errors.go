package payment

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

var (
	ErrInvalid            = errors.New("invalid request")
	ErrNotFound           = errors.New("payment not found")
	ErrAlreadyPaid        = errors.New("payment is already paid")
	ErrAlreadyRefunded    = errors.New("payment is already refunded")
	ErrInvalidState       = errors.New("payment status transition is invalid")
	ErrReservationMissing = errors.New("reservation not found")
	ErrAlreadyExists      = errors.New("reservation already has a payment")
	ErrPaymentRequired    = errors.New("reservation payment is required")
	ErrFullPaymentRequired = errors.New("full payment is required before completing")
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

func isClientError(err error) bool {
	return errors.Is(err, ErrInvalid) ||
		errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrAlreadyPaid) ||
		errors.Is(err, ErrAlreadyRefunded) ||
		errors.Is(err, ErrInvalidState) ||
		errors.Is(err, ErrReservationMissing) ||
		errors.Is(err, ErrAlreadyExists) ||
		errors.Is(err, ErrPaymentRequired) ||
		errors.Is(err, ErrFullPaymentRequired)
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, ErrNotFound):
		writeError(c, http.StatusNotFound, "PAYMENT_NOT_FOUND", "payment not found")
	case errors.Is(err, ErrReservationMissing):
		writeError(c, http.StatusNotFound, "PAYMENT_RESERVATION_NOT_FOUND", "reservation not found")
	case errors.Is(err, ErrAlreadyPaid):
		writeError(c, http.StatusConflict, "PAYMENT_ALREADY_PAID", "payment is already paid")
	case errors.Is(err, ErrAlreadyRefunded):
		writeError(c, http.StatusConflict, "PAYMENT_ALREADY_REFUNDED", "payment is already refunded")
	case errors.Is(err, ErrAlreadyExists):
		writeError(c, http.StatusConflict, "PAYMENT_ALREADY_EXISTS", "reservation already has a payment")
	case errors.Is(err, ErrInvalidState):
		writeError(c, http.StatusConflict, "PAYMENT_INVALID_STATE", "payment status transition is invalid")
	case errors.Is(err, ErrPaymentRequired):
		writeError(c, http.StatusConflict, "RESERVATION_PAYMENT_REQUIRED", "reservation payment is required")
	case errors.Is(err, ErrFullPaymentRequired):
		writeError(c, http.StatusConflict, "RESERVATION_FULL_PAYMENT_REQUIRED", "mark the remaining balance as full payment before completing")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL", "internal server error")
	}
}
