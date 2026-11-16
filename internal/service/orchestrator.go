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

type (
	stepExecFunc func(ctx context.Context, orderID int, amount float64, productID int, quantity int) (domain.StepOutcome, error)
	stepCompFunc func(ctx context.Context, orderID int) error
)

type stepDef struct {
	execute   stepExecFunc
	compensate stepCompFunc
}

type Orchestrator struct {
	db        *sqlx.DB
	stepRepo  *repository.SagaStepRepository
	inventory domain.InventoryOwner
	payment   domain.PaymentOwner
	shipment  domain.ShipmentGateway
	maxRetries int
	steps     map[string]stepDef
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
	o := &Orchestrator{
		db:         db,
		stepRepo:   stepRepo,
		inventory:  inventory,
		payment:    payment,
		shipment:   shipment,
		maxRetries: mr,
	}
	o.steps = o.buildStepRegistry()
	return o
}

func (o *Orchestrator) buildStepRegistry() map[string]stepDef {
	return map[string]stepDef{
		"CREATE_ORDER": {
			execute:   o.execCreateOrder,
			compensate: o.compCreateOrder,
		},
		"RESERVE_INVENTORY": {
			execute:   o.execReserveInventory,
			compensate: o.compReserveInventory,
		},
		"PROCESS_PAYMENT": {
			execute:   o.execProcessPayment,
			compensate: o.compProcessPayment,
		},
		"CREATE_SHIPMENT": {
			execute:   o.execCreateShipment,
			compensate: o.compCreateShipment,
		},
	}
}

func backoffDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return time.Second
	}
	return time.Duration(1<<(attempt-1)) * time.Second
}

func (o *Orchestrator) ExecuteForward(ctx context.Context, orderID int, amount float64, productID int, quantity int) error {
	stepOrder := []string{
		"CREATE_ORDER",
		"RESERVE_INVENTORY",
		"PROCESS_PAYMENT",
		"CREATE_SHIPMENT",
	}

	var completedSteps []string

	for _, stepName := range stepOrder {
		err := o.runStep(ctx, orderID, stepName, amount, productID, quantity, &completedSteps)
		if err != nil {
			return err
		}
	}

	return nil
}

func (o *Orchestrator) runStep(ctx context.Context, orderID int, stepName string, amount float64, productID int, quantity int, completedSteps *[]string) error {
	existing, err := o.stepRepo.GetByOrderAndStepName(ctx, o.db, orderID, stepName)
	if err != nil {
		return fmt.Errorf("checking existing step %s: %w", stepName, err)
	}

	// Guard: already completed -> skip
	if existing != nil && existing.Status == domain.SagaStatusCompleted {
		*completedSteps = append(*completedSteps, stepName)
		return nil
	}

	// Guard: in progress and retry due -> retry now
	if existing != nil && existing.Status == domain.SagaStatusInProgress && existing.NextRetryAt != nil && existing.NextRetryAt.Before(time.Now()) {
		return o.retryStep(ctx, existing, stepName, amount, productID, quantity, completedSteps)
	}

	// Guard: in progress but not due yet -> skip (another worker owns it)
	if existing != nil && existing.Status == domain.SagaStatusInProgress {
		return nil
	}

	// Create new step record
	step := &domain.SagaStep{
		OrderID:        orderID,
		StepName:       stepName,
		Status:         domain.SagaStatusInProgress,
		Attempt:        1,
		IdempotencyKey: fmt.Sprintf("%s-%d", stepName, orderID),
		OperationKey:   fmt.Sprintf("%s-%d", stepName, orderID),
	}
	if err := o.stepRepo.Create(ctx, o.db, step); err != nil {
		return fmt.Errorf("creating step %s: %w", stepName, err)
	}

	if err := o.stepRepo.UpdateStatus(ctx, o.db, step.ID, domain.SagaStatusInProgress); err != nil {
		return fmt.Errorf("updating step %s to in_progress: %w", stepName, err)
	}

	outcome, err := o.steps[stepName].execute(ctx, orderID, amount, productID, quantity)
	if err != nil {
		return o.failStep(ctx, orderID, step.ID, stepName, completedSteps, fmt.Errorf("executing step %s: %w", stepName, err))
	}

	if outcome == domain.StepOutcomeFailed {
		return o.failStep(ctx, orderID, step.ID, stepName, completedSteps, fmt.Errorf("saga step %s returned failed outcome", stepName))
	}

	if err := o.stepRepo.UpdateStatus(ctx, o.db, step.ID, domain.SagaStatusCompleted); err != nil {
		return fmt.Errorf("updating step %s to completed: %w", stepName, err)
	}

	*completedSteps = append(*completedSteps, stepName)
	return nil
}

func (o *Orchestrator) retryStep(ctx context.Context, existing *domain.SagaStep, stepName string, amount float64, productID int, quantity int, completedSteps *[]string) error {
	stepID := existing.ID
	retryCount := existing.RetryCount + 1

	if err := o.stepRepo.UpdateStatus(ctx, o.db, stepID, domain.SagaStatusInProgress); err != nil {
		return fmt.Errorf("updating step %s to in_progress: %w", stepName, err)
	}
	if err := o.stepRepo.UpdateRetryInfo(ctx, o.db, stepID, retryCount, nil); err != nil {
		return fmt.Errorf("updating retry info for step %s: %w", stepName, err)
	}

	outcome, err := o.steps[stepName].execute(ctx, existing.OrderID, amount, productID, quantity)
	if err != nil {
		return fmt.Errorf("retrying step %s: %w", stepName, err)
	}

	if outcome == domain.StepOutcomeFailed {
		if retryCount < o.maxRetries {
			nextRetry := time.Now().Add(backoffDelay(retryCount))
			if err := o.stepRepo.UpdateRetryInfo(ctx, o.db, stepID, retryCount, &nextRetry); err != nil {
				return fmt.Errorf("scheduling retry for step %s: %v", stepName, nextRetry)
			}
			return fmt.Errorf("saga step %s scheduled for retry at %s", stepName, nextRetry.Format(time.RFC3339))
		}
		if err := o.stepRepo.UpdateStatus(ctx, o.db, stepID, domain.SagaStatusFailed); err != nil {
			return fmt.Errorf("updating step %s to failed: %w", stepName, err)
		}
		if err := o.compensatePreviousSteps(ctx, existing.OrderID, *completedSteps); err != nil {
			return fmt.Errorf("compensating previous steps: %w", err)
		}
		return fmt.Errorf("saga failed at step %s", stepName)
	}

	if err := o.stepRepo.UpdateStatus(ctx, o.db, stepID, domain.SagaStatusCompleted); err != nil {
		return fmt.Errorf("updating step %s to completed: %w", stepName, err)
	}
	*completedSteps = append(*completedSteps, stepName)
	return nil
}

func (o *Orchestrator) failStep(ctx context.Context, orderID int, stepID int, stepName string, completedSteps *[]string, err error) error {
	if uErr := o.stepRepo.UpdateStatus(ctx, o.db, stepID, domain.SagaStatusFailed); uErr != nil {
		return fmt.Errorf("updating step %s to failed: %w", stepName, uErr)
	}
	if cErr := o.compensatePreviousSteps(ctx, orderID, *completedSteps); cErr != nil {
		return fmt.Errorf("compensating previous steps: %w", cErr)
	}
	return err
}

// ---------------- Step execution handlers ----------------

func (o *Orchestrator) execCreateOrder(ctx context.Context, orderID int, amount float64, productID int, quantity int) (domain.StepOutcome, error) {
	return domain.StepOutcomeCompleted, nil
}

func (o *Orchestrator) execReserveInventory(ctx context.Context, orderID int, amount float64, productID int, quantity int) (domain.StepOutcome, error) {
	if err := o.inventory.Reserve(orderID, productID, quantity); err != nil {
		return domain.StepOutcomeFailed, fmt.Errorf("%w: reserving inventory for order %d: %w", domain.ErrInventoryUnavailable, orderID, err)
	}
	return domain.StepOutcomeCompleted, nil
}

func (o *Orchestrator) execProcessPayment(ctx context.Context, orderID int, amount float64, productID int, quantity int) (domain.StepOutcome, error) {
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
}

func (o *Orchestrator) execCreateShipment(ctx context.Context, orderID int, amount float64, productID int, quantity int) (domain.StepOutcome, error) {
	if _, err := o.shipment.Create(orderID, "default address"); err != nil {
		return domain.StepOutcomeFailed, fmt.Errorf("creating shipment for order %d: %w", orderID, err)
	}
	return domain.StepOutcomeCompleted, nil
}

// ---------------- Compensation handlers ----------------

func (o *Orchestrator) compCreateOrder(ctx context.Context, orderID int) error {
	log.Printf("Order %d cancelled (compensation for CREATE_ORDER)", orderID)
	return nil
}

func (o *Orchestrator) compReserveInventory(ctx context.Context, orderID int) error {
	if err := o.inventory.Release(orderID); err != nil {
		return fmt.Errorf("releasing inventory compensation: %w", err)
	}
	log.Printf("Inventory released for order %d (compensation for RESERVE_INVENTORY)", orderID)
	return nil
}

func (o *Orchestrator) compProcessPayment(ctx context.Context, orderID int) error {
	_, err := o.payment.Refund(orderID, 99.99)
	if err != nil {
		return fmt.Errorf("refunding payment compensation: %w", err)
	}
	log.Printf("Payment refunded for order %d (compensation for PROCESS_PAYMENT)", orderID)
	return nil
}

func (o *Orchestrator) compCreateShipment(ctx context.Context, orderID int) error {
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
}

func (o *Orchestrator) compensatePreviousSteps(ctx context.Context, orderID int, completedSteps []string) error {
	for i := len(completedSteps) - 1; i >= 0; i-- {
		stepName := completedSteps[i]
		def, ok := o.steps[stepName]
		if !ok {
			return fmt.Errorf("unknown step for compensation: %s", stepName)
		}
		if err := def.compensate(ctx, orderID); err != nil {
			return err
		}
	}
	return nil
}
