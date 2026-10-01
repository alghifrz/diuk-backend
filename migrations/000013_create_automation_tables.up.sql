-- JOB 15: tenant automations, durable events, idempotent runs, and job types.

ALTER TABLE job_outbox DROP CONSTRAINT IF EXISTS job_outbox_job_type_check;
ALTER TABLE job_outbox ADD CONSTRAINT job_outbox_job_type_check
    CHECK (job_type IN (
        'AI_GENERATE',
        'MESSAGE_DELIVERY',
        'KNOWLEDGE_INGEST',
        'AUTOMATION_EVALUATE',
        'AUTOMATION_RUN'
    ));

CREATE TABLE automations (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id     UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    name            TEXT         NOT NULL CHECK (btrim(name) <> ''),
    description     TEXT         CHECK (description IS NULL OR btrim(description) <> ''),
    status          TEXT         NOT NULL DEFAULT 'INACTIVE'
                                 CHECK (status IN ('ACTIVE', 'INACTIVE', 'ARCHIVED')),
    trigger_type    TEXT         NOT NULL
                                 CHECK (trigger_type IN (
                                     'RESERVATION_CONFIRMED',
                                     'RESERVATION_CANCELLED',
                                     'CONVERSATION_CREATED',
                                     'CONVERSATION_INACTIVE',
                                     'CUSTOMER_CREATED',
                                     'SCHEDULED'
                                 )),
    trigger_config  JSONB        NOT NULL DEFAULT '{}'::jsonb,
    conditions      JSONB        NOT NULL DEFAULT '[]'::jsonb,
    action_type     TEXT         NOT NULL
                                 CHECK (action_type IN ('SEND_MESSAGE', 'AI_GENERATE_MESSAGE')),
    action_config   JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_by      UUID         REFERENCES users (id) ON DELETE RESTRICT,
    updated_by      UUID         REFERENCES users (id) ON DELETE RESTRICT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_automations_id_business UNIQUE (id, business_id)
);

CREATE INDEX idx_automations_business_id_status
    ON automations (business_id, status, updated_at DESC);

CREATE INDEX idx_automations_business_id_trigger
    ON automations (business_id, trigger_type, status);

CREATE TRIGGER trg_automations_updated_at
    BEFORE UPDATE ON automations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE automation_events (
    id               UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id      UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    event_type       TEXT         NOT NULL CHECK (btrim(event_type) <> ''),
    subject_type     TEXT         CHECK (subject_type IS NULL OR btrim(subject_type) <> ''),
    subject_id       UUID,
    idempotency_key  TEXT         NOT NULL CHECK (btrim(idempotency_key) <> ''),
    payload          JSONB        NOT NULL DEFAULT '{}'::jsonb,
    occurred_at      TIMESTAMPTZ  NOT NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_automation_events_id_business UNIQUE (id, business_id),
    CONSTRAINT uq_automation_events_idempotency UNIQUE (business_id, event_type, idempotency_key)
);

CREATE INDEX idx_automation_events_business_id_occurred_at
    ON automation_events (business_id, occurred_at DESC);

CREATE TABLE automation_runs (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id     UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    automation_id   UUID         NOT NULL,
    event_id        UUID,
    execution_key   TEXT         CHECK (execution_key IS NULL OR btrim(execution_key) <> ''),
    subject_type    TEXT         CHECK (subject_type IS NULL OR btrim(subject_type) <> ''),
    subject_id      UUID,
    status          TEXT         NOT NULL DEFAULT 'PENDING'
                                 CHECK (status IN ('PENDING', 'RUNNING', 'COMPLETED', 'FAILED', 'CANCELLED')),
    scheduled_at    TIMESTAMPTZ,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    failed_at       TIMESTAMPTZ,
    attempt         INTEGER      NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    error_code      TEXT         CHECK (error_code IS NULL OR btrim(error_code) <> ''),
    error_message   TEXT         CHECK (error_message IS NULL OR btrim(error_message) <> ''),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_automation_runs_id_business UNIQUE (id, business_id),
    CONSTRAINT fk_automation_runs_automation_business
        FOREIGN KEY (automation_id, business_id)
        REFERENCES automations (id, business_id)
        ON DELETE RESTRICT,
    CONSTRAINT fk_automation_runs_event_business
        FOREIGN KEY (event_id, business_id)
        REFERENCES automation_events (id, business_id)
        ON DELETE RESTRICT
);

CREATE UNIQUE INDEX uq_automation_runs_event
    ON automation_runs (automation_id, event_id)
    WHERE event_id IS NOT NULL;

CREATE UNIQUE INDEX uq_automation_runs_execution_key
    ON automation_runs (automation_id, execution_key)
    WHERE execution_key IS NOT NULL;

CREATE INDEX idx_automation_runs_automation_id_created_at
    ON automation_runs (automation_id, created_at DESC);

CREATE INDEX idx_automation_runs_business_id_status
    ON automation_runs (business_id, status, created_at DESC);

CREATE TRIGGER trg_automation_runs_updated_at
    BEFORE UPDATE ON automation_runs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO permissions (code, resource, action, description) VALUES
    ('automation.read',   'automation', 'read',   'Read automations and run history'),
    ('automation.create', 'automation', 'create', 'Create automations'),
    ('automation.update', 'automation', 'update', 'Update, activate, or deactivate automations'),
    ('automation.delete', 'automation', 'delete', 'Archive automations')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name = 'OWNER' AND p.code LIKE 'automation.%'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN ('automation.read', 'automation.create', 'automation.update', 'automation.delete')
WHERE r.name = 'MANAGER'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'automation.read'
WHERE r.name = 'STAFF'
ON CONFLICT DO NOTHING;
