-- Migration: inventory reservations table (Walking Skeleton - Step 3)
-- Rule: status can only transition RESERVED -> RELEASED within a local transaction.
CREATE TABLE IF NOT EXISTS inventory_reservations (
    id SERIAL PRIMARY KEY,
    order_id INTEGER NOT NULL,
    product_id INTEGER NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    status VARCHAR(20) NOT NULL DEFAULT 'RESERVED' CHECK (status IN ('RESERVED', 'RELEASED')),
    reserved_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
    released_at TIMESTAMP,
    FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE RESTRICT,
    UNIQUE (order_id, product_id)
);