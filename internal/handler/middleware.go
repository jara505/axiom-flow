package handler

import (
	"fmt"
	"net/http"
	"time"

	"axiom-flow/internal/domain"
	"axiom-flow/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"log/slog"
	"github.com/google/uuid"
)

// IdempotencyMiddleware valida que una clave de idempotencia no haya sido procesada previamente.
type IdempotencyMiddleware struct {
	stepRepo    *repository.SagaStepRepository
	paymentRepo *repository.PaymentRepository
	db          *sqlx.DB
}

// NewIdempotencyMiddleware crea el middleware de idempotencia.
func NewIdempotencyMiddleware(db *sqlx.DB, stepRepo *repository.SagaStepRepository, paymentRepo *repository.PaymentRepository) *IdempotencyMiddleware {
	return &IdempotencyMiddleware{
		stepRepo:    stepRepo,
		paymentRepo: paymentRepo,
		db:          db,
	}
}

// Middleware retorna el handler de Gin para validar idempotencia.
func (m *IdempotencyMiddleware) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("Idempotency-Key")
		if key == "" {
			c.Next()
			return
		}

		ctx := c.Request.Context()

		// Verificar si ya existe un saga_step con esta clave
		existing, err := m.stepRepo.GetByOrderAndStepName(ctx, m.db, 0, key)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("checking idempotency: %v", err)})
			c.Abort()
			return
		}
		if existing != nil {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "idempotency key already processed",
				"key":     key,
				"step":    existing.StepName,
				"status":  existing.Status,
			})
			c.Abort()
			return
		}

		// Verificar si ya existe un pago con esta clave
		existingPayment, err := m.paymentRepo.GetByIdempotencyKey(ctx, key)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("checking payment idempotency: %v", err)})
			c.Abort()
			return
		}
		if existingPayment != nil {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "idempotency key already processed",
				"key":     key,
				"resource": "payment",
				"status":   existingPayment.Status,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequestIDMiddleware genera o recupera un ID único por petición y lo inyecta en los logs.
type RequestIDMiddleware struct{}

func NewRequestIDMiddleware() *RequestIDMiddleware {
	return &RequestIDMiddleware{}
}

// Middleware agrega el header X-Request-ID y lo incluye en los logs de la petición.
func (m *RequestIDMiddleware) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)

		start := time.Now()

		c.Next()

		duration := time.Since(start)

		slog.Info("request completed",
			"request_id", requestID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", duration.Milliseconds(),
		)
	}
}

// Compile-time check
var _ = domain.PaymentStatusPending