package handler

import (
	"fmt"
	"net/http"
	"time"

	"axiom-flow/internal/domain"
	"axiom-flow/internal/repository"
	"axiom-flow/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// OrderHandler manages HTTP routes for orders.
type OrderHandler struct {
	orchestrator *service.Orchestrator
	db           *sqlx.DB
	stepRepo     *repository.SagaStepRepository
	metrics      *Metrics
}

// NewOrderHandler creates a new OrderHandler instance.
func NewOrderHandler(orchestrator *service.Orchestrator, db *sqlx.DB, stepRepo *repository.SagaStepRepository, metrics *Metrics) *OrderHandler {
	return &OrderHandler{
		orchestrator: orchestrator,
		db:           db,
		stepRepo:     stepRepo,
		metrics:      metrics,
	}
}

// CreateOrderRequest is the body of the POST /orders request.
type CreateOrderRequest struct {
	CustomerID    int     `json:"customer_id" binding:"required"`
	OrderNumber   string  `json:"order_number" binding:"required"`
	Total         float64 `json:"total" binding:"required,gt=0"`
	ProductID     int     `json:"product_id" binding:"required,gt=0"`
	Quantity      int     `json:"quantity" binding:"required,gt=0"`
	IdempotencyKey string `json:"idempotency_key" binding:"required"`
}

// CreateOrder creates a new order and starts the Saga forward execution.
func (h *OrderHandler) CreateOrder(c *gin.Context) {
	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var orderID int
	query := `INSERT INTO orders (customer_id, order_number, idempotency_key, status, subtotal, total)
	          VALUES ($1, $2, $3, $4, $5, $6)
	          RETURNING id`
	err := h.db.QueryRowContext(c.Request.Context(), query,
		req.CustomerID, req.OrderNumber, req.IdempotencyKey, "PENDING_PAYMENT", req.Total, req.Total).
		Scan(&orderID)
	if err != nil {
		h.metrics.IncOrdersFailed()
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	start := time.Now()

	if err := h.orchestrator.ExecuteForward(c.Request.Context(), orderID, req.Total, req.ProductID, req.Quantity); err != nil {
		h.metrics.IncOrdersFailed()
		durationMs := float64(time.Since(start).Milliseconds())
		h.metrics.RecordSagaDuration(durationMs)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "order_id": orderID})
		return
	}

	durationMs := float64(time.Since(start).Milliseconds())
	h.metrics.IncOrdersCreated()
	h.metrics.RecordSagaDuration(durationMs)

	c.JSON(http.StatusCreated, gin.H{"order_id": orderID, "status": "saga_started"})
}

// GetSaga returns all saga steps of an order.
func (h *OrderHandler) GetSaga(c *gin.Context) {
	var orderID int
	if _, err := fmt.Sscanf(c.Param("id"), "%d", &orderID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	steps, err := h.stepRepo.ListByOrder(c.Request.Context(), orderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if steps == nil {
		steps = []*domain.SagaStep{}
	}

	c.JSON(http.StatusOK, gin.H{
		"order_id": orderID,
		"steps":    steps,
	})
}