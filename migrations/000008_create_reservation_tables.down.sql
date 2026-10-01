DROP TABLE IF EXISTS reservations;
DROP TABLE IF EXISTS business_schedule_exceptions;
DROP TABLE IF EXISTS business_hours;

ALTER TABLE tables
    DROP CONSTRAINT IF EXISTS uq_tables_id_area_id;
ALTER TABLE areas
    DROP CONSTRAINT IF EXISTS uq_areas_id_business_id;
