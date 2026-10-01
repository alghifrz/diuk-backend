package payment

import (
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/money"
)

const (
	StatusUnpaid   = "UNPAID"
	StatusDeposit  = "DEPOSIT"
	StatusPaid     = "PAID"
	StatusRefunded = "REFUNDED"

	DepositFixed      = "FIXED"
	DepositPercentage = "PERCENTAGE"

	PaymentTypeNone    = "NONE"
	PaymentTypeFull    = "FULL"
	PaymentTypeDeposit = "DEPOSIT"
)

type Payment struct {
	ID            uuid.UUID
	BusinessID    uuid.UUID
	ReservationID uuid.UUID
	Status        string
	PaymentType   string
	Amount        money.Amount
	Currency      string
	Notes         *string
	ConfirmedAt   *time.Time
	ConfirmedBy   *uuid.UUID
	RefundedAt    *time.Time
	RefundedBy    *uuid.UUID
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type DepositSettings struct {
	Enabled  bool
	Type     string
	Value    money.Amount
	Currency string
}

type ListParams struct {
	Status        string
	ReservationID *uuid.UUID
	CustomerID    *uuid.UUID
	CreatedFrom   *time.Time
	CreatedTo     *time.Time
	Limit         int
	Offset        int
}

type CreatePaymentInput struct {
	PaymentType string
	OrderTotal  money.Amount
	Notes       *string
}

// RequiredAmount is the legacy fixed-deposit helper (no menu total).
func RequiredAmount(settings DepositSettings) money.Amount {
	if !settings.Enabled {
		return 0
	}
	if settings.Type == DepositPercentage {
		return 0
	}
	return money.ClampNonNegative(settings.Value)
}

func CollectedStatusForType(paymentType string) string {
	if paymentType == PaymentTypeDeposit {
		return StatusDeposit
	}
	return StatusPaid
}
