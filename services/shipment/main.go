package main

import (
	"context"
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

type Shipment struct {
	ID             int       `json:"id"`
	OrderID        int       `json:"order_id"`
	TrackingNumber string   `json:"tracking_number,omitempty"`
	Status         string    `json:"status"`
	Address        string    `json:"address,omitempty"`
	IdempotencyKey string   `json:"idempotency_key,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type CreateShipmentRequest struct {
	OrderID        int    `json:"order_id"`
	Address        string `json:"address"`
	Method         string `json:"method,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

var db *sql.DB

func main() {
	_ = godotenv.Load()

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8081"
	}

	var err error
	db, err = sql.Open("postgres", buildDSN())
	if err != nil {
		log.Fatalf("error abriendo conexion a DB: %v", err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatalf("error conectando a DB: %v", err)
	}

	if err = initSchema(); err != nil {
		log.Fatalf("error inicializando esquema: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /envíos/{order_id}", getShipment)
	mux.HandleFunc("POST /shipments", createShipment)
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /api/openapi.yaml", openapiHandler)
	mux.HandleFunc("GET /docs", docsHandler)

	log.Printf("servicio shipment escuchando en :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("error iniciando servidor: %v", err)
	}
}

func buildDSN() string {
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
		dbname = "shipment_db"
	}
	sslmode := os.Getenv("DB_SSL_MODE")
	if sslmode == "" {
		sslmode = "disable"
	}

	return "host=" + host + " port=" + port + " user=" + user + " password=" + password + " dbname=" + dbname + " sslmode=" + sslmode
}

func initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS shipments (
		id SERIAL PRIMARY KEY,
		order_id INTEGER NOT NULL,
		tracking_number VARCHAR(100),
		status VARCHAR(20) NOT NULL DEFAULT 'PENDIENTE',
		address TEXT,
		idempotency_key VARCHAR(255) UNIQUE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_shipments_order_id ON shipments(order_id);
	CREATE INDEX IF NOT EXISTS idx_shipments_idempotency_key ON shipments(idempotency_key);

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
	_, err = db.Exec(`ALTER TABLE shipments ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(255)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_shipments_idempotency_key_unique ON shipments(idempotency_key)`)
	if err != nil {
		return err
	}

	return nil
}

func createShipment(w http.ResponseWriter, r *http.Request) {
	log.Printf("POST /shipments - body: %v", r.Body)
	var req CreateShipmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Error decoding request: %v", err)
		http.Error(w, `{"error":"payload invalido"}`, http.StatusBadRequest)
		return
	}
	log.Printf("Creating shipment for order_id=%d address=%s", req.OrderID, req.Address)

	idempotencyKey := req.IdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("shipment-%d-%d", req.OrderID, time.Now().UnixNano())
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, `{"error":"no se pudo iniciar transacción"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// 1. Idempotency: verificar si ya existe un envío con esta clave
	var existingID int
	var existingStatus string
	err = tx.QueryRow(
		`SELECT id, status FROM shipments WHERE idempotency_key = $1 FOR UPDATE`,
		idempotencyKey,
	).Scan(&existingID, &existingStatus)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":           existingID,
			"order_id":     req.OrderID,
			"status":       existingStatus,
			"idempotency_key": idempotencyKey,
			"message":      "shipment already processed (idempotent)",
		})
		return
	}
	if err != sql.ErrNoRows {
		http.Error(w, `{"error":"error verificando idempotencia"}`, http.StatusInternalServerError)
		return
	}

	// 2. Insertar nuevo envío dentro de la misma transacción
	var id int
	err = tx.QueryRow(
		`INSERT INTO shipments (order_id, address, status, idempotency_key) VALUES ($1, $2, $3, $4) RETURNING id`,
		req.OrderID, req.Address, "PENDIENTE", idempotencyKey,
	).Scan(&id)
	if err != nil {
		http.Error(w, `{"error":"no se pudo crear el envio"}`, http.StatusInternalServerError)
		return
	}

	// 3. Escribir evento en outbox dentro de la misma transacción
	payload, _ := json.Marshal(map[string]interface{}{
		"shipment_id":    id,
		"order_id":       req.OrderID,
		"status":         "PENDIENTE",
		"idempotency_key": idempotencyKey,
	})
	_, err = tx.Exec(
		`INSERT INTO outbox (aggregate_type, aggregate_id, event_type, payload, status)
		 VALUES ($1, $2, $3, $4, $5)`,
		"shipment", id, "shipment.created", string(payload), "PENDING",
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
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":           id,
		"order_id":     req.OrderID,
		"status":       "PENDIENTE",
		"idempotency_key": idempotencyKey,
		"message":      "envio creado correctamente",
	})
}

func getShipment(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("order_id")

	var s Shipment
	err := db.QueryRow(
		`SELECT id, order_id, tracking_number, status, address, idempotency_key, created_at FROM shipments WHERE order_id = $1`,
		orderID,
	).Scan(&s.ID, &s.OrderID, &s.TrackingNumber, &s.Status, &s.Address, &s.IdempotencyKey, &s.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, `{"error":"envio no encontrado"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"error consultando envio"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s)
}

func health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy", "db": "disconnected"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "healthy", "db": "connected"})
}

func openapiHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "../api/openapi.yaml")
}

func docsHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "../api/swagger-ui.html")
}
