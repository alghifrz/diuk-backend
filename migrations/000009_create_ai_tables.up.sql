-- AI configuration and versioned prompts. Provider credentials stay in the
-- environment; these tables only store tenant-visible settings.

CREATE TABLE ai_settings (
    id                 UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id        UUID           NOT NULL UNIQUE REFERENCES businesses (id) ON DELETE RESTRICT,
    provider           VARCHAR(30)    NOT NULL DEFAULT 'openai'
                           CHECK (provider IN ('openai', 'groq', 'anthropic', 'ollama')),
    model              VARCHAR(100)   NOT NULL CHECK (btrim(model) <> ''),
    temperature        NUMERIC(3,2)   CHECK (temperature IS NULL OR (temperature >= 0 AND temperature <= 2)),
    max_output_tokens  INTEGER        CHECK (max_output_tokens IS NULL OR max_output_tokens > 0),
    enabled            BOOLEAN        NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ    NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_ai_settings_updated_at
    BEFORE UPDATE ON ai_settings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE ai_prompts (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id    UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    name           VARCHAR(100) NOT NULL CHECK (btrim(name) <> ''),
    description    TEXT         CHECK (description IS NULL OR btrim(description) <> ''),
    system_prompt  TEXT         NOT NULL CHECK (btrim(system_prompt) <> ''),
    version        INTEGER      NOT NULL CHECK (version >= 1),
    status         VARCHAR(20)  NOT NULL CHECK (status IN ('DRAFT', 'ACTIVE', 'ARCHIVED')),
    created_by     UUID         REFERENCES users (id) ON DELETE RESTRICT,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_ai_prompts_business_name_version UNIQUE (business_id, name, version)
);

-- One production version per logical prompt name.
CREATE UNIQUE INDEX uq_ai_prompts_one_active
    ON ai_prompts (business_id, name)
    WHERE status = 'ACTIVE';

CREATE INDEX idx_ai_prompts_business_id_name_status
    ON ai_prompts (business_id, name, status);

CREATE TRIGGER trg_ai_prompts_updated_at
    BEFORE UPDATE ON ai_prompts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
