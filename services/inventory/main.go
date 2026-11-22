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

type Reservation struct {
	ID          int       `json:"id"`
	OrderID     int       `json:"order_id"`
	ProductID   int       `json:"product_id"`
	Quantity    int       `json:"quantity"`
	Status      string    `json:"status"`
	IdempotencyKey string `json:"idempotency_key"`
	ReservedAt  time.Time `json:"reserved_at"`
	ReleasedAt  *time.Time `json:"released_at,omitempty"`
}

type ReserveRequest struct {
	OrderID        int    `json:"order_id"`
	ProductID      int    `json:"product_id"`
	Quantity       int    `json:"quantity"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type ReleaseRequest struct {
	OrderID        int    `json:"order_id"`
	ProductID      int    `json:"product_id"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type ReserveResponse struct {
	ReservationID  int    `json:"reservation_id"`
	Status         string `json:"status"`
	IdempotencyKey string `json:"idempotency_key"`
	Message        string `json:"message"`
}

type ReleaseResponse struct {
	ReservationID  int    `json:"reservation_id"`
	Status         string `json:"status"`
	IdempotencyKey string `json:"idempotency_key"`
	Message        string `json:"message"`
}

var db *sql.DB

func main() {
	_ = godotenv.Load()

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8082"
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
	mux.HandleFunc("POST /inventory/reserve", reserveHandler)
	mux.HandleFunc("POST /inventory/release", releaseHandler)
	mux.HandleFunc("GET /api/openapi.yaml", openapiHandler)
	mux.HandleFunc("GET /docs", docsHandler)

	log.Printf("servicio de inventario escuchando en :%s", port)
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
			dbname = "inventory_db"
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
	CREATE TABLE IF NOT EXISTS inventory_reservations (
	    id SERIAL PRIMARY KEY,
	    order_id INTEGER NOT NULL,
	    product_id INTEGER NOT NULL,
	    quantity INTEGER NOT NULL,
	    status VARCHAR(20) NOT NULL DEFAULT 'RESERVED',
	    idempotency_key VARCHAR(255) NOT NULL UNIQUE,
	    reserved_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
	    released_at TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_inventory_reservations_order_id ON inventory_reservations(order_id);
	CREATE INDEX IF NOT EXISTS idx_inventory_reservations_product_id ON inventory_reservations(product_id);
	CREATE INDEX IF NOT EXISTS idx_inventory_reservations_idempotency_key ON inventory_reservations(idempotency_key);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_inventory_reservations_order_product_unique ON inventory_reservations(order_id, product_id) WHERE status = 'RESERVED';

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
	if err != nil {
		return err
	}

	// Migración para bases de datos existentes que ya tienen la tabla sin idempotency_key
	_, err = db.Exec(`ALTER TABLE inventory_reservations ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(255)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_inventory_reservations_idempotency_key_unique ON inventory_reservations(idempotency_key)`)
	if err != nil {
		return err
	}

	return nil
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func openapiHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "../api/openapi.yaml")
}

func docsHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "../api/swagger-ui.html")
}

func reserveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ReserveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if req.OrderID <= 0 || req.ProductID <= 0 || req.Quantity <= 0 {
		http.Error(w, "order_id, product_id and quantity are required", http.StatusBadRequest)
		return
	}

	idempotencyKey := req.IdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("inventory-%d-%d-%d", req.OrderID, req.ProductID, time.Now().UnixNano())
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, `{"error":"no se pudo iniciar transacción"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// 1. Idempotency: verificar si ya existe una reserva con esta clave
	var existingID int
	var existingStatus string
	err = tx.QueryRow(
		`SELECT id, status FROM inventory_reservations WHERE idempotency_key = $1 FOR UPDATE`,
		idempotencyKey,
	).Scan(&existingID, &existingStatus)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ReserveResponse{
			ReservationID:  existingID,
			Status:         existingStatus,
			IdempotencyKey: idempotencyKey,
			Message:        "reservation already processed (idempotent)",
		})
		return
	}
	if err != sql.ErrNoRows {
		http.Error(w, `{"error":"error verificando idempotencia"}`, http.StatusInternalServerError)
		return
	}

	// 2. Bloqueo pesimista: verificar si ya existe una reserva activa para esta orden y producto
	err = tx.QueryRow(
		`SELECT id, status FROM inventory_reservations WHERE order_id = $1 AND product_id = $2 AND status = 'RESERVED' FOR UPDATE`,
		req.OrderID, req.ProductID,
	).Scan(&existingID, &existingStatus)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(ReserveResponse{
			ReservationID:  existingID,
			Status:         existingStatus,
			IdempotencyKey: idempotencyKey,
			Message:        "inventory already reserved for this order and product",
		})
		return
	}
	if err != sql.ErrNoRows {
		http.Error(w, `{"error":"error verificando reserva existente"}`, http.StatusInternalServerError)
		return
	}

	// 3. Insertar nueva reserva dentro de la misma transacción
	var reservationID int
	err = tx.QueryRow(
		`INSERT INTO inventory_reservations (order_id, product_id, quantity, status, idempotency_key)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		req.OrderID, req.ProductID, req.Quantity, "RESERVED", idempotencyKey,
	).Scan(&reservationID)
	if err != nil {
		http.Error(w, `{"error":"no se pudo crear la reserva de inventario"}`, http.StatusInternalServerError)
		return
	}

	// 4. Escribir evento en outbox dentro de la misma transacción
	payload, _ := json.Marshal(map[string]interface{}{
		"reservation_id": reservationID,
		"order_id":       req.OrderID,
		"product_id":     req.ProductID,
		"quantity":       req.Quantity,
		"status":         "RESERVED",
		"idempotency_key": idempotencyKey,
	})
	_, err = tx.Exec(
		`INSERT INTO outbox (aggregate_type, aggregate_id, event_type, payload, status)
		 VALUES ($1, $2, $3, $4, $5)`,
		"inventory", reservationID, "inventory.reserved", string(payload), "PENDING",
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
	json.NewEncoder(w).Encode(ReserveResponse{
		ReservationID:  reservationID,
		Status:         "RESERVED",
		IdempotencyKey: idempotencyKey,
		Message:        "inventory reserved",
	})
}

func releaseHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ReleaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if req.OrderID <= 0 || req.ProductID <= 0 {
		http.Error(w, "order_id and product_id are required", http.StatusBadRequest)
		return
	}

	idempotencyKey := req.IdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("inventory-release-%d-%d-%d", req.OrderID, req.ProductID, time.Now().UnixNano())
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, `{"error":"no se pudo iniciar transacción"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// 1. Idempotency: verificar si ya existe una liberación con esta clave
	var existingID int
	var existingStatus string
	err = tx.QueryRow(
		`SELECT id, status FROM inventory_reservations WHERE idempotency_key = $1 FOR UPDATE`,
		idempotencyKey,
	).Scan(&existingID, &existingStatus)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ReleaseResponse{
			ReservationID:  existingID,
			Status:         existingStatus,
			IdempotencyKey: idempotencyKey,
			Message:        "release already processed (idempotent)",
		})
		return
	}
	if err != sql.ErrNoRows {
		http.Error(w, `{"error":"error verificando idempotencia"}`, http.StatusInternalServerError)
		return
	}

	// 2. Bloqueo pesimista: buscar reserva activa para esta orden y producto
	var reservationID int
	err = tx.QueryRow(
		`SELECT id FROM inventory_reservations WHERE order_id = $1 AND product_id = $2 AND status = 'RESERVED' FOR UPDATE`,
		req.OrderID, req.ProductID,
	).Scan(&reservationID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "no active reservation found for this order and product", http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"error buscando reserva activa"}`, http.StatusInternalServerError)
		return
	}

	// 3. Marcar reserva como liberada dentro de la misma transacción
	_, err = tx.Exec(
		`UPDATE inventory_reservations SET status = 'RELEASED', released_at = CURRENT_TIMESTAMP, idempotency_key = $1 WHERE id = $2`,
		idempotencyKey, reservationID,
	)
	if err != nil {
		http.Error(w, `{"error":"no se pudo liberar la reserva"}`, http.StatusInternalServerError)
		return
	}

	// 4. Escribir evento en outbox dentro de la misma transacción
	payload, _ := json.Marshal(map[string]interface{}{
		"reservation_id": reservationID,
		"order_id":       req.OrderID,
		"product_id":     req.ProductID,
		"status":         "RELEASED",
		"idempotency_key": idempotencyKey,
	})
	_, err = tx.Exec(
		`INSERT INTO outbox (aggregate_type, aggregate_id, event_type, payload, status)
		 VALUES ($1, $2, $3, $4, $5)`,
		"inventory", reservationID, "inventory.released", string(payload), "PENDING",
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
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ReleaseResponse{
		ReservationID:  reservationID,
		Status:         "RELEASED",
		IdempotencyKey: idempotencyKey,
		Message:        "inventory released",
	})
}
