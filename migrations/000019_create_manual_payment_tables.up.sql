-- JOB 20: manual reservation deposits and payment records.
-- 000018 already added the PLUS plan; this job is version 19.

ALTER TABLE fnb_settings
    ADD COLUMN deposit_enabled BOOLEAN        NOT NULL DEFAULT FALSE,
    ADD COLUMN deposit_type    VARCHAR(20)    NOT NULL DEFAULT 'FIXED',
    ADD COLUMN deposit_value   NUMERIC(12, 2) NOT NULL DEFAULT 0;

ALTER TABLE fnb_settings
    ADD CONSTRAINT chk_fnb_settings_deposit_type
        CHECK (deposit_type IN ('FIXED', 'PERCENTAGE'));

ALTER TABLE fnb_settings
    ADD CONSTRAINT chk_fnb_settings_deposit_value
        CHECK (
            deposit_value >= 0
            AND (
                deposit_type <> 'PERCENTAGE'
                OR deposit_value <= 100
            )
        );

ALTER TABLE reservations
    ADD CONSTRAINT uq_reservations_id_business UNIQUE (id, business_id);

CREATE TABLE reservation_payments (
    id             UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id    UUID           NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    reservation_id UUID           NOT NULL,
    status         VARCHAR(20)    NOT NULL CHECK (status IN ('UNPAID', 'PAID', 'REFUNDED')),
    amount         NUMERIC(12, 2) NOT NULL CHECK (amount >= 0),
    currency       VARCHAR(3)     NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    notes          TEXT           CHECK (notes IS NULL OR btrim(notes) <> ''),
    confirmed_at   TIMESTAMPTZ,
    confirmed_by   UUID           REFERENCES users (id) ON DELETE RESTRICT,
    refunded_at    TIMESTAMPTZ,
    refunded_by    UUID           REFERENCES users (id) ON DELETE RESTRICT,
    created_at     TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ    NOT NULL DEFAULT now(),
    CONSTRAINT uq_reservation_payments_business_reservation UNIQUE (business_id, reservation_id),
    CONSTRAINT fk_reservation_payments_reservation_business
        FOREIGN KEY (reservation_id, business_id)
        REFERENCES reservations (id, business_id)
        ON DELETE RESTRICT,
    CONSTRAINT chk_reservation_payments_paid_audit CHECK (
        (status = 'UNPAID' AND confirmed_at IS NULL AND confirmed_by IS NULL AND refunded_at IS NULL AND refunded_by IS NULL)
        OR (status = 'PAID' AND confirmed_at IS NOT NULL AND confirmed_by IS NOT NULL AND refunded_at IS NULL AND refunded_by IS NULL)
        OR (status = 'REFUNDED' AND refunded_at IS NOT NULL AND refunded_by IS NOT NULL)
    )
);

CREATE INDEX idx_reservation_payments_business_id
    ON reservation_payments (business_id);

CREATE INDEX idx_reservation_payments_business_id_status
    ON reservation_payments (business_id, status);

CREATE INDEX idx_reservation_payments_business_id_created_at
    ON reservation_payments (business_id, created_at DESC);

CREATE TRIGGER trg_reservation_payments_updated_at
    BEFORE UPDATE ON reservation_payments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO reservation_payments (business_id, reservation_id, status, amount, currency)
SELECT r.business_id, r.id, 'UNPAID', 0, COALESCE(f.currency, 'IDR')
FROM reservations r
LEFT JOIN fnb_settings f ON f.business_id = r.business_id
ON CONFLICT (business_id, reservation_id) DO NOTHING;

INSERT INTO permissions (code, resource, action, description) VALUES
    ('payment.read',   'payment', 'read',   'Read reservation payments'),
    ('payment.update', 'payment', 'update', 'Mark reservation payments paid or refunded')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name = 'OWNER' AND p.code LIKE 'payment.%'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN ('payment.read', 'payment.update')
WHERE r.name = 'MANAGER'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'payment.read'
WHERE r.name = 'STAFF'
ON CONFLICT DO NOTHING;
