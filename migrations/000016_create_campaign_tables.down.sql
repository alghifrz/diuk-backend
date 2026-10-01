DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE code LIKE 'campaign.%');

DELETE FROM permissions WHERE code LIKE 'campaign.%';

DROP TABLE IF EXISTS campaign_recipients;
DROP TABLE IF EXISTS campaign_runs;
DROP TABLE IF EXISTS campaigns;

ALTER TABLE customers DROP COLUMN IF EXISTS marketing_opt_in;

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_sender_type_check;
ALTER TABLE messages ADD CONSTRAINT messages_sender_type_check
    CHECK (sender_type IN ('CUSTOMER', 'USER', 'SYSTEM', 'AI'));

ALTER TABLE job_outbox DROP CONSTRAINT IF EXISTS job_outbox_job_type_check;
ALTER TABLE job_outbox ADD CONSTRAINT job_outbox_job_type_check
    CHECK (job_type IN (
        'AI_GENERATE',
        'MESSAGE_DELIVERY',
        'KNOWLEDGE_INGEST',
        'AUTOMATION_EVALUATE',
        'AUTOMATION_RUN'
    ));
