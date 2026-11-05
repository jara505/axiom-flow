package domain

import (
	"errors"
	"fmt"
)

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

var (
	// ErrOrderNotFound is returned when an order does not exist.
	ErrOrderNotFound = errors.New("order not found")

	// ErrPaymentFailed is returned when a payment charge or refund fails.
	ErrPaymentFailed = errors.New("payment processing failed")

	// ErrInventoryUnavailable is returned when inventory cannot be reserved.
	ErrInventoryUnavailable = errors.New("inventory unavailable")

	// ErrShipmentFailed is returned when a shipment cannot be created.
	ErrShipmentFailed = errors.New("shipment creation failed")

	// ErrSagaStepFailed is returned when a saga step fails.
	ErrSagaStepFailed = errors.New("saga step failed")

	// ErrUnknownPaymentStatus is returned when a payment status is UNKNOWN.
	ErrUnknownPaymentStatus = errors.New("unknown payment status")
)
