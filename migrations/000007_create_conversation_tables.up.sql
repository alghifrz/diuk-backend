-- Provider-agnostic conversation threads and messages. Channel adapters,
-- AI, and reservations are later jobs; these tables stay generic.

ALTER TABLE customer_identities
    ADD CONSTRAINT uq_customer_identities_id_business_customer
    UNIQUE (id, business_id, customer_id);

CREATE TABLE conversations (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id           UUID        NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    customer_id           UUID        NOT NULL,
    channel               VARCHAR(30) NOT NULL CHECK (channel IN ('WHATSAPP', 'INSTAGRAM', 'WEBSITE')),
    customer_identity_id  UUID,
    status                VARCHAR(20) NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'CLOSED')),
    assigned_user_id      UUID        REFERENCES users (id) ON DELETE RESTRICT,
    last_message_at       TIMESTAMPTZ,
    unread_count          INTEGER     NOT NULL DEFAULT 0 CHECK (unread_count >= 0),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_conversations_id_business_id UNIQUE (id, business_id),
    CONSTRAINT fk_conversations_customer_business
        FOREIGN KEY (customer_id, business_id)
        REFERENCES customers (id, business_id)
        ON DELETE RESTRICT,
    CONSTRAINT fk_conversations_identity_customer_business
        FOREIGN KEY (customer_identity_id, business_id, customer_id)
        REFERENCES customer_identities (id, business_id, customer_id)
        ON DELETE RESTRICT
);

CREATE UNIQUE INDEX uq_conversations_open_with_identity
    ON conversations (business_id, customer_id, channel, customer_identity_id)
    WHERE status = 'OPEN' AND customer_identity_id IS NOT NULL;

CREATE UNIQUE INDEX uq_conversations_open_without_identity
    ON conversations (business_id, customer_id, channel)
    WHERE status = 'OPEN' AND customer_identity_id IS NULL;

CREATE INDEX idx_conversations_business_id_status_last_message_at
    ON conversations (business_id, status, last_message_at DESC);
CREATE INDEX idx_conversations_business_id_customer_id
    ON conversations (business_id, customer_id);
CREATE INDEX idx_conversations_business_id_channel_status
    ON conversations (business_id, channel, status);
CREATE INDEX idx_conversations_business_id_assigned_user_id_status
    ON conversations (business_id, assigned_user_id, status);
CREATE INDEX idx_conversations_business_id_unread_count
    ON conversations (business_id, unread_count);

CREATE TRIGGER trg_conversations_updated_at
    BEFORE UPDATE ON conversations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE messages (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id     UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    conversation_id UUID         NOT NULL,
    direction       VARCHAR(20)  NOT NULL CHECK (direction IN ('INBOUND', 'OUTBOUND')),
    sender_type     VARCHAR(20)  NOT NULL CHECK (sender_type IN ('CUSTOMER', 'USER', 'SYSTEM', 'AI')),
    message_type    VARCHAR(30)  NOT NULL DEFAULT 'TEXT' CHECK (message_type IN ('TEXT', 'IMAGE', 'VIDEO', 'AUDIO', 'FILE', 'LOCATION', 'STICKER', 'SYSTEM')),
    content         TEXT,
    external_id     VARCHAR(255) CHECK (external_id IS NULL OR btrim(external_id) <> ''),
    status          VARCHAR(30)  NOT NULL DEFAULT 'RECEIVED' CHECK (status IN ('RECEIVED', 'QUEUED', 'SENT', 'DELIVERED', 'READ', 'FAILED')),
    metadata        JSONB,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT fk_messages_conversation_business
        FOREIGN KEY (conversation_id, business_id)
        REFERENCES conversations (id, business_id)
        ON DELETE RESTRICT
);

CREATE UNIQUE INDEX uq_messages_conversation_external_id
    ON messages (business_id, conversation_id, external_id)
    WHERE external_id IS NOT NULL;

CREATE INDEX idx_messages_business_id_conversation_id_created_at
    ON messages (business_id, conversation_id, created_at);
CREATE INDEX idx_messages_business_id_conversation_id_id
    ON messages (business_id, conversation_id, id);
CREATE INDEX idx_messages_business_id_direction_created_at
    ON messages (business_id, direction, created_at);
CREATE INDEX idx_messages_business_id_external_id
    ON messages (business_id, external_id)
    WHERE external_id IS NOT NULL;

CREATE TRIGGER trg_messages_updated_at
    BEFORE UPDATE ON messages
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
