-- Menu catalog for an F&B tenant. Images live in private object storage;
-- PostgreSQL stores only the object path. Ordering and inventory are later jobs.

CREATE TABLE menu_categories (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    name        VARCHAR(100) NOT NULL CHECK (btrim(name) <> ''),
    description TEXT         CHECK (description IS NULL OR btrim(description) <> ''),
    status      VARCHAR(20)  NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    sort_order  INTEGER      NOT NULL DEFAULT 0 CHECK (sort_order >= 0),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_menu_categories_business_id_name UNIQUE (business_id, name)
);

CREATE INDEX idx_menu_categories_business_id_status ON menu_categories (business_id, status);
CREATE INDEX idx_menu_categories_business_id_sort_order ON menu_categories (business_id, sort_order);

CREATE TRIGGER trg_menu_categories_updated_at
    BEFORE UPDATE ON menu_categories
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE menu_items (
    id          UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id UUID          NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    category_id UUID          NOT NULL REFERENCES menu_categories (id) ON DELETE RESTRICT,
    name        VARCHAR(150)  NOT NULL CHECK (btrim(name) <> ''),
    description TEXT          CHECK (description IS NULL OR btrim(description) <> ''),
    price       NUMERIC(15,2) NOT NULL CHECK (price >= 0),
    image_path  TEXT          CHECK (image_path IS NULL OR btrim(image_path) <> ''),
    status      VARCHAR(20)   NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    sort_order  INTEGER       NOT NULL DEFAULT 0 CHECK (sort_order >= 0),
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX idx_menu_items_business_id_status ON menu_items (business_id, status);
CREATE INDEX idx_menu_items_category_id_status ON menu_items (category_id, status);
CREATE INDEX idx_menu_items_business_id_sort_order ON menu_items (business_id, sort_order);

CREATE TRIGGER trg_menu_items_updated_at
    BEFORE UPDATE ON menu_items
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
