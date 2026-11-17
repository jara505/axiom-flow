package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"axiom-flow/internal/domain"
)

type shipmentRemote struct {
	baseURL string
	client  *http.Client
}

type shipmentCreateRequest struct {
	OrderID        int    `json:"order_id"`
	Address        string `json:"address"`
	IdempotencyKey string `json:"idempotency_key"`
}

type shipmentCreateResponse struct {
	TrackingNumber string `json:"tracking_number"`
	Status         string `json:"status"`
}

// NewShipmentRemote creates a remote shipment gateway that calls an external
// service over HTTP. baseURL is the endpoint of the remote shipment service.
func NewShipmentRemote(baseURL string) domain.ShipmentGateway {
	return &shipmentRemote{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *shipmentRemote) Create(orderID int, address string) (string, error) {
	reqBody, err := json.Marshal(shipmentCreateRequest{
		OrderID:        orderID,
		Address:        address,
		IdempotencyKey: fmt.Sprintf("shipment-%d", orderID),
	})
	if err != nil {
		return "", fmt.Errorf("marshalling shipment request: %w", err)
	}

	req, err := http.NewRequest("POST", strings.TrimRight(s.baseURL, "/")+"/shipments", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("creating shipment request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", fmt.Sprintf("shipment-%d", orderID))

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling shipment service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("shipment service returned %d: %s", resp.StatusCode, string(body))
	}

	var result shipmentCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding shipment response: %w", err)
	}

	return result.TrackingNumber, nil
}

func (s *shipmentRemote) GetByOrder(orderID int) (*domain.Shipment, error) {
	url := strings.TrimRight(s.baseURL, "/") + fmt.Sprintf("/shipments/%d", orderID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating shipment get request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling shipment service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("shipment service returned %d: %s", resp.StatusCode, string(body))
	}

	var result domain.Shipment
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding shipment response: %w", err)
	}

	return &result, nil
}