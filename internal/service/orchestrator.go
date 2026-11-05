package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"axiom-flow/internal/domain"
	"axiom-flow/internal/repository"

	"github.com/jmoiron/sqlx"
)

type Orchestrator struct {
	db           *sqlx.DB
	stepRepo     *repository.SagaStepRepository
	inventory    domain.InventoryOwner
	payment      domain.PaymentOwner
	shipmentRepo *repository.ShipmentRepository
	maxRetries   int
}

func NewOrchestrator(
	db *sqlx.DB,
	stepRepo *repository.SagaStepRepository,
	inventory domain.InventoryOwner,
	payment domain.PaymentOwner,
	shipmentRepo *repository.ShipmentRepository,
	maxRetries ...int,
) *Orchestrator {
	mr := 3
	if len(maxRetries) > 0 {
		mr = maxRetries[0]
	}
	return &Orchestrator{
		db:           db,
		stepRepo:     stepRepo,
		inventory:    inventory,
		payment:      payment,
		shipmentRepo: shipmentRepo,
		maxRetries:   mr,
	}
}

// backoffDelay returns the delay duration for a given retry attempt using exponential backoff.
func backoffDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return time.Second
	}
	return time.Duration(1<<(attempt-1)) * time.Second
}

func (o *Orchestrator) ExecuteForward(ctx context.Context, orderID int, amount float64, productID int, quantity int) error {
	tx, err := o.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	steps := []string{
		"CREATE_ORDER",
		"RESERVE_INVENTORY",
		"PROCESS_PAYMENT",
		"CREATE_SHIPMENT",
	}

	for _, stepName := range steps {
		existing, err := o.stepRepo.GetByOrderAndStepName(ctx, tx, orderID, stepName)
		if err != nil {
			return fmt.Errorf("checking existing step %s: %w", stepName, err)
		}

		if existing != nil && existing.Status == domain.SagaStatusCompleted {
			continue
		}

		if existing != nil && existing.Status == domain.SagaStatusInProgress && existing.NextRetryAt != nil && existing.NextRetryAt.Before(time.Now()) {
			step := existing
			step.Attempt = existing.Attempt + 1
			step.RetryCount = existing.RetryCount + 1
			step.Status = domain.SagaStatusInProgress

			if err := o.stepRepo.UpdateStatus(ctx, tx, step.ID, domain.SagaStatusInProgress); err != nil {
				return fmt.Errorf("updating step %s to in_progress: %w", stepName, err)
			}
			if err := o.stepRepo.UpdateRetryInfo(ctx, tx, step.ID, step.RetryCount, nil); err != nil {
				return fmt.Errorf("updating retry info for step %s: %w", stepName, err)
			}

			var outcome domain.StepOutcome
			outcome, err = o.executeStep(ctx, stepName, orderID, amount, productID, quantity)
			if err != nil {
				return fmt.Errorf("retrying step %s: %w", stepName, err)
			}

			if outcome == domain.StepOutcomeFailed {
				if step.RetryCount < o.maxRetries {
					nextRetry := time.Now().Add(backoffDelay(step.RetryCount))
					if err := o.stepRepo.UpdateRetryInfo(ctx, tx, step.ID, step.RetryCount, &nextRetry); err != nil {
						return fmt.Errorf("scheduling retry for step %s: %w", stepName, err)
					}
					return fmt.Errorf("saga step %s scheduled for retry at %s", stepName, nextRetry.Format(time.RFC3339))
				}
				if err := o.stepRepo.UpdateStatus(ctx, tx, step.ID, domain.SagaStatusFailed); err != nil {
					return fmt.Errorf("updating step %s to failed: %w", stepName, err)
				}
				if err := o.compensatePreviousSteps(ctx, tx, orderID, []string{stepName}); err != nil {
					return fmt.Errorf("compensating previous steps for order %d: %w", orderID, err)
				}
				return fmt.Errorf("saga failed at step %s", stepName)
			}

			if err := o.stepRepo.UpdateStatus(ctx, tx, step.ID, domain.SagaStatusCompleted); err != nil {
				return fmt.Errorf("updating step %s to completed: %w", stepName, err)
			}
			continue
		}

		step := &domain.SagaStep{
			OrderID:        orderID,
			StepName:       stepName,
			Status:         domain.SagaStatusInProgress,
			Attempt:        1,
			RetryCount:     0,
			OperationKey:   fmt.Sprintf("order-%d-%s", orderID, stepName),
			IdempotencyKey: fmt.Sprintf("order-%d-%s-attempt-%d", orderID, stepName, 1),
		}

		if existing == nil {
			if err := o.stepRepo.Create(ctx, tx, step); err != nil {
				return fmt.Errorf("creating step %s: %w", stepName, err)
			}
		} else {
			step.ID = existing.ID
			step.Attempt = existing.Attempt + 1
			if err := o.stepRepo.UpdateStatus(ctx, tx, step.ID, domain.SagaStatusInProgress); err != nil {
				return fmt.Errorf("updating step %s to in_progress: %w", stepName, err)
			}
		}

		var outcome domain.StepOutcome
		outcome, err = o.executeStep(ctx, stepName, orderID, amount, productID, quantity)
		if err != nil {
			if step.RetryCount < o.maxRetries {
				nextRetry := time.Now().Add(backoffDelay(step.RetryCount + 1))
				if rerr := o.stepRepo.UpdateRetryInfo(ctx, tx, step.ID, step.RetryCount+1, &nextRetry); rerr != nil {
					return fmt.Errorf("scheduling retry for step %s: %w", stepName, rerr)
				}
				return fmt.Errorf("saga step %s scheduled for retry at %s", stepName, nextRetry.Format(time.RFC3339))
			}
			if err := o.stepRepo.UpdateStatus(ctx, tx, step.ID, domain.SagaStatusFailed); err != nil {
				return fmt.Errorf("updating step %s to failed: %w", stepName, err)
			}
			if err := o.compensatePreviousSteps(ctx, tx, orderID, []string{stepName}); err != nil {
				return fmt.Errorf("compensating previous steps for order %d: %w", orderID, err)
			}
			return fmt.Errorf("saga failed at step %s", stepName)
		}

		if outcome == domain.StepOutcomeFailed {
			if err := o.stepRepo.UpdateStatus(ctx, tx, step.ID, domain.SagaStatusFailed); err != nil {
				return fmt.Errorf("updating step %s to failed: %w", stepName, err)
			}
			if err := o.compensatePreviousSteps(ctx, tx, orderID, []string{stepName}); err != nil {
				return fmt.Errorf("compensating previous steps for order %d: %w", orderID, err)
			}
			return fmt.Errorf("saga failed at step %s", stepName)
		}

		if err := o.stepRepo.UpdateStatus(ctx, tx, step.ID, domain.SagaStatusCompleted); err != nil {
			return fmt.Errorf("updating step %s to completed: %w", stepName, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// compensateStep undo a single step based on its name
func (o *Orchestrator) compensateStep(ctx context.Context, tx *sqlx.Tx, orderID int, stepName string) error {
	log.Printf("Compensating step: %s for order %d", stepName, orderID)

	switch stepName {
	case "CREATE_ORDER":
		log.Printf("No compensation needed for CREATE_ORDER")
		return nil

	case "RESERVE_INVENTORY":
		if o.inventory != nil {
			err := o.inventory.Release(orderID)
			if err != nil {
				return fmt.Errorf("releasing inventory compensation: %w", err)
			}
		}
		log.Printf("Inventory released for order %d (compensation for RESERVE_INVENTORY)", orderID)
		return nil

	case "PROCESS_PAYMENT":
		if o.payment != nil {
			step, err := o.stepRepo.GetByOrderAndStepName(ctx, tx, orderID, "PROCESS_PAYMENT")
			if err != nil {
				return fmt.Errorf("getting payment step for compensation: %w", err)
			}
			if step != nil && step.IdempotencyKey != "" {
				status, err := o.payment.Refund(orderID, 99.99)
				if err != nil {
					return fmt.Errorf("refunding payment compensation: %w", err)
				}
				log.Printf("Payment refunded: %s", status)
			}
		}
		log.Printf("Payment refunded for order %d (compensation for PROCESS_PAYMENT)", orderID)
		return nil

	case "CREATE_SHIPMENT":
		if o.shipmentRepo != nil {
			s, err := o.shipmentRepo.GetByOrder(ctx, orderID)
			if err != nil {
				return fmt.Errorf("getting shipment for compensation: %w", err)
			}
			if s != nil {
				log.Printf("Cancelling shipment %v for order %d", s.ID, orderID)
				// TODO: Add shipment cancellation logic
			}
		}
		log.Printf("Shipment cancelled for order %d (compensation for CREATE_SHIPMENT)", orderID)
		return nil

	default:
		return fmt.Errorf("unknown step for compensation: %s", stepName)
	}
}

// executeStep runs the business logic for a single saga step and returns the outcome.
func (o *Orchestrator) executeStep(ctx context.Context, stepName string, orderID int, amount float64, productID int, quantity int) (domain.StepOutcome, error) {
	switch stepName {
	case "RESERVE_INVENTORY":
		if err := o.inventory.Reserve(orderID, productID, quantity); err != nil {
			return domain.StepOutcomeFailed, fmt.Errorf("%w: reserving inventory for order %d: %w", domain.ErrInventoryUnavailable, orderID, err)
		}
		return domain.StepOutcomeCompleted, nil
	case "CREATE_ORDER":
		return domain.StepOutcomeCompleted, nil
	case "PROCESS_PAYMENT":
		status, err := o.payment.Charge(orderID, amount)
		if err != nil {
			return domain.StepOutcomeFailed, fmt.Errorf("%w: processing payment for order %d: %w", domain.ErrPaymentFailed, orderID, err)
		}
		ps := domain.PaymentStatus(status)
		if ps == domain.PaymentStatusUnknown {
			payment, err := o.payment.ConsultarPorClave(fmt.Sprintf("order-%d-PROCESS_PAYMENT-attempt-1", orderID))
			if err != nil {
				return domain.StepOutcomeFailed, fmt.Errorf("consulting payment status: %w", err)
			}
			if payment != nil && payment.Status == domain.PaymentStatusApproved {
				return domain.StepOutcomeCompleted, nil
			} else if payment != nil && payment.Status == domain.PaymentStatusRejected {
				return domain.StepOutcomeFailed, nil
			}
			return domain.StepOutcomeFailed, nil
		} else if ps == domain.PaymentStatusApproved {
			return domain.StepOutcomeCompleted, nil
		}
		return domain.StepOutcomeFailed, nil
	case "CREATE_SHIPMENT":
		s := &domain.Shipment{
			OrderID:        orderID,
			TrackingNumber: fmt.Sprintf("TRACK-%d", orderID),
			Status:         "SHIPPED",
		}
		if err := o.shipmentRepo.Create(ctx, s); err != nil {
			return domain.StepOutcomeFailed, fmt.Errorf("creating shipment for order %d: %w", orderID, err)
		}
		return domain.StepOutcomeCompleted, nil
	default:
		return domain.StepOutcomeFailed, fmt.Errorf("unknown step %s", stepName)
	}
}

// compensatePreviousSteps compensates steps that were completed before the failure,
// executing them in reverse order
func (o *Orchestrator) compensatePreviousSteps(ctx context.Context, tx *sqlx.Tx, orderID int, stepsToCompensate []string) error {
	log.Printf("Compensating %d previous steps for order %d", len(stepsToCompensate), orderID)

	// Compensate in reverse order
	for i := len(stepsToCompensate) - 1; i >= 0; i-- {
		stepName := stepsToCompensate[i]
		err := o.compensateStep(ctx, tx, orderID, stepName)
		if err != nil {
			log.Printf("Error compensating step %s: %v", stepName, err)
			return fmt.Errorf("error compensating step %s: %w", stepName, err)
		}
	}

	// Mark the saga as failed after compensation
	if len(stepsToCompensate) > 0 {
		firstStep, err := o.stepRepo.GetByOrderAndStepName(ctx, tx, orderID, stepsToCompensate[0])
		if err != nil {
			return fmt.Errorf("getting first step for status update: %w", err)
		}

		if firstStep != nil {
			if err := o.stepRepo.UpdateStatus(ctx, tx, firstStep.ID, domain.SagaStatusFailed); err != nil {
				return fmt.Errorf("updating saga status to FAILED: %w", err)
			}
			log.Printf("Saga marked as FAILED after compensation for order %d", orderID)
		}
	}

	return nil
}

// ExecuteForwardWithCompensation ejecuta el flujo forward con compensación automática si un paso falla.
// Incluye la regla: no compensar UNKNOWN sin consultar primero al proveedor.
func (o *Orchestrator) ExecuteForwardWithCompensation(ctx context.Context, orderID int, amount float64, productID int, quantity int) error {
	tx, err := o.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	compensate := true

	steps := []string{
		"CREATE_ORDER",
		"RESERVE_INVENTORY",
		"PROCESS_PAYMENT",
		"CREATE_SHIPMENT",
	}

	var completedSteps []string

	for _, stepName := range steps {
		existing, err := o.stepRepo.GetByOrderAndStepName(ctx, tx, orderID, stepName)
		if err != nil {
			return fmt.Errorf("checking existing step %s: %w", stepName, err)
		}

		if existing != nil && existing.Status == domain.SagaStatusCompleted {
			completedSteps = append(completedSteps, stepName)
			continue
		}

		step := &domain.SagaStep{
			OrderID:        orderID,
			StepName:       stepName,
			Status:         domain.SagaStatusInProgress,
			Attempt:        1,
			OperationKey:   fmt.Sprintf("order-%d-%s", orderID, stepName),
			IdempotencyKey: fmt.Sprintf("order-%d-%s-attempt-1", orderID, stepName),
		}

		if existing == nil {
			if err := o.stepRepo.Create(ctx, tx, step); err != nil {
				return fmt.Errorf("creating step %s: %w", stepName, err)
			}
		} else {
			step.ID = existing.ID
			if err := o.stepRepo.UpdateStatus(ctx, tx, step.ID, domain.SagaStatusInProgress); err != nil {
				return fmt.Errorf("updating step %s to in_progress: %w", stepName, err)
			}
		}

		completedSteps = append(completedSteps, stepName)

		var outcome domain.StepOutcome

		switch stepName {
		case "RESERVE_INVENTORY":
			if err := o.inventory.Reserve(orderID, productID, quantity); err != nil {
				return fmt.Errorf("%w: reserving inventory for order %d: %w", domain.ErrInventoryUnavailable, orderID, err)
			}
			outcome = domain.StepOutcomeCompleted

		case "CREATE_ORDER":
			outcome = domain.StepOutcomeCompleted

		case "PROCESS_PAYMENT":
			status, err := o.payment.Charge(orderID, amount)
			if err != nil {
				return fmt.Errorf("%w: processing payment for order %d: %w", domain.ErrPaymentFailed, orderID, err)
			}

			// **Regla: No compensar UNKNOWN sin consultar primero al proveedor**
			if domain.PaymentStatus(status) == domain.PaymentStatusUnknown {
				log.Printf("Payment status is UNKNOWN for order %d, consulting provider before deciding...", orderID)
				payment, err := o.payment.ConsultarPorClave(step.IdempotencyKey)
				if err != nil {
					return fmt.Errorf("consulting payment status: %w", err)
				}

				if payment != nil && payment.Status == domain.PaymentStatusApproved {
					log.Printf("Payment consulted and APPROVED, marking step as COMPLETED")
					outcome = domain.StepOutcomeCompleted
				} else if payment != nil && payment.Status == domain.PaymentStatusRejected {
					log.Printf("Payment consulted and REJECTED, initiating compensation")
					if compensate {
						return o.compensatePreviousSteps(ctx, tx, orderID, completedSteps[:len(completedSteps)-1])
					}
					outcome = domain.StepOutcomeFailed
				} else {
					log.Printf("Payment status still UNKNOWN after consultation, marking as FAILED without compensation")
					outcome = domain.StepOutcomeFailed
				}
			} else if domain.PaymentStatus(status) == domain.PaymentStatusApproved {
				outcome = domain.StepOutcomeCompleted
			} else {
				outcome = domain.StepOutcomeFailed
			}

		case "CREATE_SHIPMENT":
			s := &domain.Shipment{
				OrderID:        orderID,
				TrackingNumber: fmt.Sprintf("TRACK-%d", orderID),
				Status:         "SHIPPED",
			}
			if err := o.shipmentRepo.Create(ctx, s); err != nil {
				return fmt.Errorf("creating shipment for order %d: %w", orderID, err)
			}
			outcome = domain.StepOutcomeCompleted
		}

		finalStatus := domain.SagaStatusCompleted
		if outcome == domain.StepOutcomeFailed {
			finalStatus = domain.SagaStatusFailed
		}

		if err := o.stepRepo.UpdateStatus(ctx, tx, step.ID, finalStatus); err != nil {
			return fmt.Errorf("updating step %s to final status: %w", stepName, err)
		}

		if finalStatus == domain.SagaStatusFailed {
			return fmt.Errorf("saga failed at step %s", stepName)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}
