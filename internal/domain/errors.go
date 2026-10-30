package domain

import "fmt"

type ErrSagaStepNotFound struct {
	OrderID  int
	StepName string
}

func (e *ErrSagaStepNotFound) Error() string {
	return fmt.Sprintf("saga step %s for order %d not found", e.StepName, e.OrderID)
}

// ErrIdempotencyDuplicated is returned when trying to create a step with an already existing key.
type ErrIdempotencyDuplicated struct {
	Resource string
	Key      string
}

func (e *ErrIdempotencyDuplicated) Error() string {
	return fmt.Sprintf("idempotency duplicated for %s with key %s", e.Resource, e.Key)
}
