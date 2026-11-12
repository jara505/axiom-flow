package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type Shipment struct {
	ID           int       `json:"id"`
	OrderID      int       `db:"order_id" json:"order_id"`
	TrackingNumber string   `db:"tracking_number" json:"tracking_number,omitempty"`
	Status       string    `db:"status" json:"status"`
	Address      string    `db:"address" json:"address,omitempty"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}

type CreateShipmentRequest struct {
	OrderID    int    `json:"order_id"`
	Address    string `json:"address"`
	Method     string `json:"method,omitempty"`
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
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_shipments_order_id ON shipments(order_id);
	`
	_, err := db.Exec(schema)
	return err
}

func createShipment(w http.ResponseWriter, r *http.Request) {
	var req CreateShipmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"payload invalido"}`, http.StatusBadRequest)
		return
	}

	var id int
	err := db.QueryRow(
		`INSERT INTO shipments (order_id, address, status) VALUES ($1, $2, $3) RETURNING id`,
		req.OrderID, req.Address, "PENDIENTE",
	).Scan(&id)
	if err != nil {
		http.Error(w, `{"error":"no se pudo crear el envio"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         id,
		"order_id":   req.OrderID,
		"status":     "PENDIENTE",
		"message":    "envio creado correctamente",
	})
}

func getShipment(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("order_id")

	var s Shipment
	err := db.QueryRow(
		`SELECT id, order_id, tracking_number, status, address, created_at FROM shipments WHERE order_id = $1`,
		orderID,
	).Scan(&s.ID, &s.OrderID, &s.TrackingNumber, &s.Status, &s.Address, &s.CreatedAt)
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