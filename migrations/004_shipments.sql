-- Migration: shipments table (Walking Skeleton - Step 5)
CREATE TABLE IF NOT EXISTS shipments (
    id SERIAL PRIMARY KEY,
    order_id INTEGER NOT NULL,
    tracking_number VARCHAR(100),
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'SHIPPED', 'DELIVERED', 'FAILED', 'COMPLETED')),
    shipped_at TIMESTAMP,
    delivered_at TIMESTAMP,
    address TEXT,
    FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE RESTRICT,
    UNIQUE (order_id, tracking_number)
);