package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
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

var db *sql.DB

func main() {
	_ = godotenv.Load()

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8083"
	}

	var err error
	db, err = connectDB()
	if err != nil {
		log.Fatalf("error abriendo conexión a DB: %v", err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatalf("error conectando a DB: %v", err)
	}

	if err = initSchema(); err != nil {
		log.Fatalf("error inicializando esquema: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("POST /payments/charge", chargeHandler)
	mux.HandleFunc("POST /payments/refund", refundHandler)
	mux.HandleFunc("GET /payments/{key}", consultHandler)

	log.Printf("servicio de pagos escuchando en :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("error iniciando servidor: %v", err)
	}
}

func connectDB() (*sql.DB, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		host := os.Getenv("DB_HOST")
		if host == "" {
			host = "localhost"
		}
		port := os.Getenv("DB_PORT")
		if port == "" {
			port = "5432"
		}
		user := os.Getenv("DB_USER")
		if user == "" {
			user = "postgres"
		}
		password := os.Getenv("DB_PASSWORD")
		if password == "" {
			password = "postgres"
		}
		dbname := os.Getenv("DB_NAME")
		if dbname == "" {
			dbname = "payment_db"
		}
		sslmode := os.Getenv("DB_SSL_MODE")
		if sslmode == "" {
			sslmode = "disable"
		}
		databaseURL = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
			host, port, user, password, dbname, sslmode)
	}

	return sql.Open("postgres", databaseURL)
}

func initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS payments (
	    id SERIAL PRIMARY KEY,
	    order_id INTEGER NOT NULL,
	    idempotency_key VARCHAR(255) NOT NULL UNIQUE,
	    amount NUMERIC(12,2) NOT NULL,
	    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
	    method VARCHAR(50),
	    provider VARCHAR(50),
	    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
	    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
	    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_payments_order_id ON payments(order_id);
	CREATE INDEX IF NOT EXISTS idx_payments_idempotency_key ON payments(idempotency_key);

	CREATE TABLE IF NOT EXISTS outbox (
	    id SERIAL PRIMARY KEY,
	    aggregate_type VARCHAR(50) NOT NULL,
	    aggregate_id INTEGER NOT NULL,
	    event_type VARCHAR(100) NOT NULL,
	    payload JSONB NOT NULL,
	    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
	    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
	    published_at TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_outbox_status ON outbox(status);
	CREATE INDEX IF NOT EXISTS idx_outbox_created_at ON outbox(created_at);
	`
	_, err := db.Exec(schema)
	return err
}

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

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, `{"error":"no se pudo iniciar transacción"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// 1. Idempotency: verificar si ya existe un pago con esta clave
	var existingID int
	var existingStatus string
	err = tx.QueryRow(
		`SELECT id, status FROM payments WHERE idempotency_key = $1 FOR UPDATE`,
		idempotencyKey,
	).Scan(&existingID, &existingStatus)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ChargeResponse{
			PaymentID:      existingID,
			Status:         existingStatus,
			IdempotencyKey: idempotencyKey,
			Message:        "payment already processed (idempotent)",
		})
		return
	}
	if err != sql.ErrNoRows {
		http.Error(w, `{"error":"error verificando idempotencia"}`, http.StatusInternalServerError)
		return
	}

	// 2. Bloqueo pesimista: verificar si ya existe un pago para esta orden
	err = tx.QueryRow(
		`SELECT id, status FROM payments WHERE order_id = $1 FOR UPDATE`,
		req.OrderID,
	).Scan(&existingID, &existingStatus)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ChargeResponse{
			PaymentID:      existingID,
			Status:         existingStatus,
			IdempotencyKey: idempotencyKey,
			Message:        "payment already exists for this order",
		})
		return
	}
	if err != sql.ErrNoRows {
		http.Error(w, `{"error":"error verificando orden"}`, http.StatusInternalServerError)
		return
	}

	// 3. Insertar nuevo pago dentro de la misma transacción
	var paymentID int
	err = tx.QueryRow(
		`INSERT INTO payments (order_id, idempotency_key, amount, currency, status)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		req.OrderID, idempotencyKey, req.Amount, req.Currency, "APPROVED",
	).Scan(&paymentID)
	if err != nil {
		http.Error(w, `{"error":"no se pudo registrar el pago"}`, http.StatusInternalServerError)
		return
	}

	// 4. Escribir evento en outbox dentro de la misma transacción
	payload, _ := json.Marshal(map[string]interface{}{
		"order_id":        req.OrderID,
		"payment_id":      paymentID,
		"amount":          req.Amount,
		"currency":        req.Currency,
		"status":          "APPROVED",
		"idempotency_key": idempotencyKey,
	})
	_, err = tx.Exec(
		`INSERT INTO outbox (aggregate_type, aggregate_id, event_type, payload, status)
		 VALUES ($1, $2, $3, $4, $5)`,
		"payment", paymentID, "payment.charged", string(payload), "PENDING",
	)
	if err != nil {
		http.Error(w, `{"error":"no se pudo escribir evento en outbox"}`, http.StatusInternalServerError)
		return
	}

	if err = tx.Commit(); err != nil {
		http.Error(w, `{"error":"no se pudo confirmar transacción"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ChargeResponse{
		PaymentID:      paymentID,
		Status:         "APPROVED",
		IdempotencyKey: idempotencyKey,
		Message:        "payment approved",
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

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, `{"error":"no se pudo iniciar transacción"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// 1. Idempotency: verificar si ya existe un reembolso con esta clave
	var existingID int
	var existingStatus string
	err = tx.QueryRow(
		`SELECT id, status FROM payments WHERE idempotency_key = $1 FOR UPDATE`,
		idempotencyKey,
	).Scan(&existingID, &existingStatus)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(RefundResponse{
			PaymentID:      existingID,
			Status:         existingStatus,
			IdempotencyKey: idempotencyKey,
			Message:        "refund already processed (idempotent)",
		})
		return
	}
	if err != sql.ErrNoRows {
		http.Error(w, `{"error":"error verificando idempotencia"}`, http.StatusInternalServerError)
		return
	}

	// 2. Buscar pago aprobado para esta orden con bloqueo pesimista
	var originalID int
	err = tx.QueryRow(
		`SELECT id FROM payments WHERE order_id = $1 AND status = 'APPROVED' FOR UPDATE`,
		req.OrderID,
	).Scan(&originalID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "no approved payment found for order", http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"error buscando pago original"}`, http.StatusInternalServerError)
		return
	}

	// 3. Insertar pago de reembolso dentro de la misma transacción
	var refundID int
	err = tx.QueryRow(
		`INSERT INTO payments (order_id, idempotency_key, amount, currency, status)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		req.OrderID, idempotencyKey, req.Amount, "USD", "REFUNDED",
	).Scan(&refundID)
	if err != nil {
		http.Error(w, `{"error":"no se pudo registrar el reembolso"}`, http.StatusInternalServerError)
		return
	}

	// 4. Marcar el pago original como reembolsado
	_, err = tx.Exec(
		`UPDATE payments SET status = 'REFUNDED', updated_at = CURRENT_TIMESTAMP WHERE id = $1`,
		originalID,
	)
	if err != nil {
		http.Error(w, `{"error":"no se pudo actualizar pago original"}`, http.StatusInternalServerError)
		return
	}

	// 5. Escribir evento en outbox dentro de la misma transacción
	payload, _ := json.Marshal(map[string]interface{}{
		"order_id":        req.OrderID,
		"payment_id":      refundID,
		"original_id":     originalID,
		"amount":          req.Amount,
		"currency":        "USD",
		"status":          "REFUNDED",
		"idempotency_key": idempotencyKey,
	})
	_, err = tx.Exec(
		`INSERT INTO outbox (aggregate_type, aggregate_id, event_type, payload, status)
		 VALUES ($1, $2, $3, $4, $5)`,
		"payment", refundID, "payment.refunded", string(payload), "PENDING",
	)
	if err != nil {
		http.Error(w, `{"error":"no se pudo escribir evento en outbox"}`, http.StatusInternalServerError)
		return
	}

	if err = tx.Commit(); err != nil {
		http.Error(w, `{"error":"no se pudo confirmar transacción"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(RefundResponse{
		PaymentID:      refundID,
		Status:         "REFUNDED",
		IdempotencyKey: idempotencyKey,
		Message:        "refund processed",
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

	var p Payment
	err := db.QueryRow(
		`SELECT id, order_id, idempotency_key, amount, currency, status, created_at, updated_at
		 FROM payments WHERE idempotency_key = $1`,
		idempotencyKey,
	).Scan(&p.ID, &p.OrderID, &p.IdempotencyKey, &p.Amount, &p.Currency, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "payment not found", http.StatusNotFound)
			return
		}
		http.Error(w, "error querying payment", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}
