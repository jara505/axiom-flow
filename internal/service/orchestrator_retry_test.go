package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"axiom-flow/internal/domain"
)

// failingPaymentOwner siempre devuelve error en Charge para simular fallo transitorio.
type failingPaymentOwner struct{}

func (f *failingPaymentOwner) Charge(orderID int, amount float64) (string, error) {
	return "", errors.New("pago rechazado por timeout simulado")
}

func (f *failingPaymentOwner) Refund(orderID int, amount float64) (string, error) {
	return "", nil
}

func (f *failingPaymentOwner) ConsultarPorClave(key string) (*domain.Payment, error) {
	return nil, nil
}

// stubInventoryOwner nunca falla en Reserve.
type stubInventoryOwner struct{}

func (s *stubInventoryOwner) Reserve(orderID int, productID int, quantity int) error {
	return nil
}

func (s *stubInventoryOwner) Release(orderID int) error {
	return nil
}

// stubShipmentGateway nunca falla en Create para los tests de unidad.
type stubShipmentGateway struct{}

func (s *stubShipmentGateway) Create(orderID int, address string) (string, error) {
	return "track-" + fmt.Sprintf("%d", orderID), nil
}

func (s *stubShipmentGateway) GetByOrder(orderID int) (*domain.Shipment, error) {
	return nil, nil
}

// newTestOrchestrator crea un Orchestrator con los dueños proporcionados.
// db, stepRepo y shipmentRepo se dejan en nil porque executeStep no los usa
// en los pasos probados (RESERVE_INVENTORY, CREATE_ORDER, PROCESS_PAYMENT).
func newTestOrchestrator(payment domain.PaymentOwner, inventory domain.InventoryOwner) *Orchestrator {
	return &Orchestrator{
		db:        nil,
		stepRepo:  nil,
		inventory: inventory,
		payment:   payment,
		shipment:  &stubShipmentGateway{},
		maxRetries: 3,
	}
}

var _ domain.PaymentOwner = (*failingPaymentOwner)(nil)
var _ domain.InventoryOwner = (*stubInventoryOwner)(nil)

// TestExecuteStep_PaymentFailsReturnsFailedOutcome verifica que cuando el pago falla,
// executeStep devuelve StepOutcomeFailed sin compensar (la compensación la maneja el orquestador).
func TestExecuteStep_PaymentFailsReturnsFailedOutcome(t *testing.T) {
	o := newTestOrchestrator(&failingPaymentOwner{}, &stubInventoryOwner{})

	outcome, err := o.executeStep(context.Background(), "PROCESS_PAYMENT", 1, 99.99, 1, 1)

	if err == nil {
		t.Fatal("se esperaba un error cuando el pago falla")
	}
	if outcome != domain.StepOutcomeFailed {
		t.Fatalf("se esperaba StepOutcomeFailed, se obtuvo %v", outcome)
	}
}

// TestExecuteStep_PaymentApprovedReturnsCompletedOutcome verifica que cuando el pago
// es aprobado por el fake provider, executeStep devuelve StepOutcomeCompleted.
func TestExecuteStep_PaymentApprovedReturnsCompletedOutcome(t *testing.T) {
	fakePay := NewFakePaymentProvider()
	fakePay.SetOutcome("order-1-PROCESS_PAYMENT-attempt-1", string(domain.PaymentStatusApproved))

	o := newTestOrchestrator(fakePay, &stubInventoryOwner{})

	outcome, err := o.executeStep(context.Background(), "PROCESS_PAYMENT", 1, 99.99, 1, 1)

	if err != nil {
		t.Fatalf("no se esperaba error, se obtuvo: %v", err)
	}
	if outcome != domain.StepOutcomeCompleted {
		t.Fatalf("se esperaba StepOutcomeCompleted, se obtuvo %v", outcome)
	}
}

// TestExecuteStep_ReserveInventorySucceeds verifica que la reserva de inventario
// siempre devuelve completado cuando el dueño no falla.
func TestExecuteStep_ReserveInventorySucceeds(t *testing.T) {
	o := newTestOrchestrator(&failingPaymentOwner{}, &stubInventoryOwner{})

	outcome, err := o.executeStep(context.Background(), "RESERVE_INVENTORY", 1, 0, 1, 1)

	if err != nil {
		t.Fatalf("no se esperaba error, se obtuvo: %v", err)
	}
	if outcome != domain.StepOutcomeCompleted {
		t.Fatalf("se esperaba StepOutcomeCompleted, se obtuvo %v", outcome)
	}
}

// TestExecuteStep_CreateOrderReturnsCompleted verifica que crear orden siempre
// devuelve completado (no depende de dueños externos).
func TestExecuteStep_CreateOrderReturnsCompleted(t *testing.T) {
	o := newTestOrchestrator(&failingPaymentOwner{}, &stubInventoryOwner{})

	outcome, err := o.executeStep(context.Background(), "CREATE_ORDER", 1, 0, 0, 0)

	if err != nil {
		t.Fatalf("no se esperaba error, se obtuvo: %v", err)
	}
	if outcome != domain.StepOutcomeCompleted {
		t.Fatalf("se esperaba StepOutcomeCompleted, se obtuvo %v", outcome)
	}
}