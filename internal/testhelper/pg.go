package testhelper

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	embeddedDB   *embeddedpostgres.EmbeddedPostgres
	embeddedOnce sync.Once
	embeddedErr  error
	embeddedURL  = "postgres://venus_test:venus_test_pwd@127.0.0.1:9876/venus_test_db?sslmode=disable"

	sharedPool *pgxpool.Pool
	poolOnce   sync.Once
	poolErr    error
)

// GetTestPool connects to a real PostgreSQL instance (either TEST_DATABASE_URL, DATABASE_URL,
// or a native embedded PostgreSQL server). If REQUIRE_PG=1 or CI=true and no DB is available,
// it FAILS the test immediately (never skips).
func GetTestPool(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if strings.TrimSpace(dbURL) == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}

	requirePG := os.Getenv("REQUIRE_PG") == "1" || strings.ToLower(os.Getenv("CI")) == "true"

	poolOnce.Do(func() {
		if strings.TrimSpace(dbURL) == "" {
			embeddedOnce.Do(func() {
				cfg := embeddedpostgres.DefaultConfig().
					Port(9876).
					Username("venus_test").
					Password("venus_test_pwd").
					Database("venus_test_db")
				embeddedDB = embeddedpostgres.NewDatabase(cfg)
				embeddedErr = embeddedDB.Start()
			})

			if embeddedErr != nil {
				errStr := strings.ToLower(embeddedErr.Error())
				if !strings.Contains(errStr, "already") && !strings.Contains(errStr, "in use") && !strings.Contains(errStr, "listening") && !strings.Contains(errStr, "not empty") {
					poolErr = embeddedErr
					return
				}
			}
			dbURL = embeddedURL
		}

		config, err := pgxpool.ParseConfig(dbURL)
		if err != nil {
			poolErr = err
			return
		}
		config.MaxConns = 40
		config.MinConns = 5

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		p, err := pgxpool.NewWithConfig(ctx, config)
		if err != nil {
			poolErr = err
			return
		}

		if err := p.Ping(ctx); err != nil {
			p.Close()
			poolErr = err
			return
		}

		applySchema(p)
		sharedPool = p
	})

	if poolErr != nil || sharedPool == nil {
		if requirePG {
			t.Fatalf("FATAL: REQUIRE_PG is set but failed to connect to PostgreSQL: %v", poolErr)
		}
		t.Skipf("PostgreSQL is not available: %v", poolErr)
		return nil, func() {}
	}

	cleanup := func() {}
	return sharedPool, cleanup
}

func applySchema(pool *pgxpool.Pool) {
	candidates := []string{
		"../db/schema.sql",
		"../../internal/db/schema.sql",
		"../../../internal/db/schema.sql",
		"venus_backend/internal/db/schema.sql",
		"internal/db/schema.sql",
	}

	var schemaBytes []byte
	for _, c := range candidates {
		abs, _ := filepath.Abs(c)
		b, err := os.ReadFile(abs)
		if err == nil {
			schemaBytes = b
			break
		}
	}

	if len(schemaBytes) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	_, _ = pool.Exec(ctx, string(schemaBytes))
	_, _ = pool.Exec(ctx, `ALTER TABLE users DROP CONSTRAINT IF EXISTS users_wallet_balance_check;
		ALTER TABLE users ADD CONSTRAINT users_wallet_balance_check CHECK (role = 'system' OR wallet_balance >= -credit_limit);
		DROP INDEX IF EXISTS idx_ledger_entries_unique_reversal;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_ledger_entries_unique_reversal ON ledger_entries (ref_type, ref_id, account_id) WHERE ref_type IN ('reversal', 'refund', 'agent_commission_reversal');
		CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT PRIMARY KEY, name TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT now());
		INSERT INTO schema_migrations (version, name, applied_at) VALUES (7, 'reversal_idempotency', now()) ON CONFLICT (version) DO NOTHING;`)
}
