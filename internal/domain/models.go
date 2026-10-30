package domain

import (
	"time"
)

// SagaStatus represents the possible states of a saga step.
type SagaStatus string

const (
	SagaStatusPending   SagaStatus = "PENDING"
	SagaStatusInProgress SagaStatus = "IN_PROGRESS"
	SagaStatusCompleted SagaStatus = "COMPLETED"
	SagaStatusFailed    SagaStatus = "FAILED"
	SagaStatusUnknown   SagaStatus = "UNKNOWN"
)

// StepOutcome is the result of a saga step.
type StepOutcome string

const (
	StepOutcomeCompleted StepOutcome = "COMPLETED"
	StepOutcomeFailed    StepOutcome = "FAILED"
	StepOutcomeUnknown   StepOutcome = "UNKNOWN"
)

// PaymentStatus represents the possible states of a payment.
type PaymentStatus string

const (
	PaymentStatusApproved  PaymentStatus = "APPROVED"
	PaymentStatusRejected  PaymentStatus = "REJECTED"
	PaymentStatusRefunded  PaymentStatus = "REFUNDED"
	PaymentStatusUnknown   PaymentStatus = "UNKNOWN"
	PaymentStatusPending   PaymentStatus = "PENDING"
)

// SagaStep registers the state of each step of the Saga.
type SagaStep struct {
	ID             int        `db:"id" json:"id"`
	OrderID        int        `db:"order_id" json:"order_id"`
	StepName       string     `db:"step_name" json:"step_name"`
	Status         SagaStatus `db:"status" json:"status"`
	Attempt        int        `db:"attempt" json:"attempt"`
	IdempotencyKey string     `db:"idempotency_key" json:"idempotency_key"`
	OperationKey   string     `db:"operation_key" json:"operation_key"`
	StartedAt      time.Time  `db:"started_at" json:"started_at"`
	FinishedAt     *time.Time `db:"finished_at" json:"finished_at,omitempty"`
}
