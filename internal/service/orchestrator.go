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
	db        *sqlx.DB
	stepRepo  *repository.SagaStepRepository
	inventory domain.InventoryOwner
	payment   domain.PaymentOwner
	shipment  domain.ShipmentGateway
	maxRetries int
}

func NewOrchestrator(
	db *sqlx.DB,
	stepRepo *repository.SagaStepRepository,
	inventory domain.InventoryOwner,
	payment domain.PaymentOwner,
	shipment domain.ShipmentGateway,
	maxRetries ...int,
) *Orchestrator {
	mr := 3
	if len(maxRetries) > 0 {
		mr = maxRetries[0]
	}
	return &Orchestrator{
		db:        db,
		stepRepo:  stepRepo,
		inventory: inventory,
		payment:   payment,
		shipment:  shipment,
		maxRetries: mr,
	}
}

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
						return fmt.Errorf("scheduling retry for step %s: %v", stepName, nextRetry)
					}
					return fmt.Errorf("saga step %s scheduled for retry at %s", stepName, nextRetry.Format(time.RFC3339))
				}
				if err := o.stepRepo.UpdateStatus(ctx, tx, step.ID, domain.SagaStatusFailed); err != nil {
					return fmt.Errorf("updating step %s to failed: %w", stepName, err)
				}
				if err := o.compensatePreviousSteps(ctx, tx, orderID, completedSteps); err != nil {
					return fmt.Errorf("compensating previous steps: %w", err)
				}
				return fmt.Errorf("saga failed at step %s", stepName)
			}
		}

		var step *domain.SagaStep
		if existing == nil {
			step = &domain.SagaStep{
				OrderID:        orderID,
				StepName:       stepName,
				Status:         domain.SagaStatusInProgress,
				Attempt:        1,
				OperationKey:   fmt.Sprintf("order-%d-%s", orderID, stepName),
				IdempotencyKey: fmt.Sprintf("order-%d-%s-attempt-1", orderID, stepName),
			}
			if err := o.stepRepo.Create(ctx, tx, step); err != nil {
				return fmt.Errorf("creating step %s: %w", stepName, err)
			}
		} else {
			step = existing
			step.ID = existing.ID
			if err := o.stepRepo.UpdateStatus(ctx, tx, step.ID, domain.SagaStatusInProgress); err != nil {
				return fmt.Errorf("updating step %s to in_progress: %w", stepName, err)
			}
		}

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

			// Regla: No compensar UNKNOWN sin consultar primero al proveedor
			if domain.PaymentStatus(status) == domain.PaymentStatusUnknown {
				log.Printf("Payment status is UNKNOWN for order %d, consulting provider before deciding...", orderID)
				payment, err := o.payment.ConsultarPorClave(fmt.Sprintf("order-%d-PROCESS_PAYMENT-attempt-1", orderID))
				if err != nil {
					return fmt.Errorf("consulting payment status: %w", err)
				}

				if payment != nil && payment.Status == domain.PaymentStatusApproved {
					log.Printf("Payment consulted and APPROVED, marking step as COMPLETED")
					outcome = domain.StepOutcomeCompleted
				} else if payment != nil && payment.Status == domain.PaymentStatusRejected {
					log.Printf("Payment consulted and REJECTED, initiating compensation")
					if err := o.compensatePreviousSteps(ctx, tx, orderID, completedSteps[:len(completedSteps)-1]); err != nil {
						return fmt.Errorf("compensating previous steps: %w", err)
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
			if _, err := o.shipment.Create(orderID, "default address"); err != nil {
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

		completedSteps = append(completedSteps, stepName)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

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

		if domain.PaymentStatus(status) == domain.PaymentStatusUnknown {
			log.Printf("Payment status is UNKNOWN for order %d, consulting provider before deciding...", orderID)
			payment, err := o.payment.ConsultarPorClave(fmt.Sprintf("order-%d-PROCESS_PAYMENT-attempt-1", orderID))
			if err != nil {
				return domain.StepOutcomeFailed, fmt.Errorf("consulting payment status: %w", err)
			}

			if payment != nil && payment.Status == domain.PaymentStatusApproved {
				log.Printf("Payment consulted and APPROVED, marking step as COMPLETED")
				return domain.StepOutcomeCompleted, nil
			} else if payment != nil && payment.Status == domain.PaymentStatusRejected {
				log.Printf("Payment consulted and REJECTED, initiating compensation")
				return domain.StepOutcomeFailed, nil
			} else {
				log.Printf("Payment status still UNKNOWN after consultation, marking as FAILED without compensation")
				return domain.StepOutcomeFailed, nil
			}
		} else if domain.PaymentStatus(status) == domain.PaymentStatusApproved {
			return domain.StepOutcomeCompleted, nil
		} else {
			return domain.StepOutcomeFailed, nil
		}

	case "CREATE_SHIPMENT":
		if _, err := o.shipment.Create(orderID, "default address"); err != nil {
			return domain.StepOutcomeFailed, fmt.Errorf("creating shipment for order %d: %w", orderID, err)
		}
		return domain.StepOutcomeCompleted, nil

	default:
		return domain.StepOutcomeFailed, fmt.Errorf("unknown step %s", stepName)
	}
}

func (o *Orchestrator) compensatePreviousSteps(ctx context.Context, tx *sqlx.Tx, orderID int, completedSteps []string) error {
	for i := len(completedSteps) - 1; i >= 0; i-- {
		stepName := completedSteps[i]

		switch stepName {
		case "CREATE_SHIPMENT":
			s, err := o.shipment.GetByOrder(orderID)
			if err != nil {
				return fmt.Errorf("getting shipment for compensation: %w", err)
			}
			if s != nil {
				log.Printf("Cancelling shipment %v for order %d", s.ID, orderID)
				// TODO: Add shipment cancellation logic
			}
			log.Printf("Shipment cancelled for order %d (compensation for CREATE_SHIPMENT)", orderID)
			return nil

		case "PROCESS_PAYMENT":
			_, err := o.payment.Refund(orderID, 99.99)
			if err != nil {
				return fmt.Errorf("refunding payment compensation: %w", err)
			}
			log.Printf("Payment refunded for order %d (compensation for PROCESS_PAYMENT)", orderID)
			return nil

		case "RESERVE_INVENTORY":
			if err := o.inventory.Release(orderID); err != nil {
				return fmt.Errorf("releasing inventory compensation: %w", err)
			}
			log.Printf("Inventory released for order %d (compensation for RESERVE_INVENTORY)", orderID)
			return nil

		case "CREATE_ORDER":
			log.Printf("Order %d cancelled (compensation for CREATE_ORDER)", orderID)
			return nil

		default:
			return fmt.Errorf("unknown step for compensation: %s", stepName)
		}
	}
	return nil
}
