-- JOB 18: outbound campaigns. Delivery still uses MESSAGE_DELIVERY.

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_sender_type_check;
ALTER TABLE messages ADD CONSTRAINT messages_sender_type_check
    CHECK (sender_type IN ('CUSTOMER', 'USER', 'SYSTEM', 'AI', 'CAMPAIGN'));

ALTER TABLE customers
    ADD COLUMN IF NOT EXISTS marketing_opt_in BOOLEAN NOT NULL DEFAULT true;

ALTER TABLE job_outbox DROP CONSTRAINT IF EXISTS job_outbox_job_type_check;
ALTER TABLE job_outbox ADD CONSTRAINT job_outbox_job_type_check
    CHECK (job_type IN (
        'AI_GENERATE',
        'MESSAGE_DELIVERY',
        'KNOWLEDGE_INGEST',
        'AUTOMATION_EVALUATE',
        'AUTOMATION_RUN',
        'CAMPAIGN_START',
        'CAMPAIGN_DISPATCH'
    ));

CREATE TABLE campaigns (
    id                   UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id          UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    name                 TEXT         NOT NULL CHECK (btrim(name) <> '' AND char_length(name) <= 150),
    description          TEXT         CHECK (description IS NULL OR btrim(description) <> ''),
    status               TEXT         NOT NULL DEFAULT 'DRAFT'
                                      CHECK (status IN ('DRAFT', 'SCHEDULED', 'RUNNING', 'PAUSED', 'COMPLETED', 'CANCELLED')),
    channel              TEXT         NOT NULL CHECK (channel IN ('WHATSAPP')),
    message_kind         TEXT         NOT NULL DEFAULT 'TEMPLATE'
                                      CHECK (message_kind IN ('TEXT', 'TEMPLATE')),
    template_name        TEXT         CHECK (template_name IS NULL OR btrim(template_name) <> ''),
    template_language    TEXT         CHECK (template_language IS NULL OR btrim(template_language) <> ''),
    template_parameters  JSONB        NOT NULL DEFAULT '[]'::jsonb,
    audience_type        TEXT         NOT NULL DEFAULT 'FILTER'
                                      CHECK (audience_type IN ('FILTER')),
    audience_definition  JSONB        NOT NULL DEFAULT '{}'::jsonb,
    message_body         TEXT         CHECK (message_body IS NULL OR btrim(message_body) <> ''),
    scheduled_at         TIMESTAMPTZ,
    started_at           TIMESTAMPTZ,
    paused_at            TIMESTAMPTZ,
    completed_at         TIMESTAMPTZ,
    cancelled_at         TIMESTAMPTZ,
    created_by           UUID         REFERENCES users (id) ON DELETE RESTRICT,
    updated_by           UUID         REFERENCES users (id) ON DELETE RESTRICT,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_campaigns_id_business UNIQUE (id, business_id),
    CONSTRAINT chk_campaigns_template CHECK (
        message_kind <> 'TEMPLATE'
        OR (template_name IS NOT NULL AND template_language IS NOT NULL)
    ),
    CONSTRAINT chk_campaigns_text CHECK (
        message_kind <> 'TEXT' OR message_body IS NOT NULL
    )
);

CREATE INDEX idx_campaigns_business_id_status
    ON campaigns (business_id, status, updated_at DESC);

CREATE INDEX idx_campaigns_business_id_scheduled_at
    ON campaigns (business_id, scheduled_at)
    WHERE status = 'SCHEDULED' AND scheduled_at IS NOT NULL;

CREATE TRIGGER trg_campaigns_updated_at
    BEFORE UPDATE ON campaigns
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE campaign_runs (
    id                UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id       UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    campaign_id       UUID         NOT NULL,
    status            TEXT         NOT NULL DEFAULT 'PENDING'
                                   CHECK (status IN ('PENDING', 'RUNNING', 'PAUSED', 'COMPLETED', 'CANCELLED', 'FAILED')),
    scheduled_at      TIMESTAMPTZ,
    started_at        TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ,
    paused_at         TIMESTAMPTZ,
    cancelled_at      TIMESTAMPTZ,
    total_recipients  INTEGER      NOT NULL DEFAULT 0 CHECK (total_recipients >= 0),
    queued_count      INTEGER      NOT NULL DEFAULT 0 CHECK (queued_count >= 0),
    sent_count        INTEGER      NOT NULL DEFAULT 0 CHECK (sent_count >= 0),
    delivered_count   INTEGER      NOT NULL DEFAULT 0 CHECK (delivered_count >= 0),
    read_count        INTEGER      NOT NULL DEFAULT 0 CHECK (read_count >= 0),
    failed_count      INTEGER      NOT NULL DEFAULT 0 CHECK (failed_count >= 0),
    skipped_count     INTEGER      NOT NULL DEFAULT 0 CHECK (skipped_count >= 0),
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_campaign_runs_id_business UNIQUE (id, business_id),
    CONSTRAINT fk_campaign_runs_campaign_business
        FOREIGN KEY (campaign_id, business_id)
        REFERENCES campaigns (id, business_id)
        ON DELETE RESTRICT
);

CREATE UNIQUE INDEX uq_campaign_runs_one_active
    ON campaign_runs (business_id, campaign_id)
    WHERE status IN ('PENDING', 'RUNNING', 'PAUSED');

CREATE INDEX idx_campaign_runs_business_id_campaign_id_created_at
    ON campaign_runs (business_id, campaign_id, created_at DESC);

CREATE INDEX idx_campaign_runs_business_id_status
    ON campaign_runs (business_id, status, created_at DESC);

CREATE TRIGGER trg_campaign_runs_updated_at
    BEFORE UPDATE ON campaign_runs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE campaign_recipients (
    id                     UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id            UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    campaign_run_id        UUID         NOT NULL,
    customer_id            UUID         NOT NULL,
    channel                TEXT         NOT NULL CHECK (channel IN ('WHATSAPP')),
    external_recipient_id  TEXT         CHECK (external_recipient_id IS NULL OR btrim(external_recipient_id) <> ''),
    status                 TEXT         NOT NULL DEFAULT 'PENDING'
                                        CHECK (status IN ('PENDING', 'QUEUED', 'SENT', 'DELIVERED', 'READ', 'FAILED', 'SKIPPED')),
    message_id             UUID         REFERENCES messages (id) ON DELETE RESTRICT,
    queued_at              TIMESTAMPTZ,
    sent_at                TIMESTAMPTZ,
    delivered_at           TIMESTAMPTZ,
    read_at                TIMESTAMPTZ,
    failed_at              TIMESTAMPTZ,
    skipped_at             TIMESTAMPTZ,
    failure_code           TEXT         CHECK (failure_code IS NULL OR btrim(failure_code) <> ''),
    failure_reason         TEXT         CHECK (failure_reason IS NULL OR btrim(failure_reason) <> ''),
    created_at             TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_campaign_recipients_run_customer UNIQUE (business_id, campaign_run_id, customer_id),
    CONSTRAINT fk_campaign_recipients_run_business
        FOREIGN KEY (campaign_run_id, business_id)
        REFERENCES campaign_runs (id, business_id)
        ON DELETE RESTRICT,
    CONSTRAINT fk_campaign_recipients_customer_business
        FOREIGN KEY (customer_id, business_id)
        REFERENCES customers (id, business_id)
        ON DELETE RESTRICT
);

CREATE UNIQUE INDEX uq_campaign_recipients_message
    ON campaign_recipients (message_id)
    WHERE message_id IS NOT NULL;

CREATE INDEX idx_campaign_recipients_business_run_status
    ON campaign_recipients (business_id, campaign_run_id, status);

CREATE INDEX idx_campaign_recipients_business_customer
    ON campaign_recipients (business_id, customer_id);

CREATE INDEX idx_campaign_recipients_business_run_id
    ON campaign_recipients (business_id, campaign_run_id, id);

CREATE TRIGGER trg_campaign_recipients_updated_at
    BEFORE UPDATE ON campaign_recipients
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO permissions (code, resource, action, description) VALUES
    ('campaign.read',   'campaign', 'read',   'Read campaigns, runs, and recipients'),
    ('campaign.create', 'campaign', 'create', 'Create campaigns'),
    ('campaign.update', 'campaign', 'update', 'Update, schedule, start, pause, resume, or cancel campaigns'),
    ('campaign.delete', 'campaign', 'delete', 'Cancel or archive campaigns')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name = 'OWNER' AND p.code LIKE 'campaign.%'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN ('campaign.read', 'campaign.create', 'campaign.update', 'campaign.delete')
WHERE r.name = 'MANAGER'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'campaign.read'
WHERE r.name = 'STAFF'
ON CONFLICT DO NOTHING;
