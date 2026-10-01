-- JOB 13: durable outbox, AI generation claim, and atomic delivery claim.

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_status_check;
ALTER TABLE messages ADD CONSTRAINT messages_status_check
    CHECK (status IN ('RECEIVED', 'QUEUED', 'SENDING', 'SENT', 'DELIVERED', 'READ', 'FAILED'));

CREATE TABLE job_outbox (
    id            UUID         PRIMARY KEY,
    job_type      VARCHAR(40)  NOT NULL CHECK (job_type IN ('AI_GENERATE', 'MESSAGE_DELIVERY')),
    payload       JSONB        NOT NULL,
    status        VARCHAR(20)  NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'PUBLISHED', 'FAILED')),
    attempts      INTEGER      NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts  INTEGER      NOT NULL DEFAULT 3 CHECK (max_attempts >= 1),
    available_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    processed_at  TIMESTAMPTZ,
    last_error    TEXT
);

CREATE INDEX idx_job_outbox_pending_available
    ON job_outbox (available_at, created_at)
    WHERE status = 'PENDING';

CREATE INDEX idx_job_outbox_job_type_status
    ON job_outbox (job_type, status);

-- One AI generation per inbound customer message. outbound_message_id is
-- filled after GenerateReply so crashed in-flight claims can be retried.
CREATE TABLE ai_generations (
    inbound_message_id  UUID         PRIMARY KEY REFERENCES messages (id) ON DELETE RESTRICT,
    business_id         UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    conversation_id     UUID         NOT NULL,
    outbound_message_id UUID         REFERENCES messages (id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT fk_ai_generations_conversation_business
        FOREIGN KEY (conversation_id, business_id)
        REFERENCES conversations (id, business_id)
        ON DELETE RESTRICT
);

CREATE INDEX idx_ai_generations_business_conversation
    ON ai_generations (business_id, conversation_id);
