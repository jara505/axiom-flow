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

type inventoryRemote struct {
	baseURL string
	client  *http.Client
	cb      *CircuitBreaker
}

type inventoryReserveRequest struct {
	OrderID   int    `json:"order_id"`
	ProductID int    `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

type inventoryReleaseRequest struct {
	OrderID   int `json:"order_id"`
	ProductID int `json:"product_id"`
}

type inventoryResponse struct {
	Status  string `json:"status"`
	Key     string `json:"key,omitempty"`
	Message string `json:"message,omitempty"`
}

// NewInventoryRemote creates a remote inventory gateway that calls an external
// service over HTTP. baseURL is the endpoint of the remote inventory service.
// cb is an optional circuit breaker; if nil, calls are always allowed.
func NewInventoryRemote(baseURL string, cb *CircuitBreaker) domain.InventoryOwner {
	return &inventoryRemote{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
		cb:      cb,
	}
}

func (i *inventoryRemote) Reserve(orderID int, productID int, quantity int) error {
	if i.cb != nil && !i.cb.Allow() {
		return fmt.Errorf("circuit breaker open: inventory service temporarily unavailable")
	}

	reqBody, err := json.Marshal(inventoryReserveRequest{
		OrderID:   orderID,
		ProductID: productID,
		Quantity:  quantity,
	})
	if err != nil {
		return fmt.Errorf("marshalling inventory reserve request: %w", err)
	}

	idempotencyKey := fmt.Sprintf("inventory-%d-%d", orderID, productID)
	req, err := http.NewRequest("POST", strings.TrimRight(i.baseURL, "/")+"/inventory/reserve", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("creating inventory reserve request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := i.client.Do(req)
	if err != nil {
		if i.cb != nil {
			i.cb.RecordFailure()
		}
		return fmt.Errorf("calling inventory service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if i.cb != nil {
			i.cb.RecordFailure()
		}
		return fmt.Errorf("inventory service returned %d: %s", resp.StatusCode, string(body))
	}

	if i.cb != nil {
		i.cb.RecordSuccess()
	}
	return nil
}

func (i *inventoryRemote) Release(orderID int) error {
	if i.cb != nil && !i.cb.Allow() {
		return fmt.Errorf("circuit breaker open: inventory service temporarily unavailable")
	}

	reqBody, err := json.Marshal(inventoryReleaseRequest{
		OrderID:   orderID,
		ProductID: 0, // release by order; remote service may ignore product_id if not needed
	})
	if err != nil {
		return fmt.Errorf("marshalling inventory release request: %w", err)
	}

	idempotencyKey := fmt.Sprintf("inventory-release-%d", orderID)
	req, err := http.NewRequest("POST", strings.TrimRight(i.baseURL, "/")+"/inventory/release", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("creating inventory release request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := i.client.Do(req)
	if err != nil {
		if i.cb != nil {
			i.cb.RecordFailure()
		}
		return fmt.Errorf("calling inventory service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if i.cb != nil {
			i.cb.RecordFailure()
		}
		return fmt.Errorf("inventory service returned %d: %s", resp.StatusCode, string(body))
	}

	if i.cb != nil {
		i.cb.RecordSuccess()
	}
	return nil
}

var _ domain.InventoryOwner = (*inventoryRemote)(nil)