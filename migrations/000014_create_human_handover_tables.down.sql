ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_status_check;
ALTER TABLE messages ADD CONSTRAINT messages_status_check
    CHECK (status IN ('RECEIVED', 'QUEUED', 'SENDING', 'SENT', 'DELIVERED', 'READ', 'FAILED'));

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_message_type_check;
ALTER TABLE messages ADD CONSTRAINT messages_message_type_check
    CHECK (message_type IN ('TEXT', 'IMAGE', 'VIDEO', 'AUDIO', 'FILE', 'LOCATION', 'STICKER', 'SYSTEM'));

DROP INDEX IF EXISTS idx_conversations_business_id_handling_state;

ALTER TABLE conversations
    DROP COLUMN IF EXISTS resolved_at,
    DROP COLUMN IF EXISTS human_started_at,
    DROP COLUMN IF EXISTS handover_requested_at,
    DROP COLUMN IF EXISTS handover_reason,
    DROP COLUMN IF EXISTS handling_state;
