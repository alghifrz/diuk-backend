package reservation

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

var (
	ErrInvalid              = errors.New("invalid request")
	ErrNotFound             = errors.New("reservation not found")
	ErrDisabled             = errors.New("reservations are disabled")
	ErrInvalidStatus        = errors.New("reservation status transition is invalid")
	ErrOutsideHours         = errors.New("reservation is outside operating hours")
	ErrOutsideAdvance       = errors.New("reservation is outside the advance window")
	ErrPartySize            = errors.New("party size is invalid")
	ErrTableNotFound        = errors.New("table not found")
	ErrTableInactive        = errors.New("table is not active")
	ErrAreaInactive         = errors.New("area is not active")
	ErrCapacity             = errors.New("table capacity is insufficient")
	ErrTableUnavailable     = errors.New("table is unavailable")
	ErrNoTableAvailable     = errors.New("no table is available")
	ErrInvalidTimeRange     = errors.New("reservation time range is invalid")
	ErrCrossMidnight        = errors.New("reservation must stay on one local date")
	ErrCustomerNotFound     = errors.New("customer not found")
	ErrCustomerInactive     = errors.New("customer is not active")
	ErrConflict             = errors.New("reservation conflicts with another booking")
	ErrExceptionNotFound    = errors.New("schedule exception not found")
	ErrExceptionExists      = errors.New("schedule exception already exists")
	ErrCannotUpdate         = errors.New("reservation cannot be updated")
	ErrPaymentRequired      = errors.New("reservation payment is required")
	ErrFullPaymentRequired  = errors.New("full payment is required before completing")
	ErrCalendarBlocked      = errors.New("reservation overlaps a calendar block")
	ErrRescheduleNotAllowed = errors.New("reservation cannot be rescheduled")
	ErrRescheduleConflict   = errors.New("reservation cannot be rescheduled into a conflict")
	ErrMenuItemNotFound     = errors.New("menu item not found")
	ErrMenuItemInactive     = errors.New("menu item is not active")
	ErrInvalidMenuItem      = errors.New("reservation menu item is invalid")
)

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
		writeError(c, http.StatusConflict, "RESERVATION_DISABLED", "reservations are disabled")
	case errors.Is(err, ErrInvalidStatus):
		writeError(c, http.StatusConflict, "RESERVATION_INVALID_STATUS", "reservation status transition is invalid")
	case errors.Is(err, ErrOutsideHours):
		writeError(c, http.StatusUnprocessableEntity, "RESERVATION_OUTSIDE_OPERATING_HOURS", "reservation is outside operating hours")
	case errors.Is(err, ErrOutsideAdvance):
		writeError(c, http.StatusUnprocessableEntity, "RESERVATION_OUTSIDE_ADVANCE_WINDOW", "reservation is outside the advance window")
	case errors.Is(err, ErrPartySize):
		writeError(c, http.StatusUnprocessableEntity, "RESERVATION_PARTY_SIZE_INVALID", "party size is invalid")
	case errors.Is(err, ErrTableInactive):
		writeError(c, http.StatusConflict, "RESERVATION_TABLE_INACTIVE", "table is not active")
	case errors.Is(err, ErrAreaInactive):
		writeError(c, http.StatusConflict, "RESERVATION_AREA_INACTIVE", "area is not active")
	case errors.Is(err, ErrCapacity):
		writeError(c, http.StatusConflict, "RESERVATION_TABLE_CAPACITY_INSUFFICIENT", "table capacity is insufficient")
	case errors.Is(err, ErrTableUnavailable), errors.Is(err, ErrConflict):
		writeError(c, http.StatusConflict, "RESERVATION_TABLE_UNAVAILABLE", "table is unavailable")
	case errors.Is(err, ErrNoTableAvailable):
		writeError(c, http.StatusConflict, "RESERVATION_NO_TABLE_AVAILABLE", "no table is available")
	case errors.Is(err, ErrInvalidTimeRange):
		writeError(c, http.StatusBadRequest, "RESERVATION_INVALID_TIME_RANGE", "reservation time range is invalid")
	case errors.Is(err, ErrCrossMidnight):
		writeError(c, http.StatusUnprocessableEntity, "RESERVATION_CROSS_MIDNIGHT", "reservation must stay on one local date")
	case errors.Is(err, ErrCustomerInactive):
		writeError(c, http.StatusConflict, "RESERVATION_CUSTOMER_INACTIVE", "customer is not active")
	case errors.Is(err, ErrCannotUpdate):
		writeError(c, http.StatusConflict, "RESERVATION_INVALID_STATUS", "reservation cannot be updated")
	case errors.Is(err, ErrPaymentRequired):
		writeError(c, http.StatusConflict, "RESERVATION_PAYMENT_REQUIRED", "reservation payment is required")
	case errors.Is(err, ErrFullPaymentRequired):
		writeError(c, http.StatusConflict, "RESERVATION_FULL_PAYMENT_REQUIRED", "mark the remaining balance as full payment before completing")
	case errors.Is(err, ErrCalendarBlocked):
		writeError(c, http.StatusConflict, "CALENDAR_BLOCKED", "time range is blocked")
	case errors.Is(err, ErrRescheduleNotAllowed):
		writeError(c, http.StatusConflict, "RESERVATION_RESCHEDULE_NOT_ALLOWED", "reservation cannot be rescheduled")
	case errors.Is(err, ErrRescheduleConflict):
		writeError(c, http.StatusConflict, "RESERVATION_RESCHEDULE_CONFLICT", "reservation cannot be rescheduled into a conflict")
	case errors.Is(err, ErrMenuItemNotFound):
		writeError(c, http.StatusNotFound, "RESERVATION_MENU_ITEM_NOT_FOUND", "menu item not found")
	case errors.Is(err, ErrMenuItemInactive):
		writeError(c, http.StatusConflict, "RESERVATION_MENU_ITEM_INACTIVE", "menu item is not active")
	case errors.Is(err, ErrInvalidMenuItem):
		writeError(c, http.StatusBadRequest, "RESERVATION_MENU_ITEM_INVALID", err.Error())
	case errors.Is(err, ErrExceptionExists):
		writeError(c, http.StatusConflict, "SCHEDULE_EXCEPTION_EXISTS", "schedule exception already exists")
	case errors.Is(err, ErrInvalid):
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, ErrNotFound):
		writeError(c, http.StatusNotFound, "RESERVATION_NOT_FOUND", "reservation not found")
	case errors.Is(err, ErrCustomerNotFound):
		writeError(c, http.StatusNotFound, "RESERVATION_CUSTOMER_NOT_FOUND", "customer not found")
	case errors.Is(err, ErrTableNotFound):
		writeError(c, http.StatusNotFound, "RESERVATION_TABLE_NOT_FOUND", "table not found")
	case errors.Is(err, ErrExceptionNotFound):
		writeError(c, http.StatusNotFound, "SCHEDULE_EXCEPTION_NOT_FOUND", "schedule exception not found")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL", "internal server error")
	}
}

func isClientError(err error) bool {
	return errors.Is(err, ErrInvalid) ||
		errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrDisabled) ||
		errors.Is(err, ErrInvalidStatus) ||
		errors.Is(err, ErrOutsideHours) ||
		errors.Is(err, ErrOutsideAdvance) ||
		errors.Is(err, ErrPartySize) ||
		errors.Is(err, ErrTableNotFound) ||
		errors.Is(err, ErrTableInactive) ||
		errors.Is(err, ErrAreaInactive) ||
		errors.Is(err, ErrCapacity) ||
		errors.Is(err, ErrTableUnavailable) ||
		errors.Is(err, ErrNoTableAvailable) ||
		errors.Is(err, ErrInvalidTimeRange) ||
		errors.Is(err, ErrCrossMidnight) ||
		errors.Is(err, ErrCustomerNotFound) ||
		errors.Is(err, ErrCustomerInactive) ||
		errors.Is(err, ErrConflict) ||
		errors.Is(err, ErrExceptionNotFound) ||
		errors.Is(err, ErrExceptionExists) ||
		errors.Is(err, ErrCannotUpdate) ||
		errors.Is(err, ErrPaymentRequired) ||
		errors.Is(err, ErrFullPaymentRequired) ||
		errors.Is(err, ErrCalendarBlocked) ||
		errors.Is(err, ErrRescheduleNotAllowed) ||
		errors.Is(err, ErrRescheduleConflict) ||
		errors.Is(err, ErrMenuItemNotFound) ||
		errors.Is(err, ErrMenuItemInactive) ||
		errors.Is(err, ErrInvalidMenuItem)
}
