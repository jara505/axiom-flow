package domain

import (
	"time"
)

// InventoryReservation represents an inventory stock reservation for an order.
type InventoryReservation struct {
	ID          int       `db:"id" json:"id"`
	OrderID     int       `db:"order_id" json:"order_id"`
	ProductID   int       `db:"product_id" json:"product_id"`
	Quantity    int       `db:"quantity" json:"quantity"`
	Status      string    `db:"status" json:"status"`
	ReservedAt  time.Time `db:"reserved_at" json:"reserved_at"`
	ReleasedAt  *time.Time `db:"released_at" json:"released_at,omitempty"`
}