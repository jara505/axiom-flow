package service

import (
	"sync"
	"time"
)

type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

// CircuitBreaker implementa el patrón Circuit Breaker para proteger llamadas
// a servicios remotos de fallos en cascada.
type CircuitBreaker struct {
	mu              sync.Mutex
	state           CircuitState
	failures        int
	maxFailures     int
	window          time.Duration
	timeout         time.Duration
	lastFailureTime time.Time
	halfOpenCalls   int
}

// NewCircuitBreaker crea un CircuitBreaker con los parámetros dados.
func NewCircuitBreaker(maxFailures int, timeout, window time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:      StateClosed,
		maxFailures: maxFailures,
		timeout:    timeout,
		window:     window,
	}
}

// Allow devuelve true si se permite ejecutar la llamada, false si el circuito está abierto.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if now.Sub(cb.lastFailureTime) >= cb.timeout {
			cb.state = StateHalfOpen
			cb.halfOpenCalls = 0
			return true
		}
		return false
	case StateHalfOpen:
		if cb.halfOpenCalls >= 1 {
			return false
		}
		cb.halfOpenCalls++
		return true
	}
	return true
}

// RecordSuccess registra una llamada exitosa y cierra el circuito si estaba medio abierto.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen {
		cb.state = StateClosed
		cb.failures = 0
	}
}

// RecordFailure registra una falla y abre el circuito si se supera el umbral.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	if cb.state == StateHalfOpen {
		cb.state = StateOpen
		return
	}

	if cb.failures >= cb.maxFailures {
		cb.state = StateOpen
	}
}

// State devuelve el estado actual del circuito (útil para logs/métricas).
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}