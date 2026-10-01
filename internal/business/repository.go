package business

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

const (
	profileColumns = `id, name, slug, business_type, timezone, status, description, phone, email, address, website_url, logo_url, created_at, updated_at`
	fnbColumns = `business_id, currency, reservation_enabled, reservation_slot_duration_minutes, reservation_min_party_size, reservation_max_party_size, reservation_min_advance_minutes, reservation_max_advance_days, deposit_enabled, deposit_type, deposit_value, bank_name, bank_account_number, bank_account_holder, created_at, updated_at`
)

// Store is the persistence used by the service.
type Store interface {
	GetProfile(ctx context.Context, businessID uuid.UUID) (*Profile, error)
	UpdateProfile(ctx context.Context, businessID uuid.UUID, profile Profile) (*Profile, error)
	GetFNBSettings(ctx context.Context, businessID uuid.UUID) (*FNBSettings, error)
	UpdateFNBSettings(ctx context.Context, businessID uuid.UUID, settings FNBSettings) (*FNBSettings, error)
}

// Repository talks to PostgreSQL. It never reads a business id from HTTP.
type Repository struct {
	db tenant.Querier
}

// NewRepository returns a repository backed by db.
func NewRepository(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetProfile(ctx context.Context, businessID uuid.UUID) (*Profile, error) {
	const query = `SELECT ` + profileColumns + ` FROM businesses WHERE id = $1`
	return scanProfile(r.db.QueryRow(ctx, query, businessID))
}

func (r *Repository) UpdateProfile(ctx context.Context, businessID uuid.UUID, profile Profile) (*Profile, error) {
	const query = `
		UPDATE businesses
		SET name = $2,
		    timezone = $3,
		    description = $4,
		    phone = $5,
		    email = $6,
		    address = $7,
		    website_url = $8,
		    logo_url = $9
		WHERE id = $1
		RETURNING ` + profileColumns

	return scanProfile(r.db.QueryRow(ctx, query, businessID, profile.Name, profile.Timezone, profile.Description, profile.Phone, profile.Email, profile.Address, profile.WebsiteURL, profile.LogoURL))
}

func (r *Repository) GetFNBSettings(ctx context.Context, businessID uuid.UUID) (*FNBSettings, error) {
	const query = `SELECT ` + fnbColumns + ` FROM fnb_settings WHERE business_id = $1`
	return scanFNBSettings(r.db.QueryRow(ctx, query, businessID))
}

func (r *Repository) UpdateFNBSettings(ctx context.Context, businessID uuid.UUID, settings FNBSettings) (*FNBSettings, error) {
	const query = `
		UPDATE fnb_settings
		SET currency = $2,
		    reservation_enabled = $3,
		    reservation_slot_duration_minutes = $4,
		    reservation_min_party_size = $5,
		    reservation_max_party_size = $6,
		    reservation_min_advance_minutes = $7,
		    reservation_max_advance_days = $8,
		    deposit_enabled = $9,
		    deposit_type = $10,
		    deposit_value = $11,
		    bank_name = $12,
		    bank_account_number = $13,
		    bank_account_holder = $14
		WHERE business_id = $1
		RETURNING ` + fnbColumns

	return scanFNBSettings(r.db.QueryRow(ctx, query, businessID, settings.Currency, settings.ReservationEnabled, settings.ReservationSlotDurationMinutes, settings.ReservationMinPartySize, settings.ReservationMaxPartySize, settings.ReservationMinAdvanceMinutes, settings.ReservationMaxAdvanceDays, settings.DepositEnabled, settings.DepositType, settings.DepositValue, settings.BankName, settings.BankAccountNumber, settings.BankAccountHolder))
}

func scanProfile(row pgx.Row) (*Profile, error) {
	var p Profile
	err := row.Scan(&p.ID, &p.Name, &p.Slug, &p.Type, &p.Timezone, &p.Status, &p.Description, &p.Phone, &p.Email, &p.Address, &p.WebsiteURL, &p.LogoURL, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, mapDBError(err)
	}
	return &p, nil
}

func scanFNBSettings(row pgx.Row) (*FNBSettings, error) {
	var s FNBSettings
	err := row.Scan(&s.BusinessID, &s.Currency, &s.ReservationEnabled, &s.ReservationSlotDurationMinutes, &s.ReservationMinPartySize, &s.ReservationMaxPartySize, &s.ReservationMinAdvanceMinutes, &s.ReservationMaxAdvanceDays, &s.DepositEnabled, &s.DepositType, &s.DepositValue, &s.BankName, &s.BankAccountNumber, &s.BankAccountHolder, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, mapDBError(err)
	}
	return &s, nil
}

func mapDBError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23514" {
		return invalid("invalid value")
	}

	return fmt.Errorf("database: %w", err)
}
