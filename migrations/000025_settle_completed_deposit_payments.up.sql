-- A completed reservation is always settled in full. Older rows were completed
-- while only the deposit had been recorded; promote them to a full payment
-- for the whole menu order so reporting and CRM spend stay consistent.

UPDATE reservation_payments p
SET status = 'PAID',
    payment_type = 'FULL',
    amount = t.total
FROM reservations r,
     (
         SELECT ri.reservation_id,
                ri.business_id,
                SUM(ri.unit_price * ri.quantity) AS total
         FROM reservation_items ri
         GROUP BY ri.reservation_id, ri.business_id
     ) t
WHERE r.id = p.reservation_id
  AND r.business_id = p.business_id
  AND t.reservation_id = p.reservation_id
  AND t.business_id = p.business_id
  AND r.status = 'COMPLETED'
  AND p.status = 'DEPOSIT'
  AND t.total > 0;
