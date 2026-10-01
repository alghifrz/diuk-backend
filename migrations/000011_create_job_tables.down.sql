DROP TABLE IF EXISTS ai_generations;
DROP TABLE IF EXISTS job_outbox;

UPDATE messages SET status = 'QUEUED' WHERE status = 'SENDING';

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_status_check;
ALTER TABLE messages ADD CONSTRAINT messages_status_check
    CHECK (status IN ('RECEIVED', 'QUEUED', 'SENT', 'DELIVERED', 'READ', 'FAILED'));
