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
// db, stepRepo se dejan en nil porque los tests unitarios llaman a los
// handlers de ejecución directa (exec*) que no usan BD.
func newTestOrchestrator(payment domain.PaymentOwner, inventory domain.InventoryOwner) *Orchestrator {
	o := &Orchestrator{
		db:        nil,
		stepRepo:  nil,
		inventory: inventory,
		payment:   payment,
		shipment:  &stubShipmentGateway{},
		maxRetries: 3,
	}
	o.steps = o.buildStepRegistry()
	return o
}

var _ domain.PaymentOwner = (*failingPaymentOwner)(nil)
var _ domain.InventoryOwner = (*stubInventoryOwner)(nil)

// TestExecProcessPayment_FailsReturnsFailedOutcome verifica que cuando el pago falla,
// execProcessPayment devuelve StepOutcomeFailed.
func TestExecProcessPayment_FailsReturnsFailedOutcome(t *testing.T) {
	o := newTestOrchestrator(&failingPaymentOwner{}, &stubInventoryOwner{})

	outcome, err := o.execProcessPayment(context.Background(), 1, 99.99, 1, 1)

	if err == nil {
		t.Fatal("se esperaba un error cuando el pago falla")
	}
	if outcome != domain.StepOutcomeFailed {
		t.Fatalf("se esperaba StepOutcomeFailed, se obtuvo %v", outcome)
	}
}

// TestExecProcessPayment_ApprovedReturnsCompletedOutcome verifica que cuando el pago
// es aprobado por el fake provider, execProcessPayment devuelve StepOutcomeCompleted.
func TestExecProcessPayment_ApprovedReturnsCompletedOutcome(t *testing.T) {
	fakePay := NewFakePaymentProvider()
	fakePay.SetOutcome("order-1-PROCESS_PAYMENT-attempt-1", string(domain.PaymentStatusApproved))

	o := newTestOrchestrator(fakePay, &stubInventoryOwner{})

	outcome, err := o.execProcessPayment(context.Background(), 1, 99.99, 1, 1)

	if err != nil {
		t.Fatalf("no se esperaba error, se obtuvo: %v", err)
	}
	if outcome != domain.StepOutcomeCompleted {
		t.Fatalf("se esperaba StepOutcomeCompleted, se obtuvo %v", outcome)
	}
}

// TestExecReserveInventory_Succeeds verifica que la reserva de inventario
// siempre devuelve completado cuando el dueño no falla.
func TestExecReserveInventory_Succeeds(t *testing.T) {
	o := newTestOrchestrator(&failingPaymentOwner{}, &stubInventoryOwner{})

	outcome, err := o.execReserveInventory(context.Background(), 1, 0, 1, 1)

	if err != nil {
		t.Fatalf("no se esperaba error, se obtuvo: %v", err)
	}
	if outcome != domain.StepOutcomeCompleted {
		t.Fatalf("se esperaba StepOutcomeCompleted, se obtuvo %v", outcome)
	}
}

// TestExecCreateOrder_ReturnsCompleted verifica que crear orden siempre
// devuelve completado (no depende de dueños externos).
func TestExecCreateOrder_ReturnsCompleted(t *testing.T) {
	o := newTestOrchestrator(&failingPaymentOwner{}, &stubInventoryOwner{})

	outcome, err := o.execCreateOrder(context.Background(), 1, 0, 0, 0)

	if err != nil {
		t.Fatalf("no se esperaba error, se obtuvo: %v", err)
	}
	if outcome != domain.StepOutcomeCompleted {
		t.Fatalf("se esperaba StepOutcomeCompleted, se obtuvo %v", outcome)
	}
}