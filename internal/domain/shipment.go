package domain

import (
	"time"
)

// Shipment representa un envío asociado a un orden.
type Shipment struct {
	ID          int       `db:"id" json:"id"`
	OrderID     int       `db:"order_id" json:"order_id"`
	TrackingNumber string   `db:"tracking_number" json:"tracking_number,omitempty"`
	Status      string    `db:"status" json:"status"`
	ShippedAt   time.Time `db:"shipped_at" json:"shipped_at"`
	DeliveredAt *time.Time `db:"delivered_at" json:"delivered_at,omitempty"`
	Address     string    `db:"address" json:"address,omitempty"`
}