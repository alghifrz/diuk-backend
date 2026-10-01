package campaign

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func audienceSQL(def AudienceDefinition, start int) (string, []any) {
	args := []any{}
	n := start
	next := func(v any) string {
		args = append(args, v)
		placeholder := fmt.Sprintf("$%d", n)
		n++
		return placeholder
	}

	var b strings.Builder
	b.WriteString(` c.business_id = $1`)

	if def.CustomerStatus != nil {
		b.WriteString(` AND c.status = ` + next(*def.CustomerStatus))
	}
	if t := optionalAudienceTime(def.CreatedAfter); t != nil {
		b.WriteString(` AND c.created_at >= ` + next(*t))
	}
	if t := optionalAudienceTime(def.CreatedBefore); t != nil {
		b.WriteString(` AND c.created_at < ` + next(*t))
	}
	if len(def.Tags) > 0 {
		ph := next(def.Tags)
		b.WriteString(` AND (
			SELECT COUNT(DISTINCT t.name)
			FROM customer_tags ct
			JOIN tags t ON t.id = ct.tag_id AND t.business_id = ct.business_id
			WHERE ct.customer_id = c.id AND ct.business_id = c.business_id
			  AND t.status = 'ACTIVE' AND t.name = ANY(` + ph + `)
		) = ` + next(len(def.Tags)))
	}

	needConversation := def.HasConversation != nil || def.LastConversationAfter != nil || def.LastConversationBefore != nil
	if needConversation {
		conv := `EXISTS (
			SELECT 1 FROM conversations conv
			WHERE conv.customer_id = c.id AND conv.business_id = c.business_id`
		if t := optionalAudienceTime(def.LastConversationAfter); t != nil {
			conv += ` AND COALESCE(conv.last_message_at, conv.created_at) >= ` + next(*t)
		}
		if t := optionalAudienceTime(def.LastConversationBefore); t != nil {
			conv += ` AND COALESCE(conv.last_message_at, conv.created_at) < ` + next(*t)
		}
		conv += `)`
		if def.HasConversation != nil && !*def.HasConversation {
			b.WriteString(` AND NOT ` + conv)
		} else {
			b.WriteString(` AND ` + conv)
		}
	}

	needReservation := def.HasReservation != nil || def.LastReservationStatus != nil || def.LastReservationAfter != nil || def.LastReservationBefore != nil
	if needReservation {
		res := `EXISTS (
			SELECT 1 FROM reservations r
			WHERE r.customer_id = c.id AND r.business_id = c.business_id`
		if def.LastReservationStatus != nil {
			res += ` AND r.status = ` + next(*def.LastReservationStatus)
		}
		if t := optionalAudienceTime(def.LastReservationAfter); t != nil {
			res += ` AND r.start_at >= ` + next(*t)
		}
		if t := optionalAudienceTime(def.LastReservationBefore); t != nil {
			res += ` AND r.start_at < ` + next(*t)
		}
		res += `)`
		if def.HasReservation != nil && !*def.HasReservation {
			b.WriteString(` AND NOT ` + res)
		} else {
			b.WriteString(` AND ` + res)
		}
	}

	if len(def.Channels) > 0 {
		b.WriteString(` AND EXISTS (
			SELECT 1 FROM customer_identities i
			WHERE i.customer_id = c.id AND i.business_id = c.business_id
			  AND i.status = 'ACTIVE' AND i.channel = ANY(` + next(def.Channels) + `)
		)`)
	}

	return b.String(), args
}

func (r *Repository) SnapshotRecipients(ctx context.Context, businessID, runID uuid.UUID, channel string, def AudienceDefinition) (int, error) {
	where, args := audienceSQL(def, 4)
	query := `
		INSERT INTO campaign_recipients (
			business_id, campaign_run_id, customer_id, channel, external_recipient_id,
			status, skipped_at, failure_code, failure_reason
		)
		SELECT $1, $2, c.id, $3,
			i.external_id,
			CASE
				WHEN COALESCE(c.marketing_opt_in, true) = false THEN 'SKIPPED'
				WHEN i.external_id IS NULL THEN 'SKIPPED'
				ELSE 'PENDING'
			END,
			CASE
				WHEN COALESCE(c.marketing_opt_in, true) = false OR i.external_id IS NULL THEN now()
				ELSE NULL
			END,
			CASE
				WHEN COALESCE(c.marketing_opt_in, true) = false THEN 'MARKETING_OPT_OUT'
				WHEN i.external_id IS NULL THEN 'NO_CHANNEL_IDENTITY'
				ELSE NULL
			END,
			CASE
				WHEN COALESCE(c.marketing_opt_in, true) = false THEN 'customer opted out of marketing'
				WHEN i.external_id IS NULL THEN 'no active channel identity'
				ELSE NULL
			END
		FROM customers c
		LEFT JOIN LATERAL (
			SELECT external_id
			FROM customer_identities ci
			WHERE ci.customer_id = c.id AND ci.business_id = c.business_id
			  AND ci.channel = $3 AND ci.status = 'ACTIVE'
			ORDER BY ci.created_at ASC
			LIMIT 1
		) i ON true
		WHERE ` + where + `
		ON CONFLICT (business_id, campaign_run_id, customer_id) DO NOTHING`
	all := append([]any{businessID, runID, channel}, args...)
	tag, err := r.db.Exec(ctx, query, all...)
	if err != nil {
		return 0, fmt.Errorf("database: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (r *Repository) PreviewAudience(ctx context.Context, businessID uuid.UUID, channel string, def AudienceDefinition) (AudiencePreview, error) {
	where, args := audienceSQL(def, 2)
	var count int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM customers c WHERE `+where, append([]any{businessID}, args...)...).Scan(&count); err != nil {
		return AudiencePreview{}, fmt.Errorf("database: %w", err)
	}
	rows, err := r.db.Query(ctx, `
		SELECT c.id, c.name, c.status
		FROM customers c
		WHERE `+where+`
		ORDER BY c.created_at DESC, c.id DESC
		LIMIT $`+fmt.Sprintf("%d", len(args)+2),
		append(append([]any{businessID}, args...), previewSample)...)
	if err != nil {
		return AudiencePreview{}, fmt.Errorf("database: %w", err)
	}
	defer rows.Close()
	sample := []AudienceSample{}
	for rows.Next() {
		var item AudienceSample
		if err := rows.Scan(&item.ID, &item.Name, &item.Status); err != nil {
			return AudiencePreview{}, fmt.Errorf("database: %w", err)
		}
		sample = append(sample, item)
	}
	return AudiencePreview{EstimatedCount: count, Sample: sample}, rows.Err()
}

func (r *Repository) CustomerSendable(ctx context.Context, businessID, customerID uuid.UUID, channel string) (optIn bool, identityID *uuid.UUID, externalID string, err error) {
	const query = `
		SELECT c.marketing_opt_in, i.id, i.external_id
		FROM customers c
		LEFT JOIN LATERAL (
			SELECT id, external_id
			FROM customer_identities
			WHERE customer_id = c.id AND business_id = c.business_id
			  AND channel = $3 AND status = 'ACTIVE'
			ORDER BY created_at ASC
			LIMIT 1
		) i ON true
		WHERE c.business_id = $1 AND c.id = $2`
	var id *uuid.UUID
	var ext *string
	err = r.db.QueryRow(ctx, query, businessID, customerID, channel).Scan(&optIn, &id, &ext)
	if err != nil {
		return false, nil, "", fmt.Errorf("database: %w", err)
	}
	if ext != nil {
		externalID = *ext
	}
	return optIn, id, externalID, nil
}
