DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE code LIKE 'automation.%');

DELETE FROM permissions WHERE code LIKE 'automation.%';

DROP TABLE IF EXISTS automation_runs;
DROP TABLE IF EXISTS automation_events;
DROP TABLE IF EXISTS automations;

ALTER TABLE job_outbox DROP CONSTRAINT IF EXISTS job_outbox_job_type_check;
ALTER TABLE job_outbox ADD CONSTRAINT job_outbox_job_type_check
    CHECK (job_type IN ('AI_GENERATE', 'MESSAGE_DELIVERY', 'KNOWLEDGE_INGEST'));
