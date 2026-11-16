package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"axiom-flow/internal/domain"
)

type paymentRemote struct {
	baseURL string
	client  *http.Client
}

type chargeRequest struct {
	OrderID        int     `json:"order_id"`
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency,omitempty"`
	IdempotencyKey string  `json:"idempotency_key,omitempty"`
}

type chargeResponse struct {
	PaymentID      int    `json:"payment_id"`
	Status         string `json:"status"`
	IdempotencyKey string `json:"idempotency_key"`
	Message        string `json:"message"`
}

type refundRequest struct {
	OrderID        int     `json:"order_id"`
	Amount         float64 `json:"amount"`
	IdempotencyKey string  `json:"idempotency_key,omitempty"`
}

type refundResponse struct {
	PaymentID      int    `json:"payment_id"`
	Status         string `json:"status"`
	IdempotencyKey string `json:"idempotency_key"`
	Message        string `json:"message"`
}

type paymentResponse struct {
	ID             int       `json:"id"`
	OrderID        int       `json:"order_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Amount         float64   `json:"amount"`
	Currency       string    `json:"currency"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// NewPaymentRemote creates a remote payment gateway that calls an external
// service over HTTP. baseURL is the endpoint of the remote payment service.
func NewPaymentRemote(baseURL string) domain.PaymentOwner {
	return &paymentRemote{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *paymentRemote) Charge(orderID int, amount float64) (string, error) {
	ctx := context.Background()
	idempotencyKey := fmt.Sprintf("charge-%d", orderID)

	reqBody, err := json.Marshal(chargeRequest{
		OrderID:        orderID,
		Amount:         amount,
		Currency:       "USD",
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return "", fmt.Errorf("marshalling charge request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(p.baseURL, "/")+"/payments/charge", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("creating charge request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling payment service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("payment service returned %d: %s", resp.StatusCode, string(body))
	}

	var result chargeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding charge response: %w", err)
	}

	return result.Status, nil
}

func (p *paymentRemote) Refund(orderID int, amount float64) (string, error) {
	ctx := context.Background()
	idempotencyKey := fmt.Sprintf("refund-%d", orderID)

	reqBody, err := json.Marshal(refundRequest{
		OrderID:        orderID,
		Amount:         amount,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return "", fmt.Errorf("marshalling refund request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(p.baseURL, "/")+"/payments/refund", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("creating refund request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling payment service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("payment service returned %d: %s", resp.StatusCode, string(body))
	}

	var result refundResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding refund response: %w", err)
	}

	return result.Status, nil
}

func (p *paymentRemote) ConsultarPorClave(key string) (*domain.Payment, error) {
	ctx := context.Background()

	url := strings.TrimRight(p.baseURL, "/") + "/payments/" + key
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating consult request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling payment service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("payment service returned %d: %s", resp.StatusCode, string(body))
	}

	var result paymentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding payment response: %w", err)
	}

	return &domain.Payment{
		ID:             result.ID,
		OrderID:        result.OrderID,
		IdempotencyKey: result.IdempotencyKey,
		Amount:         result.Amount,
		Currency:       result.Currency,
		Status:         domain.PaymentStatus(result.Status),
		CreatedAt:      result.CreatedAt,
	}, nil
}

var _ domain.PaymentOwner = (*paymentRemote)(nil)