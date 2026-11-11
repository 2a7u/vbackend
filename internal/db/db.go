// Package db owns the PostgreSQL connection pool and the idempotent
// auto-migration executed on server startup.
package db

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// Connect creates a pgx connection pool and verifies connectivity.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = 10                       // keep pool small; Railway/Fly instances are single-tenant processes
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Hour         // survive managed-Postgres failovers
	cfg.MaxConnIdleTime = 30 * time.Minute
	cfg.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Migrate applies schema.sql idempotently (CREATE TABLE IF NOT EXISTS ...).
// Executed with no bind args, so pgx uses the simple query protocol which
// supports multi-statement scripts.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	migCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if _, err := pool.Exec(migCtx, schemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}
