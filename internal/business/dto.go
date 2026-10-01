package business

import "github.com/alghifrz/diuk-backend/internal/money"

// UpdateProfileInput is a PATCH document. A nil field is omitted. A non-nil
// empty string clears an optional field.
type UpdateProfileInput struct {
	Name        *string
	Description *string
	Phone       *string
	Email       *string
	Address     *string
	WebsiteURL  *string
	LogoURL     *string
	Timezone    *string
}

// UpdateFNBSettingsInput is a PATCH document for operational settings.
type UpdateFNBSettingsInput struct {
	Currency                       *string
	ReservationEnabled             *bool
	ReservationSlotDurationMinutes *int
	ReservationMinPartySize        *int
	ReservationMaxPartySize        *int
	ReservationMinAdvanceMinutes   *int
	ReservationMaxAdvanceDays      *int
	DepositEnabled                 *bool
	DepositType                    *string
	DepositValue                   *money.Amount
	BankName                       *string
	BankAccountNumber              *string
	BankAccountHolder              *string
}

type profileJSON struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Slug         string  `json:"slug"`
	BusinessType string  `json:"business_type"`
	Timezone     string  `json:"timezone"`
	Status       string  `json:"status"`
	Description  *string `json:"description"`
	Phone        *string `json:"phone"`
	Email        *string `json:"email"`
	Address      *string `json:"address"`
	WebsiteURL   *string `json:"website_url"`
	LogoURL      *string `json:"logo_url"`
}

type updateProfileRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Phone       *string `json:"phone"`
	Email       *string `json:"email"`
	Address     *string `json:"address"`
	WebsiteURL  *string `json:"website_url"`
	LogoURL     *string `json:"logo_url"`
	Timezone    *string `json:"timezone"`
}

type fnbSettingsJSON struct {
	BusinessID                     string       `json:"business_id"`
	Currency                       string       `json:"currency"`
	ReservationEnabled             bool         `json:"reservation_enabled"`
	ReservationSlotDurationMinutes int          `json:"reservation_slot_duration_minutes"`
	ReservationMinPartySize        int          `json:"reservation_min_party_size"`
	ReservationMaxPartySize        int          `json:"reservation_max_party_size"`
	ReservationMinAdvanceMinutes   int          `json:"reservation_min_advance_minutes"`
	ReservationMaxAdvanceDays      int          `json:"reservation_max_advance_days"`
	DepositEnabled                 bool         `json:"deposit_enabled"`
	DepositType                    string       `json:"deposit_type"`
	DepositValue                   money.Amount `json:"deposit_value"`
	BankName                       *string      `json:"bank_name"`
	BankAccountNumber              *string      `json:"bank_account_number"`
	BankAccountHolder              *string      `json:"bank_account_holder"`
}

type updateFNBSettingsRequest struct {
	Currency                       *string       `json:"currency"`
	ReservationEnabled             *bool         `json:"reservation_enabled"`
	ReservationSlotDurationMinutes *int          `json:"reservation_slot_duration_minutes"`
	ReservationMinPartySize        *int          `json:"reservation_min_party_size"`
	ReservationMaxPartySize        *int          `json:"reservation_max_party_size"`
	ReservationMinAdvanceMinutes   *int          `json:"reservation_min_advance_minutes"`
	ReservationMaxAdvanceDays      *int          `json:"reservation_max_advance_days"`
	DepositEnabled                 *bool         `json:"deposit_enabled"`
	DepositType                    *string       `json:"deposit_type"`
	DepositValue                   *money.Amount `json:"deposit_value"`
	BankName                       *string       `json:"bank_name"`
	BankAccountNumber              *string       `json:"bank_account_number"`
	BankAccountHolder              *string       `json:"bank_account_holder"`
}

func profileResponse(p *Profile) profileJSON {
	return profileJSON{
		ID:           p.ID.String(),
		Name:         p.Name,
		Slug:         p.Slug,
		BusinessType: string(p.Type),
		Timezone:     p.Timezone,
		Status:       string(p.Status),
		Description:  p.Description,
		Phone:        p.Phone,
		Email:        p.Email,
		Address:      p.Address,
		WebsiteURL:   p.WebsiteURL,
		LogoURL:      p.LogoURL,
	}
}

func fnbSettingsResponse(s *FNBSettings) fnbSettingsJSON {
	return fnbSettingsJSON{
		BusinessID:                     s.BusinessID.String(),
		Currency:                       s.Currency,
		ReservationEnabled:             s.ReservationEnabled,
		ReservationSlotDurationMinutes: s.ReservationSlotDurationMinutes,
		ReservationMinPartySize:        s.ReservationMinPartySize,
		ReservationMaxPartySize:        s.ReservationMaxPartySize,
		ReservationMinAdvanceMinutes:   s.ReservationMinAdvanceMinutes,
		ReservationMaxAdvanceDays:      s.ReservationMaxAdvanceDays,
		DepositEnabled:                 s.DepositEnabled,
		DepositType:                    s.DepositType,
		DepositValue:                   s.DepositValue,
		BankName:                       s.BankName,
		BankAccountNumber:              s.BankAccountNumber,
		BankAccountHolder:              s.BankAccountHolder,
	}
}
