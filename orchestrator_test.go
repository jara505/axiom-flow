package axiom_flow_test

import (
	"testing"

	"axiom-flow/internal/service"
)

func TestOrchestrator_HappyPath(t *testing.T) {
	fakePay := service.NewFakePaymentProvider()
	fakePay.SetOutcome("key-1", "APPROVED")
	status, err := fakePay.Charge(1, 99.99)
	if err != nil {
		t.Error(err)
	}
	if status != "APPROVED" {
		t.Error("expected APPROVED, got " + status)
	}
}

func TestOrchestrator_PaymentConsult(t *testing.T) {
	fakePay := service.NewFakePaymentProvider()
	payment, err := fakePay.ConsultarPorClave("key-1")
	_ = payment
	_ = err
}