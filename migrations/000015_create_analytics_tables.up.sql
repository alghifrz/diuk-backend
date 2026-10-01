-- JOB 17: analytics query indexes and minimal AI generation telemetry.
-- Business metrics are derived from existing transactional tables.

ALTER TABLE ai_generations
    ADD COLUMN IF NOT EXISTS status TEXT
        CHECK (status IN ('PENDING', 'SUCCESS', 'FAILED', 'SKIPPED')),
    ADD COLUMN IF NOT EXISTS provider TEXT,
    ADD COLUMN IF NOT EXISTS model TEXT,
    ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS latency_ms INTEGER
        CHECK (latency_ms IS NULL OR latency_ms >= 0),
    ADD COLUMN IF NOT EXISTS input_tokens INTEGER
        CHECK (input_tokens IS NULL OR input_tokens >= 0),
    ADD COLUMN IF NOT EXISTS output_tokens INTEGER
        CHECK (output_tokens IS NULL OR output_tokens >= 0),
    ADD COLUMN IF NOT EXISTS total_tokens INTEGER
        CHECK (total_tokens IS NULL OR total_tokens >= 0);

-- Status is reconstructed from existing outbound_message_id only.
-- Timing and token values are not invented for historical rows.
UPDATE ai_generations
SET status = CASE
    WHEN outbound_message_id IS NULL THEN 'PENDING'
    WHEN outbound_message_id = '00000000-0000-0000-0000-000000000000' THEN 'SKIPPED'
    ELSE 'SUCCESS'
END
WHERE status IS NULL;

ALTER TABLE ai_generations
    ALTER COLUMN status SET DEFAULT 'PENDING';

CREATE INDEX IF NOT EXISTS idx_customers_business_id_created_at
    ON customers (business_id, created_at);

CREATE INDEX IF NOT EXISTS idx_conversations_business_id_created_at
    ON conversations (business_id, created_at);

CREATE INDEX IF NOT EXISTS idx_conversations_business_id_handover_requested_at
    ON conversations (business_id, handover_requested_at)
    WHERE handover_requested_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_messages_business_id_created_at_customer_facing
    ON messages (business_id, created_at)
    WHERE message_type <> 'INTERNAL_NOTE';

CREATE INDEX IF NOT EXISTS idx_ai_generations_business_id_created_at
    ON ai_generations (business_id, created_at);

CREATE INDEX IF NOT EXISTS idx_automation_runs_business_id_created_at
    ON automation_runs (business_id, created_at);
