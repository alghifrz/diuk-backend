-- JOB 19: plans, entitlements, tenant usage periods, atomic counters,
-- and source-level consumption idempotency. Plan limits live in PostgreSQL.

CREATE TABLE plans (
    id          UUID         PRIMARY KEY,
    code        TEXT         NOT NULL UNIQUE CHECK (code IN ('FREE', 'STANDARD', 'PRO', 'BUSINESS')),
    name        TEXT         NOT NULL CHECK (btrim(name) <> ''),
    description TEXT         CHECK (description IS NULL OR btrim(description) <> ''),
    status      TEXT         NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_plans_updated_at
    BEFORE UPDATE ON plans
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE plan_entitlements (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id     UUID         NOT NULL REFERENCES plans (id) ON DELETE RESTRICT,
    feature_key TEXT         NOT NULL CHECK (feature_key IN (
        'AI_RESPONSES',
        'CAMPAIGN_RECIPIENTS',
        'AUTOMATION_RUNS',
        'KNOWLEDGE_DOCUMENTS',
        'WHATSAPP_CHANNELS'
    )),
    limit_value INTEGER      CHECK (limit_value IS NULL OR limit_value >= 0),
    period      TEXT         NOT NULL CHECK (period IN ('MONTHLY', 'NONE')),
    enabled     BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_plan_entitlements_plan_feature UNIQUE (plan_id, feature_key)
);

CREATE INDEX idx_plan_entitlements_plan_id
    ON plan_entitlements (plan_id, feature_key);

CREATE TRIGGER trg_plan_entitlements_updated_at
    BEFORE UPDATE ON plan_entitlements
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO plans (id, code, name, description, status) VALUES
    ('00000000-0000-4000-8000-000000000001', 'FREE',     'Free',     'Starter plan with monthly usage caps', 'ACTIVE'),
    ('00000000-0000-4000-8000-000000000002', 'STANDARD', 'Standard', 'Default plan for new businesses',      'ACTIVE'),
    ('00000000-0000-4000-8000-000000000003', 'PRO',      'Pro',      'Higher monthly usage caps',            'ACTIVE'),
    ('00000000-0000-4000-8000-000000000004', 'BUSINESS', 'Business', 'Unlimited usage; custom resources',    'ACTIVE');

-- Seed limits are configuration, not application constants.
-- NULL limit_value means unlimited (or custom for resource features).
INSERT INTO plan_entitlements (plan_id, feature_key, limit_value, period, enabled) VALUES
    ('00000000-0000-4000-8000-000000000001', 'AI_RESPONSES',        1000,    'MONTHLY', TRUE),
    ('00000000-0000-4000-8000-000000000001', 'CAMPAIGN_RECIPIENTS', 500,     'MONTHLY', TRUE),
    ('00000000-0000-4000-8000-000000000001', 'AUTOMATION_RUNS',     100,     'MONTHLY', TRUE),
    ('00000000-0000-4000-8000-000000000001', 'KNOWLEDGE_DOCUMENTS', 10,      'NONE',    TRUE),
    ('00000000-0000-4000-8000-000000000001', 'WHATSAPP_CHANNELS',   1,       'NONE',    TRUE),

    ('00000000-0000-4000-8000-000000000002', 'AI_RESPONSES',        50000,   'MONTHLY', TRUE),
    ('00000000-0000-4000-8000-000000000002', 'CAMPAIGN_RECIPIENTS', 100000,  'MONTHLY', TRUE),
    ('00000000-0000-4000-8000-000000000002', 'AUTOMATION_RUNS',     10000,   'MONTHLY', TRUE),
    ('00000000-0000-4000-8000-000000000002', 'KNOWLEDGE_DOCUMENTS', 100,     'NONE',    TRUE),
    ('00000000-0000-4000-8000-000000000002', 'WHATSAPP_CHANNELS',   3,       'NONE',    TRUE),

    ('00000000-0000-4000-8000-000000000003', 'AI_RESPONSES',        150000,  'MONTHLY', TRUE),
    ('00000000-0000-4000-8000-000000000003', 'CAMPAIGN_RECIPIENTS', 500000,  'MONTHLY', TRUE),
    ('00000000-0000-4000-8000-000000000003', 'AUTOMATION_RUNS',     50000,   'MONTHLY', TRUE),
    ('00000000-0000-4000-8000-000000000003', 'KNOWLEDGE_DOCUMENTS', 500,     'NONE',    TRUE),
    ('00000000-0000-4000-8000-000000000003', 'WHATSAPP_CHANNELS',   10,      'NONE',    TRUE),

    ('00000000-0000-4000-8000-000000000004', 'AI_RESPONSES',        NULL,    'MONTHLY', TRUE),
    ('00000000-0000-4000-8000-000000000004', 'CAMPAIGN_RECIPIENTS', NULL,    'MONTHLY', TRUE),
    ('00000000-0000-4000-8000-000000000004', 'AUTOMATION_RUNS',     NULL,    'MONTHLY', TRUE),
    ('00000000-0000-4000-8000-000000000004', 'KNOWLEDGE_DOCUMENTS', NULL,    'NONE',    TRUE),
    ('00000000-0000-4000-8000-000000000004', 'WHATSAPP_CHANNELS',   NULL,    'NONE',    TRUE);

ALTER TABLE businesses
    ADD COLUMN plan_id UUID REFERENCES plans (id) ON DELETE RESTRICT;

UPDATE businesses
SET plan_id = '00000000-0000-4000-8000-000000000002'
WHERE plan_id IS NULL;

ALTER TABLE businesses
    ALTER COLUMN plan_id SET DEFAULT '00000000-0000-4000-8000-000000000002';

ALTER TABLE businesses
    ALTER COLUMN plan_id SET NOT NULL;

CREATE INDEX idx_businesses_plan_id ON businesses (plan_id);

CREATE TABLE usage_periods (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id  UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    period_type  TEXT         NOT NULL CHECK (period_type IN ('MONTHLY')),
    period_start TIMESTAMPTZ  NOT NULL,
    period_end   TIMESTAMPTZ  NOT NULL,
    timezone     TEXT         NOT NULL CHECK (btrim(timezone) <> ''),
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_usage_periods_business_type_start UNIQUE (business_id, period_type, period_start),
    CONSTRAINT chk_usage_periods_half_open CHECK (period_end > period_start)
);

CREATE INDEX idx_usage_periods_business_start
    ON usage_periods (business_id, period_start DESC);

CREATE TRIGGER trg_usage_periods_updated_at
    BEFORE UPDATE ON usage_periods
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE usage_counters (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id     UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    usage_period_id UUID         NOT NULL REFERENCES usage_periods (id) ON DELETE RESTRICT,
    feature_key     TEXT         NOT NULL CHECK (feature_key IN (
        'AI_RESPONSES',
        'CAMPAIGN_RECIPIENTS',
        'AUTOMATION_RUNS'
    )),
    used            BIGINT       NOT NULL DEFAULT 0 CHECK (used >= 0),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_usage_counters_period_feature UNIQUE (usage_period_id, feature_key)
);

CREATE INDEX idx_usage_counters_business_feature
    ON usage_counters (business_id, feature_key);

CREATE TRIGGER trg_usage_counters_updated_at
    BEFORE UPDATE ON usage_counters
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE usage_consumptions (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id     UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    usage_period_id UUID         NOT NULL REFERENCES usage_periods (id) ON DELETE RESTRICT,
    feature_key     TEXT         NOT NULL CHECK (feature_key IN (
        'AI_RESPONSES',
        'CAMPAIGN_RECIPIENTS',
        'AUTOMATION_RUNS'
    )),
    source_type     TEXT         NOT NULL CHECK (source_type IN (
        'AI_GENERATION',
        'CAMPAIGN_RECIPIENT',
        'AUTOMATION_RUN'
    )),
    source_id       UUID         NOT NULL,
    quantity        INTEGER      NOT NULL CHECK (quantity > 0),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_usage_consumptions_source UNIQUE (business_id, feature_key, source_type, source_id)
);

CREATE INDEX idx_usage_consumptions_period_feature
    ON usage_consumptions (usage_period_id, feature_key);

INSERT INTO permissions (code, resource, action, description) VALUES
    ('usage.read', 'usage', 'read', 'Read current plan usage and history')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name IN ('OWNER', 'MANAGER', 'STAFF') AND p.code = 'usage.read'
ON CONFLICT DO NOTHING;
