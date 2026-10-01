-- JOB 21: calendar blocks around the existing reservation schedule.
-- Hours and exceptions stay authoritative; blocks only withhold bookable capacity.

CREATE TABLE calendar_blocks (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    area_id     UUID,
    table_id    UUID,
    title       VARCHAR(200) NOT NULL CHECK (btrim(title) <> ''),
    description TEXT         CHECK (description IS NULL OR btrim(description) <> ''),
    start_at    TIMESTAMPTZ  NOT NULL,
    end_at      TIMESTAMPTZ  NOT NULL,
    status      VARCHAR(20)  NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'CANCELLED')),
    created_by  UUID         NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT chk_calendar_blocks_time_range CHECK (start_at < end_at),
    CONSTRAINT chk_calendar_blocks_scope CHECK (
        (area_id IS NULL AND table_id IS NULL)
        OR (area_id IS NOT NULL AND table_id IS NULL)
        OR (area_id IS NOT NULL AND table_id IS NOT NULL)
    ),
    CONSTRAINT fk_calendar_blocks_area_business
        FOREIGN KEY (area_id, business_id)
        REFERENCES areas (id, business_id)
        ON DELETE RESTRICT,
    CONSTRAINT fk_calendar_blocks_table_area
        FOREIGN KEY (table_id, area_id)
        REFERENCES tables (id, area_id)
        ON DELETE RESTRICT,
    CONSTRAINT excl_calendar_blocks_business_overlap
        EXCLUDE USING gist (
            business_id WITH =,
            tstzrange(start_at, end_at, '[)') WITH &&
        )
        WHERE (status = 'ACTIVE' AND area_id IS NULL AND table_id IS NULL),
    CONSTRAINT excl_calendar_blocks_area_overlap
        EXCLUDE USING gist (
            area_id WITH =,
            tstzrange(start_at, end_at, '[)') WITH &&
        )
        WHERE (status = 'ACTIVE' AND area_id IS NOT NULL AND table_id IS NULL),
    CONSTRAINT excl_calendar_blocks_table_overlap
        EXCLUDE USING gist (
            table_id WITH =,
            tstzrange(start_at, end_at, '[)') WITH &&
        )
        WHERE (status = 'ACTIVE' AND table_id IS NOT NULL)
);

CREATE INDEX idx_calendar_blocks_business_id_start_end
    ON calendar_blocks (business_id, start_at, end_at);

CREATE INDEX idx_calendar_blocks_business_id_status_start
    ON calendar_blocks (business_id, status, start_at);

CREATE INDEX idx_calendar_blocks_area_id_start_end
    ON calendar_blocks (area_id, start_at, end_at)
    WHERE area_id IS NOT NULL;

CREATE INDEX idx_calendar_blocks_table_id_start_end
    ON calendar_blocks (table_id, start_at, end_at)
    WHERE table_id IS NOT NULL;

CREATE TRIGGER trg_calendar_blocks_updated_at
    BEFORE UPDATE ON calendar_blocks
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO permissions (code, resource, action, description) VALUES
    ('calendar.read',   'calendar', 'read',   'Read calendar events and schedule blocks'),
    ('calendar.update', 'calendar', 'update', 'Create, update, or cancel calendar blocks')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name = 'OWNER' AND p.code LIKE 'calendar.%'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN ('calendar.read', 'calendar.update')
WHERE r.name = 'MANAGER'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'calendar.read'
WHERE r.name = 'STAFF'
ON CONFLICT DO NOTHING;
