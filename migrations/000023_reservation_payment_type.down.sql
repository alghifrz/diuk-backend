ALTER TABLE reservation_payments
    DROP CONSTRAINT IF EXISTS chk_reservation_payments_paid_audit;

ALTER TABLE reservation_payments
    DROP CONSTRAINT IF EXISTS reservation_payments_status_check;

ALTER TABLE reservation_payments
    DROP CONSTRAINT IF EXISTS chk_reservation_payments_payment_type;

ALTER TABLE reservation_payments
    DROP COLUMN IF EXISTS payment_type;

ALTER TABLE reservation_payments
    ADD CONSTRAINT reservation_payments_status_check
        CHECK (status IN ('UNPAID', 'PAID', 'REFUNDED'));

ALTER TABLE reservation_payments
    ADD CONSTRAINT chk_reservation_payments_paid_audit CHECK (
        (status = 'UNPAID' AND confirmed_at IS NULL AND confirmed_by IS NULL AND refunded_at IS NULL AND refunded_by IS NULL)
        OR (status = 'PAID' AND confirmed_at IS NOT NULL AND confirmed_by IS NOT NULL AND refunded_at IS NULL AND refunded_by IS NULL)
        OR (status = 'REFUNDED' AND refunded_at IS NOT NULL AND refunded_by IS NOT NULL)
    );
