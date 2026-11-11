package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"axiom-flow/internal/db"
	"axiom-flow/internal/handler"
	"axiom-flow/internal/repository"
	"axiom-flow/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/jmoiron/sqlx"

	"axiom-flow/internal/handler/ratelimit"
)

func main() {
	_ = godotenv.Load()

	sqlDB, err := db.Connect()
	if err != nil {
		slog.Error("cannot connect to database", "error", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	dbx := sqlx.NewDb(sqlDB, "pgx")

	stepRepo := repository.NewSagaStepRepository(dbx)
	paymentRepo := repository.NewPaymentRepository(dbx)

	metrics := handler.NewMetrics()

	fakePayment := service.NewFakePaymentProvider()

	shipment, err := service.NewAdaptiveShipmentGatewayFromEnv(dbx)
	if err != nil {
		slog.Error("cannot create shipment gateway", "error", err)
		os.Exit(1)
	}

	orchestrator := service.NewOrchestrator(
		dbx,
		stepRepo,
		service.StubInventoryOwner{},
		fakePayment,
		shipment,
		3, // maxRetries
	)

	// Recovery worker para reintentar pasos fallidos por timeout
	recoveryWorker := service.NewRecoveryWorker(dbx, stepRepo, orchestrator, 10*time.Second)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go recoveryWorker.Start(ctx)

	// Middleware de idempotencia
	idempotencyMiddleware := handler.NewIdempotencyMiddleware(dbx, stepRepo, paymentRepo)

	// Middleware de request ID y logging estructurado
	requestIDMiddleware := handler.NewRequestIDMiddleware()

	// Rate limiting: 60 peticiones por minuto por IP (in-memory, sin dependencias externas)
	rateLimiter := ratelimit.NewInMemoryRateLimiter(60, 1*time.Minute)

	// Health handler con métricas
	healthHandler := handler.NewHealthHandler(dbx, metrics)

	orderHandler := handler.NewOrderHandler(orchestrator, dbx, stepRepo, metrics)

	r := gin.Default()
	r.Use(requestIDMiddleware.Middleware())
	r.Use(idempotencyMiddleware.Middleware())
	r.Use(ratelimit.RateLimitMiddleware(rateLimiter))
	r.GET("/health", healthHandler.Health)
	r.POST("/orders", orderHandler.CreateOrder)
	r.GET("/orders/:id/saga", orderHandler.GetSaga)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	// Graceful shutdown: esperar a que terminen peticiones en curso
	go func() {
		<-ctx.Done()
		slog.Info("shutting down server gracefully...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown error", "error", err)
		}
	}()

	slog.Info("server starting", "port", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}