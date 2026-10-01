DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE code LIKE 'channel.%');

DELETE FROM permissions WHERE code LIKE 'channel.%';

DROP INDEX IF EXISTS uq_messages_business_external_id;

DROP TABLE IF EXISTS business_channels;
