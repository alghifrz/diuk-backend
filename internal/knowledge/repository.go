package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

const documentColumns = `id, business_id, title, description, source_type, source_url, storage_path, mime_type, status, version, processing_error, created_by, updated_by, created_at, updated_at`

type Repository struct {
	db tenant.Querier
}

func NewRepository(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateDocument(ctx context.Context, doc *Document) (*Document, error) {
	const query = `
		INSERT INTO knowledge_documents (
			id, business_id, title, description, source_type, source_url, storage_path, mime_type,
			status, version, processing_error, created_by, updated_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING ` + documentColumns
	if doc.ID == uuid.Nil {
		doc.ID = uuid.New()
	}
	if doc.Status == "" {
		doc.Status = StatusPending
	}
	if doc.Version < 1 {
		doc.Version = 1
	}
	return scanDocument(r.db.QueryRow(ctx, query,
		doc.ID, doc.BusinessID, doc.Title, doc.Description, doc.SourceType, doc.SourceURL, doc.StoragePath, doc.MimeType,
		doc.Status, doc.Version, doc.ProcessingError, doc.CreatedBy, doc.UpdatedBy,
	))
}

func (r *Repository) GetDocument(ctx context.Context, businessID, documentID uuid.UUID) (*Document, error) {
	const query = `SELECT ` + documentColumns + ` FROM knowledge_documents WHERE id = $1 AND business_id = $2`
	doc, err := scanDocument(r.db.QueryRow(ctx, query, documentID, businessID))
	if err != nil {
		if errors.Is(err, tenant.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDocumentNotFound
		}
		return nil, err
	}
	return doc, nil
}

func (r *Repository) ListDocuments(ctx context.Context, businessID uuid.UUID, status string) ([]Document, error) {
	query := `SELECT ` + documentColumns + ` FROM knowledge_documents WHERE business_id = $1`
	args := []any{businessID}
	if status != "" {
		query += ` AND status = $2`
		args = append(args, status)
	}
	query += ` ORDER BY updated_at DESC, created_at DESC`
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := make([]Document, 0)
	for rows.Next() {
		doc, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *doc)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateDocumentMeta(ctx context.Context, businessID, documentID uuid.UUID, title string, description *string, updatedBy *uuid.UUID) (*Document, error) {
	const query = `
		UPDATE knowledge_documents
		SET title = $3, description = $4, updated_by = $5
		WHERE id = $1 AND business_id = $2
		RETURNING ` + documentColumns
	doc, err := scanDocument(r.db.QueryRow(ctx, query, documentID, businessID, title, description, updatedBy))
	if err != nil {
		if errors.Is(err, tenant.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDocumentNotFound
		}
		return nil, err
	}
	return doc, nil
}

func (r *Repository) UpdateDocumentStorage(ctx context.Context, businessID, documentID uuid.UUID, path, mime string) (*Document, error) {
	const query = `
		UPDATE knowledge_documents
		SET storage_path = $3, mime_type = $4
		WHERE id = $1 AND business_id = $2
		RETURNING ` + documentColumns
	doc, err := scanDocument(r.db.QueryRow(ctx, query, documentID, businessID, path, mime))
	if err != nil {
		if errors.Is(err, tenant.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDocumentNotFound
		}
		return nil, err
	}
	return doc, nil
}

func (r *Repository) MarkPending(ctx context.Context, businessID, documentID uuid.UUID, updatedBy *uuid.UUID) (*Document, error) {
	const query = `
		UPDATE knowledge_documents
		SET status = 'PENDING', processing_error = NULL, version = version + 1, updated_by = $3
		WHERE id = $1 AND business_id = $2
		RETURNING ` + documentColumns
	doc, err := scanDocument(r.db.QueryRow(ctx, query, documentID, businessID, updatedBy))
	if err != nil {
		if errors.Is(err, tenant.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDocumentNotFound
		}
		return nil, err
	}
	return doc, nil
}

func (r *Repository) ReleaseProcessing(ctx context.Context, businessID, documentID uuid.UUID) error {
	const query = `
		UPDATE knowledge_documents
		SET status = 'PENDING'
		WHERE id = $1 AND business_id = $2 AND status = 'PROCESSING'`
	if _, err := r.db.Exec(ctx, query, documentID, businessID); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	return nil
}

func (r *Repository) ClaimProcessing(ctx context.Context, businessID, documentID uuid.UUID, staleAfter time.Duration) (*Document, error) {
	if staleAfter <= 0 {
		staleAfter = 15 * time.Minute
	}
	const query = `
		UPDATE knowledge_documents
		SET status = 'PROCESSING', processing_error = NULL
		WHERE id = $1
		  AND business_id = $2
		  AND (
			status = 'PENDING'
			OR (status = 'PROCESSING' AND updated_at <= now() - make_interval(secs => $3))
		  )
		RETURNING ` + documentColumns
	doc, err := scanDocument(r.db.QueryRow(ctx, query, documentID, businessID, int(staleAfter.Seconds())))
	if err != nil {
		if errors.Is(err, tenant.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDocumentAlreadyProcessing
		}
		return nil, err
	}
	return doc, nil
}

func (r *Repository) MarkReady(ctx context.Context, businessID, documentID uuid.UUID) (*Document, error) {
	const query = `
		UPDATE knowledge_documents
		SET status = 'READY', processing_error = NULL
		WHERE id = $1 AND business_id = $2
		RETURNING ` + documentColumns
	doc, err := scanDocument(r.db.QueryRow(ctx, query, documentID, businessID))
	if err != nil {
		if errors.Is(err, tenant.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDocumentNotFound
		}
		return nil, err
	}
	return doc, nil
}

func (r *Repository) MarkFailed(ctx context.Context, businessID, documentID uuid.UUID, processingError string) (*Document, error) {
	const query = `
		UPDATE knowledge_documents
		SET status = 'FAILED', processing_error = $3
		WHERE id = $1 AND business_id = $2
		RETURNING ` + documentColumns
	doc, err := scanDocument(r.db.QueryRow(ctx, query, documentID, businessID, processingError))
	if err != nil {
		if errors.Is(err, tenant.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDocumentNotFound
		}
		return nil, err
	}
	return doc, nil
}

func (r *Repository) DeleteDocument(ctx context.Context, businessID, documentID uuid.UUID) error {
	const query = `DELETE FROM knowledge_documents WHERE id = $1 AND business_id = $2`
	tag, err := r.db.Exec(ctx, query, documentID, businessID)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDocumentNotFound
	}
	return nil
}

func (r *Repository) ReplaceChunks(ctx context.Context, businessID, documentID uuid.UUID, chunks []Chunk) error {
	batch, ok := r.db.(interface {
		Begin(ctx context.Context) (pgx.Tx, error)
	})
	if !ok {
		return r.replaceChunks(ctx, r.db, businessID, documentID, chunks)
	}
	tx, err := batch.Begin(ctx)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := r.replaceChunks(ctx, tx, businessID, documentID, chunks); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) replaceChunks(ctx context.Context, db tenant.Querier, businessID, documentID uuid.UUID, chunks []Chunk) error {
	if _, err := db.Exec(ctx, `DELETE FROM knowledge_chunks WHERE document_id = $1 AND business_id = $2`, documentID, businessID); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	const insert = `
		INSERT INTO knowledge_chunks (id, business_id, document_id, chunk_index, content, embedding, token_count, metadata)
		VALUES ($1, $2, $3, $4, $5, $6::vector, $7, $8)`
	for i := range chunks {
		chunk := chunks[i]
		if chunk.ID == uuid.Nil {
			chunk.ID = uuid.New()
		}
		meta, err := json.Marshal(chunk.Metadata)
		if err != nil {
			return fmt.Errorf("database: %w", err)
		}
		if chunk.Metadata == nil {
			meta = nil
		}
		if _, err := db.Exec(ctx, insert,
			chunk.ID, businessID, documentID, chunk.ChunkIndex, chunk.Content, encodeVector(chunk.Embedding), chunk.TokenCount, meta,
		); err != nil {
			return fmt.Errorf("database: %w", err)
		}
	}
	return nil
}

func (r *Repository) SearchChunks(ctx context.Context, businessID uuid.UUID, embedding []float32, minSimilarity float64, limit int) ([]SearchHit, error) {
	if limit < 1 {
		limit = DefaultTopK
	}
	const query = `
		SELECT
			kc.id,
			kc.document_id,
			kd.title,
			kc.content,
			kc.metadata,
			1 - (kc.embedding <=> $1::vector) AS similarity
		FROM knowledge_chunks kc
		JOIN knowledge_documents kd
			ON kd.id = kc.document_id AND kd.business_id = kc.business_id
		WHERE kc.business_id = $2
		  AND kd.status = 'READY'
		  AND 1 - (kc.embedding <=> $1::vector) >= $3
		ORDER BY kc.embedding <=> $1::vector
		LIMIT $4`
	rows, err := r.db.Query(ctx, query, encodeVector(embedding), businessID, minSimilarity, limit)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	out := make([]SearchHit, 0)
	for rows.Next() {
		var hit SearchHit
		var meta []byte
		if err := rows.Scan(&hit.ChunkID, &hit.DocumentID, &hit.Title, &hit.Content, &meta, &hit.Similarity); err != nil {
			return nil, fmt.Errorf("database: %w", err)
		}
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &hit.Metadata)
		}
		out = append(out, hit)
	}
	return out, rows.Err()
}

func (r *Repository) CountChunks(ctx context.Context, businessID, documentID uuid.UUID) (int, error) {
	const query = `SELECT count(*) FROM knowledge_chunks WHERE business_id = $1 AND document_id = $2`
	var n int
	if err := r.db.QueryRow(ctx, query, businessID, documentID).Scan(&n); err != nil {
		return 0, fmt.Errorf("database: %w", err)
	}
	return n, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanDocument(row scanner) (*Document, error) {
	var doc Document
	err := row.Scan(
		&doc.ID, &doc.BusinessID, &doc.Title, &doc.Description, &doc.SourceType, &doc.SourceURL, &doc.StoragePath,
		&doc.MimeType, &doc.Status, &doc.Version, &doc.ProcessingError, &doc.CreatedBy, &doc.UpdatedBy, &doc.CreatedAt, &doc.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDocumentNotFound
		}
		return nil, fmt.Errorf("database: %w", err)
	}
	return &doc, nil
}

func parseStatusFilter(raw string) (string, error) {
	status := strings.ToUpper(strings.TrimSpace(raw))
	if status == "" {
		return "", nil
	}
	switch status {
	case StatusPending, StatusProcessing, StatusReady, StatusFailed, StatusArchived:
		return status, nil
	default:
		return "", invalid("status is invalid")
	}
}
