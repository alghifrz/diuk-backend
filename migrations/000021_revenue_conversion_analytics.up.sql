-- JOB 22: targeted indexes for reservation/payment aggregation.
-- No analytics_events table. Metrics stay descriptive aggregations
-- over reservations and reservation_payments.

CREATE INDEX idx_reservations_business_id_created_at_status
    ON reservations (business_id, created_at, status);

CREATE INDEX idx_reservation_payments_business_id_created_at_status
    ON reservation_payments (business_id, created_at, status);
