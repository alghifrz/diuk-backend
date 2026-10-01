package business

import (
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/alghifrz/diuk-backend/internal/money"
)

func normalizeOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func validateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return invalid("name is required")
	}
	return nil
}

func validateTimezone(tz string) error {
	if strings.TrimSpace(tz) == "" {
		return invalid("timezone is required")
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return invalid("timezone is invalid")
	}
	return nil
}

func validateOptionalEmail(email *string) (*string, error) {
	email = normalizeOptional(email)
	if email == nil {
		return nil, nil
	}
	normalized := strings.ToLower(*email)
	if _, err := mail.ParseAddress(normalized); err != nil || !strings.Contains(normalized, "@") {
		return nil, invalid("email is invalid")
	}
	return &normalized, nil
}

func validateOptionalURL(raw *string, field string) (*string, error) {
	raw = normalizeOptional(raw)
	if raw == nil {
		return nil, nil
	}
	parsed, err := url.Parse(*raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, invalid(field + " is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, invalid(field + " is invalid")
	}
	return raw, nil
}

func validateCurrency(currency string) error {
	if len(currency) != 3 {
		return invalid("currency is invalid")
	}
	for _, r := range currency {
		if r < 'A' || r > 'Z' {
			return invalid("currency is invalid")
		}
	}
	return nil
}

func validateFNBSettings(s FNBSettings) error {
	if err := validateCurrency(s.Currency); err != nil {
		return err
	}
	if s.ReservationSlotDurationMinutes <= 0 {
		return invalid("reservation_slot_duration_minutes must be greater than 0")
	}
	if s.ReservationMinPartySize < 1 {
		return invalid("reservation_min_party_size must be at least 1")
	}
	if s.ReservationMaxPartySize < s.ReservationMinPartySize {
		return invalid("reservation_max_party_size must be greater than or equal to reservation_min_party_size")
	}
	if s.ReservationMinAdvanceMinutes < 0 {
		return invalid("reservation_min_advance_minutes must be at least 0")
	}
	if s.ReservationMaxAdvanceDays < 1 {
		return invalid("reservation_max_advance_days must be at least 1")
	}
	if s.DepositType != depositTypeFixed && s.DepositType != depositTypePercentage {
		return invalid("deposit_type is invalid")
	}
	if s.DepositValue.Negative() {
		return invalid("deposit_value must be at least 0")
	}
	if s.DepositType == depositTypePercentage && s.DepositValue > money.FromMajor(100) {
		return invalid("deposit_value must be at most 100 when deposit_type is PERCENTAGE")
	}
	if err := validateOptionalText(s.BankName, "bank_name", 80); err != nil {
		return err
	}
	if err := validateOptionalText(s.BankAccountNumber, "bank_account_number", 64); err != nil {
		return err
	}
	if err := validateOptionalText(s.BankAccountHolder, "bank_account_holder", 120); err != nil {
		return err
	}
	return nil
}

func validateOptionalText(value *string, field string, max int) error {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) > max {
		return invalid(field + " is too long")
	}
	return nil
}

func normalizePhone(phone *string) *string {
	return normalizeOptional(phone)
}
