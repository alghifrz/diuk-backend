-- JOB 22: structured pre-order line items on reservations.

ALTER TABLE menu_items
    ADD CONSTRAINT uq_menu_items_id_business UNIQUE (id, business_id);

CREATE TABLE reservation_items (
    id              UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id     UUID           NOT NULL REFERENCES businesses (id) ON DELETE RESTRICT,
    reservation_id  UUID           NOT NULL,
    menu_item_id    UUID           NOT NULL,
    menu_item_name  VARCHAR(200)   NOT NULL,
    quantity        INT            NOT NULL CHECK (quantity > 0 AND quantity <= 99),
    unit_price      NUMERIC(12, 2) NOT NULL CHECK (unit_price >= 0),
    notes           TEXT           CHECK (notes IS NULL OR btrim(notes) <> ''),
    created_at      TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ    NOT NULL DEFAULT now(),
    CONSTRAINT fk_reservation_items_reservation_business
        FOREIGN KEY (reservation_id, business_id)
        REFERENCES reservations (id, business_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_reservation_items_menu_item_business
        FOREIGN KEY (menu_item_id, business_id)
        REFERENCES menu_items (id, business_id)
        ON DELETE RESTRICT
);

CREATE INDEX idx_reservation_items_business_reservation
    ON reservation_items (business_id, reservation_id);

CREATE INDEX idx_reservation_items_business_menu_item
    ON reservation_items (business_id, menu_item_id);

CREATE TRIGGER trg_reservation_items_updated_at
    BEFORE UPDATE ON reservation_items
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
