-- Reservation engine, weekly hours, and schedule exceptions.
-- btree_gist lets EXCLUDE combine equality (table_id) with a tstzrange overlap.
-- Inbox/list queries use (business_id, start_at) because the primary access
-- pattern is "what is booked for this tenant around this time".

CREATE EXTENSION IF NOT EXISTS btree_gist;

ALTER TABLE areas
    ADD CONSTRAINT uq_areas_id_business_id UNIQUE (id, business_id);

ALTER TABLE tables
    ADD CONSTRAINT uq_tables_id_area_id UNIQUE (id, area_id);

CREATE TABLE business_hours (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id  UUID        NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    day_of_week  INTEGER     NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    sort_order   INTEGER     NOT NULL CHECK (sort_order >= 0),
    open_time    TIME        NOT NULL,
    close_time   TIME        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_business_hours_open_before_close CHECK (open_time < close_time),
    CONSTRAINT uq_business_hours_business_day_sort UNIQUE (business_id, day_of_week, sort_order)
);

CREATE INDEX idx_business_hours_business_id_day_of_week
    ON business_hours (business_id, day_of_week);

CREATE TRIGGER trg_business_hours_updated_at
    BEFORE UPDATE ON business_hours
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE business_schedule_exceptions (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id     UUID        NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    exception_date  DATE        NOT NULL,
    is_closed       BOOLEAN     NOT NULL,
    open_time       TIME,
    close_time      TIME,
    note            TEXT        CHECK (note IS NULL OR btrim(note) <> ''),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_schedule_exceptions_business_date UNIQUE (business_id, exception_date),
    CONSTRAINT chk_schedule_exceptions_hours CHECK (
        (is_closed AND open_time IS NULL AND close_time IS NULL)
        OR (
            NOT is_closed
            AND open_time IS NOT NULL
            AND close_time IS NOT NULL
            AND open_time < close_time
        )
    )
);

CREATE INDEX idx_schedule_exceptions_business_id_exception_date
    ON business_schedule_exceptions (business_id, exception_date);

CREATE TRIGGER trg_schedule_exceptions_updated_at
    BEFORE UPDATE ON business_schedule_exceptions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE reservations (
    id                   UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id          UUID         NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    customer_id          UUID         NOT NULL,
    table_id             UUID         REFERENCES tables (id) ON DELETE RESTRICT,
    party_size           INTEGER      NOT NULL CHECK (party_size >= 1),
    start_at             TIMESTAMPTZ  NOT NULL,
    end_at               TIMESTAMPTZ  NOT NULL,
    status               VARCHAR(20)  NOT NULL CHECK (status IN ('PENDING', 'CONFIRMED', 'CANCELLED', 'COMPLETED')),
    notes                TEXT         CHECK (notes IS NULL OR btrim(notes) <> ''),
    cancellation_reason  TEXT         CHECK (cancellation_reason IS NULL OR btrim(cancellation_reason) <> ''),
    confirmed_at         TIMESTAMPTZ,
    completed_at         TIMESTAMPTZ,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT chk_reservations_time_range CHECK (start_at < end_at),
    CONSTRAINT chk_reservations_confirmed_has_table CHECK (status <> 'CONFIRMED' OR table_id IS NOT NULL),
    CONSTRAINT fk_reservations_customer_business
        FOREIGN KEY (customer_id, business_id)
        REFERENCES customers (id, business_id)
        ON DELETE RESTRICT,
    CONSTRAINT excl_reservations_table_overlap
        EXCLUDE USING gist (
            table_id WITH =,
            tstzrange(start_at, end_at, '[)') WITH &&
        )
        WHERE (table_id IS NOT NULL AND status IN ('PENDING', 'CONFIRMED'))
);

-- Inbox: tenant bookings ordered by start time.
CREATE INDEX idx_reservations_business_id_start_at
    ON reservations (business_id, start_at);
-- Filter by lifecycle then time.
CREATE INDEX idx_reservations_business_id_status_start_at
    ON reservations (business_id, status, start_at);
CREATE INDEX idx_reservations_business_id_customer_id_start_at
    ON reservations (business_id, customer_id, start_at);
CREATE INDEX idx_reservations_business_id_table_id_start_at
    ON reservations (business_id, table_id, start_at);
CREATE INDEX idx_reservations_business_id_created_at
    ON reservations (business_id, created_at);

CREATE TRIGGER trg_reservations_updated_at
    BEFORE UPDATE ON reservations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
