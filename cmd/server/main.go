package main

import (
	"log/slog"
	"os"

	"axiom-flow/internal/db"
	"axiom-flow/internal/handler"
	"axiom-flow/internal/repository"
	"axiom-flow/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/jmoiron/sqlx"
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

	paypalPayment := service.NewPayPalSandboxProvider()

	orchestrator := service.NewOrchestrator(
		dbx,
		stepRepo,
		service.StubInventoryOwner{},
		paypalPayment,
		service.StubShipmentOwner{},
	)

	orderHandler := handler.NewOrderHandler(orchestrator, dbx, stepRepo)

	r := gin.Default()
	r.POST("/orders", orderHandler.CreateOrder)
	r.GET("/orders/:id/saga", orderHandler.GetSaga)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	slog.Info("server starting", "port", port)
	if err := r.Run(":" + port); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}