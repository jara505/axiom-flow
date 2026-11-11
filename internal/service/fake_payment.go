package service

import (
	"sync"

	"axiom-flow/internal/domain"
)

type FakePaymentProvider struct {
	mu             sync.RWMutex
	outcomes       map[string]string
	defaultOutcome string
	payments       map[string]*domain.Payment
	muPayments     sync.RWMutex
}

func NewFakePaymentProvider() *FakePaymentProvider {
	return &FakePaymentProvider{
		outcomes:       make(map[string]string),
		defaultOutcome: "APPROVED",
		payments:       make(map[string]*domain.Payment),
	}
}

func (f *FakePaymentProvider) SetOutcome(idempotencyKey string, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outcomes[idempotencyKey] = status
}

func (f *FakePaymentProvider) Charge(orderID int, amount float64) (string, error) {
	return "APPROVED", nil
}

func (f *FakePaymentProvider) Refund(orderID int, amount float64) (string, error) {
	return "REFUNDED", nil
}

func (f *FakePaymentProvider) ConsultarPorClave(key string) (*domain.Payment, error) {
	return nil, nil
}

type StubInventoryOwner struct{}

func (s StubInventoryOwner) Reserve(orderID int, productID int, quantity int) error {
	return nil
}

func (s StubInventoryOwner) Release(orderID int) error {
	return nil
}

type StubShipmentGateway struct{}

func (s StubShipmentGateway) Create(orderID int, address string) (string, error) {
	return "created", nil
}
