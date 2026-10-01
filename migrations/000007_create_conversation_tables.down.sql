DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS conversations;

ALTER TABLE customer_identities
    DROP CONSTRAINT IF EXISTS uq_customer_identities_id_business_customer;
