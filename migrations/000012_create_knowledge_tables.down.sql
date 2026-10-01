DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE code LIKE 'knowledge.%');

DELETE FROM permissions WHERE code LIKE 'knowledge.%';

DROP TABLE IF EXISTS knowledge_chunks;
DROP TABLE IF EXISTS knowledge_documents;

ALTER TABLE job_outbox DROP CONSTRAINT IF EXISTS job_outbox_job_type_check;
ALTER TABLE job_outbox ADD CONSTRAINT job_outbox_job_type_check
    CHECK (job_type IN ('AI_GENERATE', 'MESSAGE_DELIVERY'));

DROP EXTENSION IF EXISTS vector;
