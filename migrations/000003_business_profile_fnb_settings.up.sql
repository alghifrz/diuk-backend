-- Business identity fields plus one-to-one F&B operational settings.
-- Opening hours, holidays and reservation capacity are later jobs.

ALTER TABLE businesses
    ADD COLUMN description TEXT CHECK (description IS NULL OR btrim(description) <> ''),
    ADD COLUMN phone       TEXT CHECK (phone IS NULL OR btrim(phone) <> ''),
    ADD COLUMN email       TEXT CHECK (email IS NULL OR (email = lower(email) AND email LIKE '%_@_%')),
    ADD COLUMN address     TEXT CHECK (address IS NULL OR btrim(address) <> ''),
    ADD COLUMN website_url TEXT CHECK (website_url IS NULL OR btrim(website_url) <> ''),
    ADD COLUMN logo_url    TEXT CHECK (logo_url IS NULL OR btrim(logo_url) <> '');

CREATE TABLE fnb_settings (
    business_id                         UUID        PRIMARY KEY REFERENCES businesses (id) ON DELETE RESTRICT,
    currency                            VARCHAR(3)  NOT NULL DEFAULT 'IDR' CHECK (currency ~ '^[A-Z]{3}$'),
    reservation_enabled                 BOOLEAN     NOT NULL DEFAULT TRUE,
    reservation_slot_duration_minutes   INTEGER     NOT NULL DEFAULT 30 CHECK (reservation_slot_duration_minutes > 0),
    reservation_min_party_size          INTEGER     NOT NULL DEFAULT 1 CHECK (reservation_min_party_size >= 1),
    reservation_max_party_size          INTEGER     NOT NULL DEFAULT 20,
    reservation_min_advance_minutes     INTEGER     NOT NULL DEFAULT 60 CHECK (reservation_min_advance_minutes >= 0),
    reservation_max_advance_days        INTEGER     NOT NULL DEFAULT 30 CHECK (reservation_max_advance_days >= 1),
    created_at                          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_fnb_settings_party_size CHECK (reservation_max_party_size >= reservation_min_party_size)
);

CREATE TRIGGER trg_fnb_settings_updated_at
    BEFORE UPDATE ON fnb_settings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- fnb_settings may only belong to an FNB tenant.
CREATE OR REPLACE FUNCTION enforce_fnb_settings_business_type()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM businesses
        WHERE id = NEW.business_id AND business_type = 'FNB'
    ) THEN
        RAISE EXCEPTION 'fnb_settings require business_type FNB';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_fnb_settings_business_type
    BEFORE INSERT OR UPDATE ON fnb_settings
    FOR EACH ROW EXECUTE FUNCTION enforce_fnb_settings_business_type();

-- New FNB businesses get default operational settings automatically.
CREATE OR REPLACE FUNCTION create_default_fnb_settings()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.business_type = 'FNB' THEN
        INSERT INTO fnb_settings (business_id) VALUES (NEW.id)
        ON CONFLICT (business_id) DO NOTHING;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_businesses_default_fnb_settings
    AFTER INSERT ON businesses
    FOR EACH ROW EXECUTE FUNCTION create_default_fnb_settings();

-- Existing FNB tenants, including Kasiko Coffee, receive one default row.
INSERT INTO fnb_settings (business_id)
SELECT id
FROM businesses
WHERE business_type = 'FNB'
ON CONFLICT (business_id) DO NOTHING;
