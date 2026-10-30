package domain

import (
	"time"
)

// Payment represents a payment associated with an order.
type Payment struct {
	ID             int       `db:"id" json:"id"`
	OrderID        int       `db:"order_id" json:"order_id"`
	IdempotencyKey string   `db:"idempotency_key" json:"idempotency_key"`
	Amount         float64   `db:"amount" json:"amount"`
	Currency       string    `db:"currency" json:"currency"`
	Method         string    `db:"method" json:"method"`
	Provider       string    `db:"provider" json:"provider"`
	Status         string    `db:"status" json:"status"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
}