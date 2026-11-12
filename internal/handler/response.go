package handler

import (
	"time"
)

// OrderResponse es la respuesta estandarizada para creación de órdenes.
type OrderResponse struct {
	OrderID   int       `json:"order_id"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// SagaStepResponse representa un paso de la saga en la respuesta.
type SagaStepResponse struct {
	ID          int       `json:"id"`
	OrderID     int       `json:"order_id"`
	StepName    string    `json:"step_name"`
	Status      string    `json:"status"`
	Attempt     int       `json:"attempt"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

// SagaResponse es la respuesta de consulta de saga.
type SagaResponse struct {
	OrderID int                `json:"order_id"`
	Steps   []SagaStepResponse `json:"steps"`
	Message string             `json:"message"`
}

// ErrorResponse es la respuesta estandarizada para errores.
type ErrorResponse struct {
	Error     string    `json:"error"`
	Message   string    `json:"message,omitempty"`
	OrderID   *int      `json:"order_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

func now() time.Time {
	return time.Now().UTC()
}