-- JOB 16: conversation handling state for shared human handover.

ALTER TABLE conversations
    ADD COLUMN handling_state TEXT NOT NULL DEFAULT 'AI_ACTIVE'
        CHECK (handling_state IN ('AI_ACTIVE', 'HUMAN_REQUESTED', 'HUMAN_ACTIVE', 'RESOLVED')),
    ADD COLUMN handover_reason TEXT
        CHECK (handover_reason IS NULL OR handover_reason IN (
            'CUSTOMER_REQUESTED_HUMAN',
            'AI_UNABLE_TO_ANSWER',
            'MANUAL_HANDOFF'
        )),
    ADD COLUMN handover_requested_at TIMESTAMPTZ,
    ADD COLUMN human_started_at TIMESTAMPTZ,
    ADD COLUMN resolved_at TIMESTAMPTZ;

CREATE INDEX idx_conversations_business_id_handling_state
    ON conversations (business_id, handling_state, handover_requested_at ASC NULLS LAST);

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_message_type_check;
ALTER TABLE messages ADD CONSTRAINT messages_message_type_check
    CHECK (message_type IN ('TEXT', 'IMAGE', 'VIDEO', 'AUDIO', 'FILE', 'LOCATION', 'STICKER', 'SYSTEM', 'INTERNAL_NOTE'));

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_status_check;
ALTER TABLE messages ADD CONSTRAINT messages_status_check
    CHECK (status IN ('RECEIVED', 'QUEUED', 'SENDING', 'SENT', 'DELIVERED', 'READ', 'FAILED', 'INTERNAL'));
