package handler

import (
	"net/http"
	"sync"
	"time"

	"axiom-flow/internal/domain"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"log/slog"
)

// Metrics almacena contadores simples en memoria para observabilidad.
type Metrics struct {
	mu             sync.Mutex
	ordersCreated  int
	ordersFailed   int
	sagaDurationMs []float64
}

// NewMetrics crea una instancia vacía de métricas.
func NewMetrics() *Metrics {
	return &Metrics{
		sagaDurationMs: make([]float64, 0),
	}
}

// IncOrdersCreated incrementa el contador de órdenes creadas.
func (m *Metrics) IncOrdersCreated() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ordersCreated++
}

// IncOrdersFailed incrementa el contador de órdenes fallidas.
func (m *Metrics) IncOrdersFailed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ordersFailed++
}

// RecordSagaDuration registra el tiempo de duración de una saga en milisegundos.
func (m *Metrics) RecordSagaDuration(durationMs float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sagaDurationMs = append(m.sagaDurationMs, durationMs)
}

// Snapshot devuelve una copia de los valores actuales de las métricas.
func (m *Metrics) Snapshot() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	var totalMs float64
	for _, d := range m.sagaDurationMs {
		totalMs += d
	}
	avgMs := 0.0
	if len(m.sagaDurationMs) > 0 {
		avgMs = totalMs / float64(len(m.sagaDurationMs))
	}

	return map[string]interface{}{
		"orders_created":    m.ordersCreated,
		"orders_failed":     m.ordersFailed,
		"saga_count":        len(m.sagaDurationMs),
		"saga_avg_duration_ms": avgMs,
	}
}

// HealthHandler maneja el endpoint de salud y expone métricas.
type HealthHandler struct {
	db      *sqlx.DB
	metrics *Metrics
}

// NewHealthHandler crea una instancia del manejador de salud.
func NewHealthHandler(db *sqlx.DB, metrics *Metrics) *HealthHandler {
	return &HealthHandler{
		db:      db,
		metrics: metrics,
	}
}

// Health responde con el estado del servicio y sus dependencias.
func (h *HealthHandler) Health(c *gin.Context) {
	// Verificar conexión a la base de datos
	dbErr := h.db.Ping()
	dbStatus := "up"
	if dbErr != nil {
		dbStatus = "down"
		slog.Error("database health check failed", "error", dbErr)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    dbStatus,
		"service":   "axiom-flow",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"metrics":   h.metrics.Snapshot(),
	})
}

// Compile-time check
var _ = domain.PaymentStatusPending