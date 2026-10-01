package business

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// Service applies profile and F&B settings rules. It always operates on the
// business id supplied by the caller, which must come from tenant context.
type Service struct {
	store Store
}

// NewService returns a service backed by store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Profile(ctx context.Context, businessID uuid.UUID) (*Profile, error) {
	return s.store.GetProfile(ctx, businessID)
}

func (s *Service) UpdateProfile(ctx context.Context, businessID uuid.UUID, in UpdateProfileInput) (*Profile, error) {
	current, err := s.store.GetProfile(ctx, businessID)
	if err != nil {
		return nil, err
	}

	next := *current
	if in.Name != nil {
		next.Name = strings.TrimSpace(*in.Name)
	}
	if in.Timezone != nil {
		next.Timezone = strings.TrimSpace(*in.Timezone)
	}
	if in.Description != nil {
		next.Description = normalizeOptional(in.Description)
	}
	if in.Phone != nil {
		next.Phone = normalizePhone(in.Phone)
	}
	if in.Email != nil {
		email, err := validateOptionalEmail(in.Email)
		if err != nil {
			return nil, err
		}
		next.Email = email
	}
	if in.Address != nil {
		next.Address = normalizeOptional(in.Address)
	}
	if in.WebsiteURL != nil {
		website, err := validateOptionalURL(in.WebsiteURL, "website_url")
		if err != nil {
			return nil, err
		}
		next.WebsiteURL = website
	}
	if in.LogoURL != nil {
		logo, err := validateOptionalURL(in.LogoURL, "logo_url")
		if err != nil {
			return nil, err
		}
		next.LogoURL = logo
	}

	if err := validateName(next.Name); err != nil {
		return nil, err
	}
	if err := validateTimezone(next.Timezone); err != nil {
		return nil, err
	}

	// Slug, type and status are identity fields and are never written here.
	next.Slug = current.Slug
	next.Type = current.Type
	next.Status = current.Status

	return s.store.UpdateProfile(ctx, businessID, next)
}

func (s *Service) FNBSettings(ctx context.Context, businessID uuid.UUID) (*FNBSettings, error) {
	return s.store.GetFNBSettings(ctx, businessID)
}

func (s *Service) UpdateFNBSettings(ctx context.Context, businessID uuid.UUID, in UpdateFNBSettingsInput) (*FNBSettings, error) {
	current, err := s.store.GetFNBSettings(ctx, businessID)
	if err != nil {
		return nil, err
	}

	next := *current
	if in.Currency != nil {
		next.Currency = strings.ToUpper(strings.TrimSpace(*in.Currency))
	}
	if in.ReservationEnabled != nil {
		next.ReservationEnabled = *in.ReservationEnabled
	}
	if in.ReservationSlotDurationMinutes != nil {
		next.ReservationSlotDurationMinutes = *in.ReservationSlotDurationMinutes
	}
	if in.ReservationMinPartySize != nil {
		next.ReservationMinPartySize = *in.ReservationMinPartySize
	}
	if in.ReservationMaxPartySize != nil {
		next.ReservationMaxPartySize = *in.ReservationMaxPartySize
	}
	if in.ReservationMinAdvanceMinutes != nil {
		next.ReservationMinAdvanceMinutes = *in.ReservationMinAdvanceMinutes
	}
	if in.ReservationMaxAdvanceDays != nil {
		next.ReservationMaxAdvanceDays = *in.ReservationMaxAdvanceDays
	}
	if in.DepositEnabled != nil {
		next.DepositEnabled = *in.DepositEnabled
	}
	if in.DepositType != nil {
		next.DepositType = strings.ToUpper(strings.TrimSpace(*in.DepositType))
	}
	if in.DepositValue != nil {
		next.DepositValue = *in.DepositValue
	}
	if in.BankName != nil {
		next.BankName = normalizeOptional(in.BankName)
	}
	if in.BankAccountNumber != nil {
		next.BankAccountNumber = normalizeOptional(in.BankAccountNumber)
	}
	if in.BankAccountHolder != nil {
		next.BankAccountHolder = normalizeOptional(in.BankAccountHolder)
	}

	if err := validateFNBSettings(next); err != nil {
		return nil, err
	}

	return s.store.UpdateFNBSettings(ctx, businessID, next)
}
