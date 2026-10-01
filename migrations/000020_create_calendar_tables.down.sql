DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE code LIKE 'calendar.%');

DELETE FROM permissions WHERE code LIKE 'calendar.%';

DROP TABLE IF EXISTS calendar_blocks;
