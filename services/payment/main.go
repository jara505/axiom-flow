package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

type Payment struct {
	ID             int       `json:"id"`
	OrderID        int       `json:"order_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Amount         float64   `json:"amount"`
	Currency       string    `json:"currency"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ChargeRequest struct {
	OrderID        int     `json:"order_id"`
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency,omitempty"`
	IdempotencyKey string  `json:"idempotency_key,omitempty"`
}

type RefundRequest struct {
	OrderID        int     `json:"order_id"`
	Amount         float64 `json:"amount"`
	IdempotencyKey string  `json:"idempotency_key,omitempty"`
}

type ChargeResponse struct {
	PaymentID      int    `json:"payment_id"`
	Status         string `json:"status"`
	IdempotencyKey string `json:"idempotency_key"`
	Message        string `json:"message"`
}

type RefundResponse struct {
	PaymentID      int    `json:"payment_id"`
	Status         string `json:"status"`
	IdempotencyKey string `json:"idempotency_key"`
	Message        string `json:"message"`
}

var (
	mu       sync.RWMutex
	payments = make(map[string]*Payment)
	nextID   = 1
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func chargeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChargeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if req.OrderID <= 0 || req.Amount <= 0 {
		http.Error(w, "order_id and amount are required", http.StatusBadRequest)
		return
	}

	if req.Currency == "" {
		req.Currency = "USD"
	}

	idempotencyKey := req.IdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("charge-%d-%d", req.OrderID, time.Now().UnixNano())
	}

	mu.Lock()
	defer mu.Unlock()

	if existing, ok := payments[idempotencyKey]; ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ChargeResponse{
			PaymentID:      existing.ID,
			Status:         existing.Status,
			IdempotencyKey: idempotencyKey,
			Message:        "payment already processed (idempotent)",
		})
		return
	}

	payment := &Payment{
		ID:             nextID,
		OrderID:        req.OrderID,
		IdempotencyKey: idempotencyKey,
		Amount:         req.Amount,
		Currency:       req.Currency,
		Status:         "APPROVED",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	nextID++
	payments[idempotencyKey] = payment

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ChargeResponse{
		PaymentID:      payment.ID,
		Status:         payment.Status,
		IdempotencyKey: idempotencyKey,
		Message:        "payment approved (simulated)",
	})
}

func refundHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RefundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if req.OrderID <= 0 || req.Amount <= 0 {
		http.Error(w, "order_id and amount are required", http.StatusBadRequest)
		return
	}

	idempotencyKey := req.IdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("refund-%d-%d", req.OrderID, time.Now().UnixNano())
	}

	mu.Lock()
	defer mu.Unlock()

	if existing, ok := payments[idempotencyKey]; ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(RefundResponse{
			PaymentID:      existing.ID,
			Status:         existing.Status,
			IdempotencyKey: idempotencyKey,
			Message:        "refund already processed (idempotent)",
		})
		return
	}

	var originalPayment *Payment
	for _, p := range payments {
		if p.OrderID == req.OrderID && p.Status == "APPROVED" {
			originalPayment = p
			break
		}
	}

	if originalPayment == nil {
		http.Error(w, "no approved payment found for order", http.StatusNotFound)
		return
	}

	refundPayment := &Payment{
		ID:             nextID,
		OrderID:        req.OrderID,
		IdempotencyKey: idempotencyKey,
		Amount:         req.Amount,
		Currency:       originalPayment.Currency,
		Status:         "REFUNDED",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	nextID++
	payments[idempotencyKey] = refundPayment

	originalPayment.Status = "REFUNDED"
	originalPayment.UpdatedAt = time.Now()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(RefundResponse{
		PaymentID:      refundPayment.ID,
		Status:         refundPayment.Status,
		IdempotencyKey: idempotencyKey,
		Message:        "refund processed (simulated)",
	})
}

func consultHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idempotencyKey := r.PathValue("key")
	if idempotencyKey == "" {
		http.Error(w, "idempotency key required", http.StatusBadRequest)
		return
	}

	mu.RLock()
	payment, ok := payments[idempotencyKey]
	mu.RUnlock()

	if !ok {
		http.Error(w, "payment not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payment)
}

func main() {
	port := "8083"

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("POST /payments/charge", chargeHandler)
	mux.HandleFunc("POST /payments/refund", refundHandler)
	mux.HandleFunc("GET /payments/{key}", consultHandler)

	log.Printf("Payment service listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}