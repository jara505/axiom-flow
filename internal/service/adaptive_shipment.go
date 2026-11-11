package service

import (
	"fmt"
	"os"
	"time"

	"axiom-flow/internal/domain"
	"axiom-flow/internal/repository"

	"github.com/jmoiron/sqlx"
)

// AdaptiveShipmentGateway decide en tiempo de ejecución si usar el envío local o remoto,
// basándose en el estado del CircuitBreaker. No depende de variables de entorno para elegir.
type AdaptiveShipmentGateway struct {
	local  domain.ShipmentGateway
	remote domain.ShipmentGateway
	cb     *CircuitBreaker
}

// NewAdaptiveShipmentGateway crea un adaptador que intenta usar el servicio remoto mientras el
// circuito esté cerrado o medio abierto, y falla al local si el circuito está abierto o el remoto
// devuelve error.
func NewAdaptiveShipmentGateway(local domain.ShipmentGateway, remote domain.ShipmentGateway, cb *CircuitBreaker) *AdaptiveShipmentGateway {
	return &AdaptiveShipmentGateway{
		local:  local,
		remote: remote,
		cb:     cb,
	}
}

// Create crea un envío usando el servicio remoto si el circuito lo permite, o el servicio local
// como respaldo cuando el remoto está fallando o el circuito está abierto.
func (a *AdaptiveShipmentGateway) Create(orderID int, address string) (string, error) {
	// Si el circuito permite intento remoto, lo intentamos primero.
	if a.cb.Allow() {
		tracking, err := a.remote.Create(orderID, address)
		if err == nil {
			return tracking, nil
		}
		// Remoto falló: registramos la falla y caemos a local.
		a.cb.RecordFailure()
	}

	// Fallback a implementación local (siempre disponible, sin dependencia de red externa).
	tracking, err := a.local.Create(orderID, address)
	if err != nil {
		return "", fmt.Errorf("both remote and local shipment failed: remote: %w; local: %w", err, err)
	}
	return tracking, nil
}

var _ domain.ShipmentGateway = (*AdaptiveShipmentGateway)(nil)

// GetByOrder obtiene el envío de un orden. Intenta local primero; si no existe, intenta remoto.
func (a *AdaptiveShipmentGateway) GetByOrder(orderID int) (*domain.Shipment, error) {
	s, err := a.local.GetByOrder(orderID)
	if err == nil && s != nil {
		return s, nil
	}
	return a.remote.GetByOrder(orderID)
}

// NewAdaptiveShipmentGatewayFromEnv construye el adaptador completo sin depender de
// SHIPMENT_MODE para decidir el comportamiento. Siempre se crean ambas implementaciones y el
// CircuitBreaker decide en runtime cuál usar.
func NewAdaptiveShipmentGatewayFromEnv(db *sqlx.DB) (*AdaptiveShipmentGateway, error) {
	localRepo := repository.NewShipmentRepository(db)
	local := NewShipmentLocal(localRepo)

	remoteURL := getEnv("SHIPMENT_REMOTE_URL", "http://localhost:8081")
	cb := NewCircuitBreaker(3, 30*time.Second, 30*time.Second)
	remote := NewShipmentRemote(remoteURL, cb)

	return NewAdaptiveShipmentGateway(local, remote, cb), nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}