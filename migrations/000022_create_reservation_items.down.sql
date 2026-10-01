DROP TRIGGER IF EXISTS trg_reservation_items_updated_at ON reservation_items;
DROP TABLE IF EXISTS reservation_items;
ALTER TABLE menu_items DROP CONSTRAINT IF EXISTS uq_menu_items_id_business;
