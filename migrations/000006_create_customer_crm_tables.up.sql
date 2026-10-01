-- Customer profiles, channel identities, and CRM tags. Conversation and
-- reservation modules will reference these rows later; deletions are soft.

CREATE TABLE customers (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    name        VARCHAR(150) NOT NULL CHECK (btrim(name) <> ''),
    phone       VARCHAR(30)  CHECK (phone IS NULL OR btrim(phone) <> ''),
    email       VARCHAR(254) CHECK (email IS NULL OR (email = lower(email) AND email LIKE '%_@_%')),
    notes       TEXT         CHECK (notes IS NULL OR btrim(notes) <> ''),
    status      VARCHAR(20)  NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_customers_id_business_id UNIQUE (id, business_id)
);

CREATE INDEX idx_customers_business_id_status ON customers (business_id, status);
CREATE INDEX idx_customers_business_id_name ON customers (business_id, name);
CREATE INDEX idx_customers_business_id_phone ON customers (business_id, phone) WHERE phone IS NOT NULL;

CREATE TRIGGER trg_customers_updated_at
    BEFORE UPDATE ON customers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE customer_identities (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id  UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    customer_id  UUID         NOT NULL,
    channel      VARCHAR(30)  NOT NULL CHECK (channel IN ('WHATSAPP', 'INSTAGRAM', 'WEBSITE')),
    external_id  VARCHAR(255) NOT NULL CHECK (btrim(external_id) <> ''),
    display_name VARCHAR(150) CHECK (display_name IS NULL OR btrim(display_name) <> ''),
    status       VARCHAR(20)  NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT fk_customer_identities_customer_business
        FOREIGN KEY (customer_id, business_id)
        REFERENCES customers (id, business_id)
        ON DELETE RESTRICT,
    CONSTRAINT uq_customer_identities_business_channel_external
        UNIQUE (business_id, channel, external_id)
);

CREATE INDEX idx_customer_identities_business_id_customer_id ON customer_identities (business_id, customer_id);
CREATE INDEX idx_customer_identities_business_id_status ON customer_identities (business_id, status);

CREATE TRIGGER trg_customer_identities_updated_at
    BEFORE UPDATE ON customer_identities
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE tags (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    name        VARCHAR(50)  NOT NULL CHECK (btrim(name) <> ''),
    description VARCHAR(255) CHECK (description IS NULL OR btrim(description) <> ''),
    status      VARCHAR(20)  NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_tags_business_id_name UNIQUE (business_id, name),
    CONSTRAINT uq_tags_id_business_id UNIQUE (id, business_id)
);

CREATE INDEX idx_tags_business_id_status ON tags (business_id, status);

CREATE TRIGGER trg_tags_updated_at
    BEFORE UPDATE ON tags
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- business_id is required so both composite FKs share one tenant.
CREATE TABLE customer_tags (
    customer_id UUID        NOT NULL,
    tag_id      UUID        NOT NULL,
    business_id UUID        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (customer_id, tag_id),
    CONSTRAINT fk_customer_tags_customer_business
        FOREIGN KEY (customer_id, business_id)
        REFERENCES customers (id, business_id)
        ON DELETE RESTRICT,
    CONSTRAINT fk_customer_tags_tag_business
        FOREIGN KEY (tag_id, business_id)
        REFERENCES tags (id, business_id)
        ON DELETE RESTRICT
);

CREATE INDEX idx_customer_tags_tag_id ON customer_tags (tag_id);
CREATE INDEX idx_customer_tags_business_id_tag_id ON customer_tags (business_id, tag_id);
