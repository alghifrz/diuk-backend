package channel

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

const channelColumns = `id, business_id, channel, status, display_name, external_account_id, external_phone_number_id, created_at, updated_at`

type Store interface {
	List(ctx context.Context, businessID uuid.UUID) ([]BusinessChannel, error)
	Get(ctx context.Context, businessID, channelID uuid.UUID) (*BusinessChannel, error)
	FindByPhoneNumberID(ctx context.Context, channel, phoneNumberID string) (*BusinessChannel, error)
	FindActive(ctx context.Context, businessID uuid.UUID, channel string) (*BusinessChannel, error)
	Create(ctx context.Context, item BusinessChannel) (*BusinessChannel, error)
	Update(ctx context.Context, businessID uuid.UUID, item BusinessChannel) (*BusinessChannel, error)
}

type Repository struct {
	db tenant.Querier
}

func NewRepository(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) List(ctx context.Context, businessID uuid.UUID) ([]BusinessChannel, error) {
	const query = `SELECT ` + channelColumns + ` FROM business_channels WHERE business_id = $1 ORDER BY channel ASC, created_at ASC`
	rows, err := r.db.Query(ctx, query, businessID)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()
	out := make([]BusinessChannel, 0)
	for rows.Next() {
		item, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, mapDBError(rows.Err())
}

func (r *Repository) Get(ctx context.Context, businessID, channelID uuid.UUID) (*BusinessChannel, error) {
	const query = `SELECT ` + channelColumns + ` FROM business_channels WHERE business_id = $1 AND id = $2`
	return scanChannel(r.db.QueryRow(ctx, query, businessID, channelID))
}

func (r *Repository) FindByPhoneNumberID(ctx context.Context, channel, phoneNumberID string) (*BusinessChannel, error) {
	const query = `SELECT ` + channelColumns + ` FROM business_channels WHERE channel = $1 AND external_phone_number_id = $2 ORDER BY CASE WHEN status = 'ACTIVE' THEN 0 ELSE 1 END, updated_at DESC LIMIT 1`
	item, err := scanChannel(r.db.QueryRow(ctx, query, channel, phoneNumberID))
	if errors.Is(err, ErrChannelNotFound) {
		return nil, nil
	}
	return item, err
}

func (r *Repository) FindActive(ctx context.Context, businessID uuid.UUID, channel string) (*BusinessChannel, error) {
	const query = `
		SELECT ` + channelColumns + `
		FROM business_channels
		WHERE business_id = $1 AND channel = $2 AND status = 'ACTIVE'
		ORDER BY updated_at DESC
		LIMIT 1`
	item, err := scanChannel(r.db.QueryRow(ctx, query, businessID, channel))
	if errors.Is(err, ErrChannelNotFound) {
		return nil, nil
	}
	return item, err
}

func (r *Repository) Create(ctx context.Context, item BusinessChannel) (*BusinessChannel, error) {
	const query = `
		INSERT INTO business_channels (business_id, channel, status, display_name, external_account_id, external_phone_number_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + channelColumns
	return scanChannel(r.db.QueryRow(ctx, query, item.BusinessID, item.Channel, item.Status, item.DisplayName, item.ExternalAccountID, item.ExternalPhoneNumberID))
}

func (r *Repository) Update(ctx context.Context, businessID uuid.UUID, item BusinessChannel) (*BusinessChannel, error) {
	const query = `
		UPDATE business_channels
		SET status = $3, display_name = $4, external_account_id = $5, external_phone_number_id = $6
		WHERE business_id = $1 AND id = $2
		RETURNING ` + channelColumns
	return scanChannel(r.db.QueryRow(ctx, query, businessID, item.ID, item.Status, item.DisplayName, item.ExternalAccountID, item.ExternalPhoneNumberID))
}

type scanner interface {
	Scan(dest ...any) error
}

func scanChannel(row scanner) (*BusinessChannel, error) {
	var item BusinessChannel
	if err := row.Scan(&item.ID, &item.BusinessID, &item.Channel, &item.Status, &item.DisplayName, &item.ExternalAccountID, &item.ExternalPhoneNumberID, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, mapDBError(err)
	}
	return &item, nil
}

func mapDBError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrChannelNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return invalid("channel configuration already exists")
		case "23514":
			return invalid("invalid value")
		}
	}
	return fmt.Errorf("database: %w", err)
}
