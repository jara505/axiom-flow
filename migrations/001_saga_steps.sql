CREATE TABLE IF NOT EXISTS saga_steps (
    id SERIAL PRIMARY KEY,
    order_id INTEGER NOT NULL,
    step_name VARCHAR(50) NOT NULL CHECK (step_name IN (
        'CREATE_ORDER',
        'RESERVE_INVENTORY',
        'PROCESS_PAYMENT',
        'CREATE_SHIPMENT',
        'REFUND_PAYMENT',
        'RELEASE_INVENTORY'
    )),
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN (
        'PENDING',
        'IN_PROGRESS',
        'COMPLETED',
        'FAILED',
        'UNKNOWN'
    )),
    attempt INTEGER NOT NULL DEFAULT 1,
    idempotency_key VARCHAR(100) NOT NULL UNIQUE,
    operation_key VARCHAR(100) NOT NULL UNIQUE,
    started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
    finished_at TIMESTAMP,
    FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE RESTRICT,
    UNIQUE (order_id, step_name)
);
