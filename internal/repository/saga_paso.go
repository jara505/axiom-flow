package repository

import (
	"context"
	"database/sql"
	"fmt"

	"axiom-flow/internal/domain"

	"github.com/jmoiron/sqlx"
)

type extTx interface {
	QueryRowxContext(ctx context.Context, query string, args ...interface{}) *sqlx.Row
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
}

type SagaStepRepository struct {
	ext extTx
}

func NewSagaStepRepository(ext extTx) *SagaStepRepository {
	return &SagaStepRepository{ext: ext}
}

// Create registers a new saga step.
func (r *SagaStepRepository) Create(ctx context.Context, ext extTx, step *domain.SagaStep) error {
	query := `
		INSERT INTO saga_steps (order_id, step_name, status, attempt, idempotency_key, operation_key)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, started_at`
	err := ext.QueryRowxContext(ctx, query,
		step.OrderID, step.StepName, step.Status, step.Attempt,
		step.IdempotencyKey, step.OperationKey).
		Scan(&step.ID, &step.StartedAt)
	if err != nil {
		return fmt.Errorf("creating saga step: %w", err)
	}
	return nil
}

// GetByOrderAndStepName searches for a step by order and step name.
func (r *SagaStepRepository) GetByOrderAndStepName(ctx context.Context, ext extTx, orderID int, stepName string) (*domain.SagaStep, error) {
	var step domain.SagaStep
	query := `SELECT id, order_id, step_name, status, attempt, idempotency_key, operation_key, started_at, finished_at 
	          FROM saga_steps WHERE order_id = $1 AND step_name = $2`
	err := ext.GetContext(ctx, &step, query, orderID, stepName)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting saga step: %w", err)
	}
	return &step, nil
}

// UpdateStatus changes the status of a saga step.
func (r *SagaStepRepository) UpdateStatus(ctx context.Context, ext extTx, id int, status domain.SagaStatus) error {
	query := `UPDATE saga_steps SET status = $1, finished_at = CURRENT_TIMESTAMP WHERE id = $2`
	_, err := ext.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("updating saga step status: %w", err)
	}
	return nil
}

// ListByOrder retrieves all saga steps of an order, ordered by started_at.
func (r *SagaStepRepository) ListByOrder(ctx context.Context, orderID int) ([]*domain.SagaStep, error) {
	var steps []*domain.SagaStep
	query := `SELECT id, order_id, step_name, status, attempt, idempotency_key, operation_key, started_at, finished_at 
	          FROM saga_steps WHERE order_id = $1 ORDER BY started_at ASC`
	err := r.ext.SelectContext(ctx, &steps, query, orderID)
	if err != nil {
		return nil, fmt.Errorf("listing saga steps: %w", err)
	}
	return steps, nil
}
