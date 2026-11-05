package service

import (
	"context"
	"log/slog"
	"time"

	"axiom-flow/internal/domain"
	"axiom-flow/internal/repository"

	"github.com/jmoiron/sqlx"
)

// RecoveryWorker reintenta pasos de Saga que quedaron pendientes por tiempo de espera agotado.
type RecoveryWorker struct {
	db           *sqlx.DB
	stepRepo     *repository.SagaStepRepository
	orchestrator *Orchestrator
	interval     time.Duration
	stop         chan struct{}
}

// NewRecoveryWorker crea un nuevo worker de recuperación.
func NewRecoveryWorker(db *sqlx.DB, stepRepo *repository.SagaStepRepository, orchestrator *Orchestrator, interval time.Duration) *RecoveryWorker {
	return &RecoveryWorker{
		db:           db,
		stepRepo:     stepRepo,
		orchestrator: orchestrator,
		interval:     interval,
		stop:         make(chan struct{}),
	}
}

// Start inicia el worker en segundo plano.
func (w *RecoveryWorker) Start(ctx context.Context) {
	slog.Info("recovery worker started", "interval", w.interval)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("recovery worker stopped")
			return
		case <-w.stop:
			slog.Info("recovery worker stopped by stop channel")
			return
		case <-ticker.C:
			if err := w.processRetries(ctx); err != nil {
				slog.Error("recovery worker error", "error", err)
			}
		}
	}
}

// Stop detiene el worker.
func (w *RecoveryWorker) Stop() {
	close(w.stop)
}

// processRetries busca pasos listos para reintentar y los ejecuta.
func (w *RecoveryWorker) processRetries(ctx context.Context) error {
	steps, err := w.stepRepo.GetStepsReadyForRetry(ctx, w.orchestrator.maxRetries)
	if err != nil {
		return err
	}

	if len(steps) == 0 {
		return nil
	}

	slog.Info("recovery worker found steps to retry", "count", len(steps))

	for _, step := range steps {
		// Determinar amount, productID, quantity basado en el paso
		// Por ahora usamos valores por defecto; en una versión futura estos vendrían del contexto de la orden
		amount := 99.99
		productID := 1
		quantity := 1

		if err := w.orchestrator.ExecuteForward(ctx, step.OrderID, amount, productID, quantity); err != nil {
			slog.Warn("recovery worker: step retry failed",
				"step", step.StepName,
				"order", step.OrderID,
				"error", err)
		} else {
			slog.Info("recovery worker: step retried successfully",
				"step", step.StepName,
				"order", step.OrderID)
		}
	}

	return nil
}

// Compile-time check
var _ = domain.SagaStatusPending