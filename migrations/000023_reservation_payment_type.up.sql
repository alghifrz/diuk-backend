-- Menu-aligned payments: payment_type + DEPOSIT status.

ALTER TABLE reservation_payments
    DROP CONSTRAINT IF EXISTS chk_reservation_payments_paid_audit;

ALTER TABLE reservation_payments
    DROP CONSTRAINT IF EXISTS reservation_payments_status_check;

ALTER TABLE reservation_payments
    ADD COLUMN IF NOT EXISTS payment_type VARCHAR(20) NOT NULL DEFAULT 'NONE';

UPDATE reservation_payments
SET payment_type = 'NONE'
WHERE payment_type IS NULL OR btrim(payment_type) = '';

ALTER TABLE reservation_payments
    DROP CONSTRAINT IF EXISTS chk_reservation_payments_payment_type;

ALTER TABLE reservation_payments
    ADD CONSTRAINT chk_reservation_payments_payment_type
        CHECK (payment_type IN ('NONE', 'FULL', 'DEPOSIT'));

ALTER TABLE reservation_payments
    ADD CONSTRAINT reservation_payments_status_check
        CHECK (status IN ('UNPAID', 'DEPOSIT', 'PAID', 'REFUNDED'));

ALTER TABLE reservation_payments
    ADD CONSTRAINT chk_reservation_payments_paid_audit CHECK (
        (status = 'UNPAID' AND confirmed_at IS NULL AND confirmed_by IS NULL AND refunded_at IS NULL AND refunded_by IS NULL)
        OR (status IN ('DEPOSIT', 'PAID') AND confirmed_at IS NOT NULL AND confirmed_by IS NOT NULL AND refunded_at IS NULL AND refunded_by IS NULL)
        OR (status = 'REFUNDED' AND refunded_at IS NOT NULL AND refunded_by IS NOT NULL)
    );
