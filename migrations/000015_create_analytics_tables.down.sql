DROP INDEX IF EXISTS idx_automation_runs_business_id_created_at;
DROP INDEX IF EXISTS idx_ai_generations_business_id_created_at;
DROP INDEX IF EXISTS idx_messages_business_id_created_at_customer_facing;
DROP INDEX IF EXISTS idx_conversations_business_id_handover_requested_at;
DROP INDEX IF EXISTS idx_conversations_business_id_created_at;
DROP INDEX IF EXISTS idx_customers_business_id_created_at;

ALTER TABLE ai_generations
    DROP COLUMN IF EXISTS total_tokens,
    DROP COLUMN IF EXISTS output_tokens,
    DROP COLUMN IF EXISTS input_tokens,
    DROP COLUMN IF EXISTS latency_ms,
    DROP COLUMN IF EXISTS completed_at,
    DROP COLUMN IF EXISTS started_at,
    DROP COLUMN IF EXISTS model,
    DROP COLUMN IF EXISTS provider,
    DROP COLUMN IF EXISTS status;
