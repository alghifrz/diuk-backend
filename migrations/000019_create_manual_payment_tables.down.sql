DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE code LIKE 'payment.%');

DELETE FROM permissions WHERE code LIKE 'payment.%';

DROP TABLE IF EXISTS reservation_payments;

ALTER TABLE reservations DROP CONSTRAINT IF EXISTS uq_reservations_id_business;

ALTER TABLE fnb_settings DROP CONSTRAINT IF EXISTS chk_fnb_settings_deposit_value;
ALTER TABLE fnb_settings DROP CONSTRAINT IF EXISTS chk_fnb_settings_deposit_type;
ALTER TABLE fnb_settings
    DROP COLUMN IF EXISTS deposit_enabled,
    DROP COLUMN IF EXISTS deposit_type,
    DROP COLUMN IF EXISTS deposit_value;
