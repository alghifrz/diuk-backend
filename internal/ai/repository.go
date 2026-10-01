package ai

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
	settingsColumns = `id, business_id, provider, model, temperature, max_output_tokens, enabled, created_at, updated_at`
	promptColumns   = `id, business_id, name, description, system_prompt, version, status, created_by, created_at, updated_at`
)

type beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type Store interface {
	RunInTx(ctx context.Context, fn func(Store) error) error
	GetSettings(ctx context.Context, businessID uuid.UUID) (*Settings, error)
	UpsertSettings(ctx context.Context, settings Settings) (*Settings, error)
	ListPrompts(ctx context.Context, businessID uuid.UUID, name, status string) ([]Prompt, error)
	GetPrompt(ctx context.Context, businessID, promptID uuid.UUID) (*Prompt, error)
	GetActivePrompt(ctx context.Context, businessID uuid.UUID, name string) (*Prompt, error)
	NextPromptVersion(ctx context.Context, businessID uuid.UUID, name string) (int, error)
	CreatePrompt(ctx context.Context, prompt Prompt) (*Prompt, error)
	UpdatePrompt(ctx context.Context, businessID uuid.UUID, prompt Prompt) (*Prompt, error)
	ArchiveActive(ctx context.Context, businessID uuid.UUID, name string) error
}

type Repository struct {
	db tenant.Querier
}

func NewRepository(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) RunInTx(ctx context.Context, fn func(Store) error) error {
	b, ok := r.db.(beginner)
	if !ok {
		return fn(r)
	}
	tx, err := b.Begin(ctx)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := fn(NewRepository(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	return nil
}

func (r *Repository) GetSettings(ctx context.Context, businessID uuid.UUID) (*Settings, error) {
	const query = `SELECT ` + settingsColumns + ` FROM ai_settings WHERE business_id = $1`
	s, err := scanSettings(r.db.QueryRow(ctx, query, businessID))
	if errors.Is(err, ErrSettingsNotFound) {
		return nil, nil
	}
	return s, err
}

func (r *Repository) UpsertSettings(ctx context.Context, settings Settings) (*Settings, error) {
	const query = `
		INSERT INTO ai_settings (business_id, provider, model, temperature, max_output_tokens, enabled)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (business_id) DO UPDATE
		SET provider = EXCLUDED.provider,
		    model = EXCLUDED.model,
		    temperature = EXCLUDED.temperature,
		    max_output_tokens = EXCLUDED.max_output_tokens,
		    enabled = EXCLUDED.enabled
		RETURNING ` + settingsColumns
	return scanSettings(r.db.QueryRow(ctx, query, settings.BusinessID, settings.Provider, settings.Model, settings.Temperature, settings.MaxOutputTokens, settings.Enabled))
}

func (r *Repository) ListPrompts(ctx context.Context, businessID uuid.UUID, name, status string) ([]Prompt, error) {
	query := `SELECT ` + promptColumns + ` FROM ai_prompts WHERE business_id = $1`
	args := []any{businessID}
	if name != "" {
		query += ` AND name = $2`
		args = append(args, name)
	}
	if status != "" {
		query += fmt.Sprintf(` AND status = $%d`, len(args)+1)
		args = append(args, status)
	}
	query += ` ORDER BY name ASC, version DESC`
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, mapDBError(err, "prompt")
	}
	defer rows.Close()
	out := make([]Prompt, 0)
	for rows.Next() {
		item, err := scanPrompt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, mapDBError(rows.Err(), "prompt")
}

func (r *Repository) GetPrompt(ctx context.Context, businessID, promptID uuid.UUID) (*Prompt, error) {
	const query = `SELECT ` + promptColumns + ` FROM ai_prompts WHERE business_id = $1 AND id = $2`
	return scanPrompt(r.db.QueryRow(ctx, query, businessID, promptID))
}

func (r *Repository) GetActivePrompt(ctx context.Context, businessID uuid.UUID, name string) (*Prompt, error) {
	const query = `SELECT ` + promptColumns + ` FROM ai_prompts WHERE business_id = $1 AND name = $2 AND status = 'ACTIVE'`
	p, err := scanPrompt(r.db.QueryRow(ctx, query, businessID, name))
	if errors.Is(err, ErrPromptNotFound) {
		return nil, nil
	}
	return p, err
}

func (r *Repository) NextPromptVersion(ctx context.Context, businessID uuid.UUID, name string) (int, error) {
	const query = `SELECT COALESCE(MAX(version), 0) + 1 FROM ai_prompts WHERE business_id = $1 AND name = $2`
	var version int
	if err := r.db.QueryRow(ctx, query, businessID, name).Scan(&version); err != nil {
		return 0, mapDBError(err, "prompt")
	}
	return version, nil
}

func (r *Repository) CreatePrompt(ctx context.Context, prompt Prompt) (*Prompt, error) {
	const query = `
		INSERT INTO ai_prompts (business_id, name, description, system_prompt, version, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING ` + promptColumns
	return scanPrompt(r.db.QueryRow(ctx, query, prompt.BusinessID, prompt.Name, prompt.Description, prompt.SystemPrompt, prompt.Version, prompt.Status, prompt.CreatedBy))
}

func (r *Repository) UpdatePrompt(ctx context.Context, businessID uuid.UUID, prompt Prompt) (*Prompt, error) {
	const query = `
		UPDATE ai_prompts
		SET name = $3, description = $4, system_prompt = $5, status = $6
		WHERE business_id = $1 AND id = $2
		RETURNING ` + promptColumns
	return scanPrompt(r.db.QueryRow(ctx, query, businessID, prompt.ID, prompt.Name, prompt.Description, prompt.SystemPrompt, prompt.Status))
}

func (r *Repository) ArchiveActive(ctx context.Context, businessID uuid.UUID, name string) error {
	_, err := r.db.Exec(ctx, `UPDATE ai_prompts SET status = 'ARCHIVED' WHERE business_id = $1 AND name = $2 AND status = 'ACTIVE'`, businessID, name)
	return mapDBError(err, "prompt")
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSettings(row scanner) (*Settings, error) {
	var s Settings
	if err := row.Scan(&s.ID, &s.BusinessID, &s.Provider, &s.Model, &s.Temperature, &s.MaxOutputTokens, &s.Enabled, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, mapDBError(err, "settings")
	}
	return &s, nil
}

func scanPrompt(row scanner) (*Prompt, error) {
	var p Prompt
	if err := row.Scan(&p.ID, &p.BusinessID, &p.Name, &p.Description, &p.SystemPrompt, &p.Version, &p.Status, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, mapDBError(err, "prompt")
	}
	return &p, nil
}

func mapDBError(err error, kind string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		if kind == "settings" {
			return ErrSettingsNotFound
		}
		return ErrPromptNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return invalid("prompt version already exists")
		case "23514":
			return invalid("invalid value")
		}
	}
	return fmt.Errorf("database: %w", err)
}
