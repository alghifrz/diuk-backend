DROP TABLE IF EXISTS business_users;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS businesses;

DROP FUNCTION IF EXISTS set_updated_at();

-- The pgcrypto extension is intentionally left in place: other schemas in the
-- same database may depend on it.
