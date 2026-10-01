package payment

import "github.com/alghifrz/diuk-backend/internal/money"

// AmountDue computes what the guest owes for the chosen payment type from the pre-order total.
func AmountDue(settings DepositSettings, paymentType string, orderTotal money.Amount) (amount money.Amount, resolvedType string, err error) {
	orderTotal = money.ClampNonNegative(orderTotal)

	if !settings.Enabled {
		return 0, PaymentTypeNone, nil
	}

	switch paymentType {
	case PaymentTypeFull:
		if orderTotal <= 0 {
			return 0, "", invalid("menu items are required for full payment")
		}
		return orderTotal, PaymentTypeFull, nil
	case PaymentTypeDeposit:
		if orderTotal <= 0 {
			return 0, "", invalid("menu items are required for deposit payment")
		}
		switch settings.Type {
		case DepositPercentage:
			due := orderTotal.PercentOf(settings.Value)
			if due <= 0 {
				return 0, "", invalid("deposit percentage produces a zero amount")
			}
			return due, PaymentTypeDeposit, nil
		default:
			due := money.ClampNonNegative(settings.Value)
			if due <= 0 {
				return 0, "", invalid("deposit amount is not configured")
			}
			if due > orderTotal {
				due = orderTotal
			}
			return due, PaymentTypeDeposit, nil
		}
	case PaymentTypeNone, "":
		return 0, "", invalid("payment_type must be FULL or DEPOSIT when deposit is required")
	default:
		return 0, "", invalid("payment_type is invalid")
	}
}
