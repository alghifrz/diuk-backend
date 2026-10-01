package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

const (
	conversationSelect = `
		conv.id, conv.business_id, conv.customer_id, conv.channel, conv.customer_identity_id,
		conv.status, conv.assigned_user_id, conv.last_message_at, conv.unread_count,
		conv.handling_state, conv.handover_reason, conv.handover_requested_at, conv.human_started_at, conv.resolved_at,
		conv.created_at, conv.updated_at,
		c.id, c.name, c.phone, c.email,
		i.id, i.channel, i.external_id,
		u.id, u.full_name`
	messageColumns = `id, business_id, conversation_id, direction, sender_type, message_type, content, external_id, status, metadata, created_at, updated_at`
)

type beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Store is the persistence used by the service.
type Store interface {
	RunInTx(ctx context.Context, fn func(Store) error) error
	ListConversations(ctx context.Context, businessID uuid.UUID, params ListParams) ([]Conversation, error)
	GetConversation(ctx context.Context, businessID, conversationID uuid.UUID) (*Conversation, error)
	LockConversation(ctx context.Context, businessID, conversationID uuid.UUID) (*Conversation, error)
	FindOpenConversation(ctx context.Context, businessID, customerID uuid.UUID, channel Channel, identityID *uuid.UUID) (*Conversation, error)
	FindLatestConversation(ctx context.Context, businessID, customerID uuid.UUID, channel Channel, identityID *uuid.UUID) (*Conversation, error)
	CreateConversation(ctx context.Context, conv Conversation) (*Conversation, error)
	UpdateConversation(ctx context.Context, businessID uuid.UUID, conv Conversation) (*Conversation, error)
	MarkRead(ctx context.Context, businessID, conversationID uuid.UUID) (*Conversation, error)
	MarkInboundRead(ctx context.Context, businessID, conversationID uuid.UUID) ([]string, error)
	ApplyMessageSideEffects(ctx context.Context, businessID, conversationID uuid.UUID, at time.Time, unreadDelta int, reopen bool) error

	GetCustomer(ctx context.Context, businessID, customerID uuid.UUID) (*CustomerRecord, error)
	GetIdentity(ctx context.Context, businessID, identityID uuid.UUID) (*IdentityRecord, error)

	CreateMessage(ctx context.Context, msg Message) (*Message, error)
	ListMessages(ctx context.Context, businessID, conversationID uuid.UUID, params MessageListParams) ([]Message, error)
	ListRecentMessages(ctx context.Context, businessID, conversationID uuid.UUID, limit int) ([]Message, error)
	GetMessageByExternalID(ctx context.Context, businessID, conversationID uuid.UUID, externalID string) (*Message, error)
	FindMessageByExternalID(ctx context.Context, businessID uuid.UUID, externalID string) (*Message, error)
	GetMessage(ctx context.Context, businessID, messageID uuid.UUID) (*Message, error)
	UpdateMessageDelivery(ctx context.Context, businessID, messageID uuid.UUID, status MessageStatus, externalID *string) (*Message, error)
	ClaimForDelivery(ctx context.Context, businessID, messageID uuid.UUID, staleAfter time.Duration) (*Message, error)
	ListHumanInbox(ctx context.Context, businessID uuid.UUID, states []HandlingState, limit, offset int) ([]HumanInboxItem, error)
}

// Repository talks to PostgreSQL.
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

func (r *Repository) ListConversations(ctx context.Context, businessID uuid.UUID, params ListParams) ([]Conversation, error) {
	query := `
		SELECT ` + conversationSelect + `,
		       last.content, last.created_at, last.sender_type, last.direction, last.status
		FROM conversations conv
		JOIN customers c ON c.id = conv.customer_id AND c.business_id = conv.business_id
		LEFT JOIN customer_identities i ON i.id = conv.customer_identity_id AND i.business_id = conv.business_id
		LEFT JOIN users u ON u.id = conv.assigned_user_id
		LEFT JOIN LATERAL (
			SELECT m.content, m.created_at, m.sender_type, m.direction, m.status
			FROM messages m
			WHERE m.business_id = conv.business_id
			  AND m.conversation_id = conv.id
			  AND m.message_type <> 'INTERNAL_NOTE'
			ORDER BY m.created_at DESC, m.id DESC
			LIMIT 1
		) last ON true
		WHERE conv.business_id = $1 AND conv.status = $2`
	args := []any{businessID, params.Status}
	n := 3
	if params.Channel != "" {
		query += fmt.Sprintf(` AND conv.channel = $%d`, n)
		args = append(args, params.Channel)
		n++
	}
	if params.AssignedUserID != nil {
		query += fmt.Sprintf(` AND conv.assigned_user_id = $%d`, n)
		args = append(args, *params.AssignedUserID)
		n++
	}
	if params.CustomerID != nil {
		query += fmt.Sprintf(` AND conv.customer_id = $%d`, n)
		args = append(args, *params.CustomerID)
		n++
	}
	if params.UnreadOnly {
		query += ` AND conv.unread_count > 0`
	}
	if params.Search != "" {
		query += fmt.Sprintf(` AND (
			c.name ILIKE $%d ESCAPE '\'
			OR COALESCE(c.phone, '') ILIKE $%d ESCAPE '\'
			OR COALESCE(c.email, '') ILIKE $%d ESCAPE '\'
		)`, n, n, n)
		args = append(args, params.Search)
		n++
	}
	query += fmt.Sprintf(`
		ORDER BY conv.last_message_at DESC NULLS LAST, conv.id DESC
		LIMIT $%d OFFSET $%d`, n, n+1)
	args = append(args, params.Limit, params.Offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, mapDBError(err, "conversation")
	}
	defer rows.Close()
	out := make([]Conversation, 0)
	for rows.Next() {
		item, err := scanConversationWithPreview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, mapDBError(rows.Err(), "conversation")
}

func (r *Repository) GetConversation(ctx context.Context, businessID, conversationID uuid.UUID) (*Conversation, error) {
	const query = `
		SELECT ` + conversationSelect + `
		FROM conversations conv
		JOIN customers c ON c.id = conv.customer_id AND c.business_id = conv.business_id
		LEFT JOIN customer_identities i ON i.id = conv.customer_identity_id AND i.business_id = conv.business_id
		LEFT JOIN users u ON u.id = conv.assigned_user_id
		WHERE conv.business_id = $1 AND conv.id = $2`
	return scanConversation(r.db.QueryRow(ctx, query, businessID, conversationID))
}

func (r *Repository) LockConversation(ctx context.Context, businessID, conversationID uuid.UUID) (*Conversation, error) {
	const query = `
		SELECT id FROM conversations
		WHERE business_id = $1 AND id = $2
		FOR UPDATE`
	var id uuid.UUID
	if err := r.db.QueryRow(ctx, query, businessID, conversationID).Scan(&id); err != nil {
		return nil, mapDBError(err, "conversation")
	}
	return r.GetConversation(ctx, businessID, conversationID)
}

func (r *Repository) FindOpenConversation(ctx context.Context, businessID, customerID uuid.UUID, channel Channel, identityID *uuid.UUID) (*Conversation, error) {
	query := `
		SELECT ` + conversationSelect + `
		FROM conversations conv
		JOIN customers c ON c.id = conv.customer_id AND c.business_id = conv.business_id
		LEFT JOIN customer_identities i ON i.id = conv.customer_identity_id AND i.business_id = conv.business_id
		LEFT JOIN users u ON u.id = conv.assigned_user_id
		WHERE conv.business_id = $1 AND conv.customer_id = $2 AND conv.channel = $3 AND conv.status = 'OPEN'`
	args := []any{businessID, customerID, channel}
	if identityID == nil {
		query += ` AND conv.customer_identity_id IS NULL`
	} else {
		query += ` AND conv.customer_identity_id = $4`
		args = append(args, *identityID)
	}
	conv, err := scanConversation(r.db.QueryRow(ctx, query, args...))
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return conv, err
}

func (r *Repository) FindLatestConversation(ctx context.Context, businessID, customerID uuid.UUID, channel Channel, identityID *uuid.UUID) (*Conversation, error) {
	query := `
		SELECT ` + conversationSelect + `
		FROM conversations conv
		JOIN customers c ON c.id = conv.customer_id AND c.business_id = conv.business_id
		LEFT JOIN customer_identities i ON i.id = conv.customer_identity_id AND i.business_id = conv.business_id
		LEFT JOIN users u ON u.id = conv.assigned_user_id
		WHERE conv.business_id = $1 AND conv.customer_id = $2 AND conv.channel = $3`
	args := []any{businessID, customerID, channel}
	if identityID == nil {
		query += ` AND conv.customer_identity_id IS NULL`
	} else {
		query += ` AND conv.customer_identity_id = $4`
		args = append(args, *identityID)
	}
	query += ` ORDER BY conv.last_message_at DESC NULLS LAST, conv.updated_at DESC, conv.id DESC LIMIT 1`
	conv, err := scanConversation(r.db.QueryRow(ctx, query, args...))
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return conv, err
}

func (r *Repository) CreateConversation(ctx context.Context, conv Conversation) (*Conversation, error) {
	const query = `
		INSERT INTO conversations (business_id, customer_id, channel, customer_identity_id, status)
		VALUES ($1, $2, $3, $4, 'OPEN')
		RETURNING id`
	var id uuid.UUID
	if err := r.db.QueryRow(ctx, query, conv.BusinessID, conv.CustomerID, conv.Channel, conv.CustomerIdentityID).Scan(&id); err != nil {
		return nil, mapDBError(err, "conversation")
	}
	return r.GetConversation(ctx, conv.BusinessID, id)
}

func (r *Repository) UpdateConversation(ctx context.Context, businessID uuid.UUID, conv Conversation) (*Conversation, error) {
	const query = `
		UPDATE conversations
		SET status = $3, assigned_user_id = $4,
		    handling_state = $5, handover_reason = $6,
		    handover_requested_at = $7, human_started_at = $8, resolved_at = $9
		WHERE business_id = $1 AND id = $2
		RETURNING id`
	var id uuid.UUID
	if err := r.db.QueryRow(ctx, query, businessID, conv.ID, conv.Status, conv.AssignedUserID,
		normalizeHandling(conv.HandlingState), conv.HandoverReason, conv.HandoverRequestedAt, conv.HumanStartedAt, conv.ResolvedAt,
	).Scan(&id); err != nil {
		return nil, mapDBError(err, "conversation")
	}
	return r.GetConversation(ctx, businessID, conv.ID)
}

func (r *Repository) MarkRead(ctx context.Context, businessID, conversationID uuid.UUID) (*Conversation, error) {
	const query = `
		UPDATE conversations
		SET unread_count = 0
		WHERE business_id = $1 AND id = $2
		RETURNING id`
	var id uuid.UUID
	if err := r.db.QueryRow(ctx, query, businessID, conversationID).Scan(&id); err != nil {
		return nil, mapDBError(err, "conversation")
	}
	return r.GetConversation(ctx, businessID, conversationID)
}

func (r *Repository) MarkInboundRead(ctx context.Context, businessID, conversationID uuid.UUID) ([]string, error) {
	const query = `
		WITH updated AS (
			UPDATE messages
			SET status = 'READ'
			WHERE business_id = $1
			  AND conversation_id = $2
			  AND direction = 'INBOUND'
			  AND sender_type = 'CUSTOMER'
			  AND status = 'RECEIVED'
			RETURNING external_id, created_at
		)
		SELECT external_id FROM updated ORDER BY created_at ASC`
	rows, err := r.db.Query(ctx, query, businessID, conversationID)
	if err != nil {
		return nil, mapDBError(err, "message")
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var externalID *string
		if err := rows.Scan(&externalID); err != nil {
			return nil, mapDBError(err, "message")
		}
		if externalID != nil && strings.TrimSpace(*externalID) != "" {
			ids = append(ids, strings.TrimSpace(*externalID))
		}
	}
	return ids, rows.Err()
}

func (r *Repository) ApplyMessageSideEffects(ctx context.Context, businessID, conversationID uuid.UUID, at time.Time, unreadDelta int, reopen bool) error {
	const query = `
		UPDATE conversations
		SET last_message_at = $3,
		    unread_count = unread_count + $4,
		    status = CASE WHEN $5 THEN 'OPEN' ELSE status END
		WHERE business_id = $1 AND id = $2`
	_, err := r.db.Exec(ctx, query, businessID, conversationID, at, unreadDelta, reopen)
	return mapDBError(err, "conversation")
}

func (r *Repository) GetCustomer(ctx context.Context, businessID, customerID uuid.UUID) (*CustomerRecord, error) {
	const query = `SELECT id, status, name FROM customers WHERE business_id = $1 AND id = $2`
	var rec CustomerRecord
	if err := r.db.QueryRow(ctx, query, businessID, customerID).Scan(&rec.ID, &rec.Status, &rec.Name); err != nil {
		return nil, mapDBError(err, "customer")
	}
	return &rec, nil
}

func (r *Repository) GetIdentity(ctx context.Context, businessID, identityID uuid.UUID) (*IdentityRecord, error) {
	const query = `SELECT id, customer_id, channel, status FROM customer_identities WHERE business_id = $1 AND id = $2`
	var rec IdentityRecord
	if err := r.db.QueryRow(ctx, query, businessID, identityID).Scan(&rec.ID, &rec.CustomerID, &rec.Channel, &rec.Status); err != nil {
		return nil, mapDBError(err, "identity")
	}
	return &rec, nil
}

func (r *Repository) CreateMessage(ctx context.Context, msg Message) (*Message, error) {
	const query = `
		INSERT INTO messages (business_id, conversation_id, direction, sender_type, message_type, content, external_id, status, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING ` + messageColumns
	var metadata any
	if len(msg.Metadata) > 0 {
		metadata = msg.Metadata
	}
	return scanMessage(r.db.QueryRow(ctx, query, msg.BusinessID, msg.ConversationID, msg.Direction, msg.SenderType, msg.MessageType, msg.Content, msg.ExternalID, msg.Status, metadata))
}

func (r *Repository) ListMessages(ctx context.Context, businessID, conversationID uuid.UUID, params MessageListParams) ([]Message, error) {
	query := `
		SELECT ` + messageColumns + `
		FROM messages
		WHERE business_id = $1 AND conversation_id = $2`
	args := []any{businessID, conversationID}
	n := 3
	if params.Before != nil {
		query += fmt.Sprintf(` AND created_at < $%d`, n)
		args = append(args, *params.Before)
		n++
	}
	if params.After != nil {
		query += fmt.Sprintf(` AND created_at > $%d`, n)
		args = append(args, *params.After)
		n++
	}
	query += fmt.Sprintf(`
		ORDER BY created_at ASC, id ASC
		LIMIT $%d OFFSET $%d`, n, n+1)
	args = append(args, params.Limit, params.Offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, mapDBError(err, "message")
	}
	defer rows.Close()
	out := make([]Message, 0)
	for rows.Next() {
		item, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, mapDBError(rows.Err(), "message")
}

func (r *Repository) ListRecentMessages(ctx context.Context, businessID, conversationID uuid.UUID, limit int) ([]Message, error) {
	const query = `
		SELECT ` + messageColumns + `
		FROM messages
		WHERE business_id = $1 AND conversation_id = $2
		ORDER BY created_at DESC, id DESC
		LIMIT $3`
	rows, err := r.db.Query(ctx, query, businessID, conversationID, limit)
	if err != nil {
		return nil, mapDBError(err, "message")
	}
	defer rows.Close()
	newestFirst := make([]Message, 0, limit)
	for rows.Next() {
		item, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		newestFirst = append(newestFirst, *item)
	}
	if err := mapDBError(rows.Err(), "message"); err != nil {
		return nil, err
	}
	out := make([]Message, 0, len(newestFirst))
	for i := len(newestFirst) - 1; i >= 0; i-- {
		out = append(out, newestFirst[i])
	}
	return out, nil
}

func (r *Repository) GetMessageByExternalID(ctx context.Context, businessID, conversationID uuid.UUID, externalID string) (*Message, error) {
	const query = `
		SELECT ` + messageColumns + `
		FROM messages
		WHERE business_id = $1 AND conversation_id = $2 AND external_id = $3`
	msg, err := scanMessage(r.db.QueryRow(ctx, query, businessID, conversationID, externalID))
	if errors.Is(err, ErrMessageNotFound) {
		return nil, nil
	}
	return msg, err
}

func (r *Repository) FindMessageByExternalID(ctx context.Context, businessID uuid.UUID, externalID string) (*Message, error) {
	const query = `
		SELECT ` + messageColumns + `
		FROM messages
		WHERE business_id = $1 AND external_id = $2
		ORDER BY created_at DESC, id DESC
		LIMIT 1`
	msg, err := scanMessage(r.db.QueryRow(ctx, query, businessID, externalID))
	if errors.Is(err, ErrMessageNotFound) {
		return nil, nil
	}
	return msg, err
}

func (r *Repository) GetMessage(ctx context.Context, businessID, messageID uuid.UUID) (*Message, error) {
	const query = `SELECT ` + messageColumns + ` FROM messages WHERE business_id = $1 AND id = $2`
	return scanMessage(r.db.QueryRow(ctx, query, businessID, messageID))
}

func (r *Repository) ListHumanInbox(ctx context.Context, businessID uuid.UUID, states []HandlingState, limit, offset int) ([]HumanInboxItem, error) {
	if len(states) == 0 {
		states = []HandlingState{HandlingHumanRequested, HandlingHumanActive}
	}
	stateVals := make([]string, 0, len(states))
	for _, state := range states {
		stateVals = append(stateVals, string(state))
	}
	query := `
		SELECT ` + conversationSelect + `,
		       last.content, last.created_at, last.sender_type, last.direction, last.status
		FROM conversations conv
		JOIN customers c ON c.id = conv.customer_id AND c.business_id = conv.business_id
		LEFT JOIN customer_identities i ON i.id = conv.customer_identity_id AND i.business_id = conv.business_id
		LEFT JOIN users u ON u.id = conv.assigned_user_id
		LEFT JOIN LATERAL (
			SELECT m.content, m.created_at, m.sender_type, m.direction, m.status
			FROM messages m
			WHERE m.business_id = conv.business_id
			  AND m.conversation_id = conv.id
			  AND m.message_type <> 'INTERNAL_NOTE'
			ORDER BY m.created_at DESC, m.id DESC
			LIMIT 1
		) last ON true
		WHERE conv.business_id = $1 AND conv.handling_state = ANY($2)
		ORDER BY
			CASE conv.handling_state
				WHEN 'HUMAN_REQUESTED' THEN 0
				WHEN 'HUMAN_ACTIVE' THEN 1
				ELSE 2
			END,
			conv.handover_requested_at ASC NULLS LAST,
			conv.updated_at DESC
		LIMIT $3 OFFSET $4`
	rows, err := r.db.Query(ctx, query, businessID, stateVals, limit, offset)
	if err != nil {
		return nil, mapDBError(err, "conversation")
	}
	defer rows.Close()
	out := make([]HumanInboxItem, 0)
	for rows.Next() {
		item, err := scanHumanInbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) ClaimForDelivery(ctx context.Context, businessID, messageID uuid.UUID, staleAfter time.Duration) (*Message, error) {
	if staleAfter <= 0 {
		staleAfter = 2 * time.Minute
	}
	const query = `
		UPDATE messages
		SET status = 'SENDING'
		WHERE business_id = $1 AND id = $2
		  AND message_type <> 'INTERNAL_NOTE'
		  AND (
		      status = 'QUEUED'
		      OR (status = 'SENDING' AND updated_at < now() - make_interval(secs => $3))
		  )
		RETURNING ` + messageColumns
	msg, err := scanMessage(r.db.QueryRow(ctx, query, businessID, messageID, int(staleAfter.Seconds())))
	if errors.Is(err, ErrMessageNotFound) {
		return nil, nil
	}
	return msg, err
}

func (r *Repository) UpdateMessageDelivery(ctx context.Context, businessID, messageID uuid.UUID, status MessageStatus, externalID *string) (*Message, error) {
	const query = `
		UPDATE messages
		SET status = $3, external_id = COALESCE($4, external_id)
		WHERE business_id = $1 AND id = $2
		RETURNING ` + messageColumns
	return scanMessage(r.db.QueryRow(ctx, query, businessID, messageID, status, externalID))
}

type scanner interface {
	Scan(dest ...any) error
}

func scanConversation(row scanner) (*Conversation, error) {
	var (
		c            Conversation
		identUUID    *uuid.UUID
		identChannel *string
		identExt     *string
		assignedID   *uuid.UUID
		assignedName *string
	)
	err := row.Scan(
		&c.ID, &c.BusinessID, &c.CustomerID, &c.Channel, &c.CustomerIdentityID,
		&c.Status, &c.AssignedUserID, &c.LastMessageAt, &c.UnreadCount,
		&c.HandlingState, &c.HandoverReason, &c.HandoverRequestedAt, &c.HumanStartedAt, &c.ResolvedAt,
		&c.CreatedAt, &c.UpdatedAt,
		&c.Customer.ID, &c.Customer.Name, &c.Customer.Phone, &c.Customer.Email,
		&identUUID, &identChannel, &identExt,
		&assignedID, &assignedName,
	)
	if err != nil {
		return nil, mapDBError(err, "conversation")
	}
	if identUUID != nil && identChannel != nil && identExt != nil {
		c.Identity = &IdentitySummary{ID: *identUUID, Channel: Channel(*identChannel), ExternalID: *identExt}
	}
	if assignedID != nil && assignedName != nil {
		c.AssignedUser = &UserSummary{ID: *assignedID, FullName: *assignedName}
	}
	c.HandlingState = normalizeHandling(c.HandlingState)
	return &c, nil
}

func scanConversationWithPreview(row scanner) (*Conversation, error) {
	var (
		c            Conversation
		identUUID    *uuid.UUID
		identChannel *string
		identExt     *string
		assignedID   *uuid.UUID
		assignedName *string
		content      *string
		createdAt    *time.Time
		senderType   *string
		direction    *string
		status       *string
	)
	err := row.Scan(
		&c.ID, &c.BusinessID, &c.CustomerID, &c.Channel, &c.CustomerIdentityID,
		&c.Status, &c.AssignedUserID, &c.LastMessageAt, &c.UnreadCount,
		&c.HandlingState, &c.HandoverReason, &c.HandoverRequestedAt, &c.HumanStartedAt, &c.ResolvedAt,
		&c.CreatedAt, &c.UpdatedAt,
		&c.Customer.ID, &c.Customer.Name, &c.Customer.Phone, &c.Customer.Email,
		&identUUID, &identChannel, &identExt,
		&assignedID, &assignedName,
		&content, &createdAt, &senderType, &direction, &status,
	)
	if err != nil {
		return nil, mapDBError(err, "conversation")
	}
	if identUUID != nil && identChannel != nil && identExt != nil {
		c.Identity = &IdentitySummary{ID: *identUUID, Channel: Channel(*identChannel), ExternalID: *identExt}
	}
	if assignedID != nil && assignedName != nil {
		c.AssignedUser = &UserSummary{ID: *assignedID, FullName: *assignedName}
	}
	c.HandlingState = normalizeHandling(c.HandlingState)
	if createdAt != nil && senderType != nil && direction != nil && status != nil {
		c.LastMessage = &MessagePreview{
			Content:    content,
			CreatedAt:  *createdAt,
			SenderType: SenderType(*senderType),
			Direction:  Direction(*direction),
			Status:     MessageStatus(*status),
		}
	}
	return &c, nil
}

func scanHumanInbox(row scanner) (*HumanInboxItem, error) {
	conv, err := scanConversationWithPreview(row)
	if err != nil {
		return nil, err
	}
	return &HumanInboxItem{Conversation: *conv}, nil
}

func scanMessage(row scanner) (*Message, error) {
	var m Message
	var metadata []byte
	if err := row.Scan(&m.ID, &m.BusinessID, &m.ConversationID, &m.Direction, &m.SenderType, &m.MessageType, &m.Content, &m.ExternalID, &m.Status, &metadata, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return nil, mapDBError(err, "message")
	}
	if len(metadata) > 0 {
		m.Metadata = json.RawMessage(metadata)
	}
	return &m, nil
}

func mapDBError(err error, kind string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		switch kind {
		case "customer":
			return ErrCustomerNotFound
		case "identity":
			return ErrIdentityNotFound
		case "message":
			return ErrMessageNotFound
		default:
			return ErrNotFound
		}
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			switch pgErr.ConstraintName {
			case "uq_messages_conversation_external_id", "uq_messages_business_external_id":
				return ErrDuplicateExternalID
			case "uq_conversations_open_with_identity", "uq_conversations_open_without_identity":
				return ErrOpenConversationExists
			}
		case "23514":
			return invalid("invalid value")
		}
	}
	return fmt.Errorf("database: %w", err)
}
