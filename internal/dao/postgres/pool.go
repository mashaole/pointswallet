package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"pointswallet/internal/config"
)

func OpenPool(cfg config.Config) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return db, nil
}

func RunMigrations(db *sql.DB) error {
	sqlBytes, err := os.ReadFile("migrations/001_init.sql")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	if _, err := db.Exec(string(sqlBytes)); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
