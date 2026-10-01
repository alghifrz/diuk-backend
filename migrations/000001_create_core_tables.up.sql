-- Core multi-tenant tables: businesses (tenants), users (profiles mirroring
-- Supabase Auth), roles and the business_users membership join table.

-- PostgreSQL 13+ ships gen_random_uuid() in core; older servers need pgcrypto.
DO $$
BEGIN
    IF to_regprocedure('gen_random_uuid()') IS NULL THEN
        CREATE EXTENSION IF NOT EXISTS pgcrypto;
    END IF;
END
$$;

-- Keeps updated_at honest regardless of which statement touches the row.
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE businesses (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT        NOT NULL CHECK (btrim(name) <> ''),
    slug          TEXT        NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    business_type TEXT        NOT NULL CHECK (business_type IN ('FNB')),
    timezone      TEXT        NOT NULL DEFAULT 'Asia/Jakarta' CHECK (btrim(timezone) <> ''),
    status        TEXT        NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- businesses.slug is already indexed by its UNIQUE constraint.
CREATE INDEX idx_businesses_status ON businesses (status);

CREATE TRIGGER trg_businesses_updated_at
    BEFORE UPDATE ON businesses
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Application-level profile data. Credentials live in Supabase Auth only.
CREATE TABLE users (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    auth_user_id UUID        NOT NULL UNIQUE,
    full_name    TEXT        NOT NULL CHECK (btrim(full_name) <> ''),
    email        TEXT        NOT NULL UNIQUE CHECK (email = lower(email) AND email LIKE '%_@_%'),
    phone        TEXT        CHECK (phone IS NULL OR btrim(phone) <> ''),
    status       TEXT        NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- users.auth_user_id and users.email are indexed by their UNIQUE constraints.

CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL UNIQUE CHECK (btrim(name) <> ''),
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_roles_updated_at
    BEFORE UPDATE ON roles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO roles (name, description) VALUES
    ('OWNER',   'Owns the business and has full access'),
    ('MANAGER', 'Manages day to day operations of the business'),
    ('STAFF',   'Handles operational tasks within the business')
ON CONFLICT (name) DO NOTHING;

-- Membership of a user in a business. A user can belong to many businesses
-- with a different role in each one.
CREATE TABLE business_users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id UUID        NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    user_id     UUID        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    role_id     UUID        NOT NULL REFERENCES roles (id) ON DELETE RESTRICT,
    status      TEXT        NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_business_users_business_user UNIQUE (business_id, user_id)
);

-- business_users.business_id is covered by the leading column of
-- uq_business_users_business_user, so only the other keys need an index.
CREATE INDEX idx_business_users_user_id ON business_users (user_id);
CREATE INDEX idx_business_users_role_id ON business_users (role_id);

CREATE TRIGGER trg_business_users_updated_at
    BEFORE UPDATE ON business_users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
