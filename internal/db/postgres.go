package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Connect opens a connection to PostgreSQL using the DATABASE_URL environment variable.
// If DATABASE_URL is not set, it builds the URL from DB_HOST, DB_PORT, DB_USER,
// DB_PASSWORD, DB_NAME and DB_SSL_MODE (useful under Docker Compose).
func Connect() (*sql.DB, error) {
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
			dbname = "axiom_flow"
		}
		sslmode := os.Getenv("DB_SSL_MODE")
		if sslmode == "" {
			sslmode = "disable"
		}
		databaseURL = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
			user, password, host, port, dbname, sslmode)
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("opening postgres connection: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}

	slog.Info("connected to postgres")
	return db, nil
}