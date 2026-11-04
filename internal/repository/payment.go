package repository

import (
	"context"
	"database/sql"
	"fmt"

	"axiom-flow/internal/domain"
	"github.com/jmoiron/sqlx"
)

// PaymentRepository manages persistence of payments.
type PaymentRepository struct {
	db *sqlx.DB
}

// NewPaymentRepository creates a new instance of the repository.
func NewPaymentRepository(db *sqlx.DB) *PaymentRepository {
	return &PaymentRepository{db: db}
}

// Create inserts a new payment.
// Returns an error if idempotency_key already exists (UNIQUE constraint).
func (r *PaymentRepository) Create(ctx context.Context, p *domain.Payment) error {
	query := `
		INSERT INTO payments (order_id, idempotency_key, amount, currency, method, provider, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`
	err := r.db.QueryRowxContext(ctx, query,
		p.OrderID, p.IdempotencyKey, p.Amount, p.Currency, p.Method, p.Provider, p.Status).
		Scan(&p.ID, &p.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating payment: %w", err)
	}
	return nil
}

// GetByIdempotencyKey retrieves a payment by its idempotency key (operation key).
// Returns nil if not found.
func (r *PaymentRepository) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error) {
	var p domain.Payment
	query := `SELECT id, order_id, idempotency_key, amount, currency, method, provider, status, created_at 
	          FROM payments WHERE idempotency_key = $1`
	err := r.db.GetContext(ctx, &p, query, key)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting payment by idempotency key: %w", err)
	}
	return &p, nil
}

// UpdateStatus changes the status of a payment.
func (r *PaymentRepository) UpdateStatus(ctx context.Context, id int, status string) error {
	query := `UPDATE payments SET status = $1 WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("updating payment status: %w", err)
	}
	return nil
}