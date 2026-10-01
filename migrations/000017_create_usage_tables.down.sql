DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE code = 'usage.read');

DELETE FROM permissions WHERE code = 'usage.read';

DROP TABLE IF EXISTS usage_consumptions;
DROP TABLE IF EXISTS usage_counters;
DROP TABLE IF EXISTS usage_periods;

ALTER TABLE businesses DROP COLUMN IF EXISTS plan_id;

DROP TABLE IF EXISTS plan_entitlements;
DROP TABLE IF EXISTS plans;
