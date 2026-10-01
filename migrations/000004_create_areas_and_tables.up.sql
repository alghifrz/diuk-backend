-- Physical seating layout for an F&B tenant. Reservation occupancy is a later
-- job; these rows are configuration only and are soft-deactivated via status.

CREATE TABLE areas (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id UUID        NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    name        VARCHAR(100) NOT NULL CHECK (btrim(name) <> ''),
    description TEXT        CHECK (description IS NULL OR btrim(description) <> ''),
    status      VARCHAR(20) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_areas_business_id_name UNIQUE (business_id, name)
);

CREATE INDEX idx_areas_business_id_status ON areas (business_id, status);

CREATE TRIGGER trg_areas_updated_at
    BEFORE UPDATE ON areas
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE tables (
    id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    area_id    UUID         NOT NULL REFERENCES areas (id) ON DELETE RESTRICT,
    name       VARCHAR(100) NOT NULL CHECK (btrim(name) <> ''),
    capacity   INTEGER      NOT NULL CHECK (capacity >= 1),
    status     VARCHAR(20)  NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_tables_area_id_name UNIQUE (area_id, name)
);

CREATE INDEX idx_tables_area_id_status ON tables (area_id, status);

CREATE TRIGGER trg_tables_updated_at
    BEFORE UPDATE ON tables
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
