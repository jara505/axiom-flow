package service

import (
	"context"
	"fmt"

	"axiom-flow/internal/domain"
	"axiom-flow/internal/repository"
)

// shipmentLocal implements ShipmentGateway using the local database.
type shipmentLocal struct {
	repo *repository.ShipmentRepository
}

// NewShipmentLocal creates a local shipment gateway backed by SQL.
func NewShipmentLocal(repo *repository.ShipmentRepository) domain.ShipmentGateway {
	return &shipmentLocal{repo: repo}
}

func (s *shipmentLocal) Create(orderID int, address string) (string, error) {
	shipment := &domain.Shipment{
		OrderID:        orderID,
		TrackingNumber: fmt.Sprintf("TRACK-%d", orderID),
		Status:         "SHIPPED",
		Address:        address,
	}
	if err := s.repo.Create(context.Background(), shipment); err != nil {
		return "", fmt.Errorf("creating shipment: %w", err)
	}
	return shipment.TrackingNumber, nil
}

func (s *shipmentLocal) GetByOrder(orderID int) (*domain.Shipment, error) {
	return s.repo.GetByOrder(context.Background(), orderID)
}