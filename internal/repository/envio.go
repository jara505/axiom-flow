package repository

import (
	"context"
	"database/sql"
	"fmt"

	"axiom-flow/internal/domain"
	"github.com/jmoiron/sqlx"
)

// ShipmentRepository gestiona persistencia de envios.
type ShipmentRepository struct {
	db *sqlx.DB
}

// NewShipmentRepository crea una nueva instancia del repositorio.
func NewShipmentRepository(db *sqlx.DB) *ShipmentRepository {
	return &ShipmentRepository{db: db}
}

// Create inserta un nuevo envío.
func (r *ShipmentRepository) Create(ctx context.Context, s *domain.Shipment) error {
	query := `
		INSERT INTO shipments (order_id, tracking_number, status, shipped_at, address)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, shipped_at`
	err := r.db.QueryRowxContext(ctx, query,
		s.OrderID, s.TrackingNumber, s.Status, s.ShippedAt, s.Address).
		Scan(&s.ID, &s.ShippedAt)
	if err != nil {
		return fmt.Errorf("creating shipment: %w", err)
	}
	return nil
}

// GetByOrder recupera el envío asociado a una orden.
func (r *ShipmentRepository) GetByOrder(ctx context.Context, orderID int) (*domain.Shipment, error) {
	var s domain.Shipment
	query := `SELECT id, order_id, tracking_number, status, shipped_at, delivered_at, address 
	          FROM shipments WHERE order_id = $1`
	err := r.db.GetContext(ctx, &s, query, orderID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting shipment by order: %w", err)
	}
	return &s, nil
}