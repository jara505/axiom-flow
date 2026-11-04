package repository

import (
	"context"
	"database/sql"
	"fmt"

	"axiom-flow/internal/domain"
	"github.com/jmoiron/sqlx"
)

// InventoryReservationRepository manages persistence of inventory reservations.
type InventoryReservationRepository struct {
	db *sqlx.DB
}

// NewInventoryReservationRepository creates a new instance of the repository.
func NewInventoryReservationRepository(db *sqlx.DB) *InventoryReservationRepository {
	return &InventoryReservationRepository{db: db}
}

// Create inserts a new inventory reservation.
// Returns an error if (order_id, product_id) already exists (idempotency via UNIQUE constraint).
func (r *InventoryReservationRepository) Create(ctx context.Context, res *domain.InventoryReservation) error {
	query := `
		INSERT INTO inventory_reservations (order_id, product_id, quantity, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id, reserved_at`
	err := r.db.QueryRowxContext(ctx, query,
		res.OrderID, res.ProductID, res.Quantity, res.Status).
		Scan(&res.ID, &res.ReservedAt)
	if err != nil {
		return fmt.Errorf("creating inventory reservation: %w", err)
	}
	return nil
}

// GetByOrder retrieves the reservation associated with an order.
func (r *InventoryReservationRepository) GetByOrder(ctx context.Context, orderID int) (*domain.InventoryReservation, error) {
	var res domain.InventoryReservation
	query := `SELECT id, order_id, product_id, quantity, status, reserved_at, released_at 
	          FROM inventory_reservations WHERE order_id = $1`
	err := r.db.GetContext(ctx, &res, query, orderID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting inventory reservation by order: %w", err)
	}
	return &res, nil
}

// Release atomically releases the reservation (RESERVED -> RELEASED) within a transaction.
func (r *InventoryReservationRepository) Release(ctx context.Context, tx *sqlx.Tx, id int) error {
	query := `UPDATE inventory_reservations 
	          SET status = 'RELEASED', released_at = CURRENT_TIMESTAMP 
	          WHERE id = $1 AND status = 'RESERVED'`
	result, err := tx.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("releasing inventory reservation: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("inventory reservation %d not found or already released", id)
	}
	return nil
}