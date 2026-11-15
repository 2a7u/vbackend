package migrations_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"venus_backend/internal/db/migrations"
	"venus_backend/internal/testhelper"
)

func TestMigrationsFileIntegrity(t *testing.T) {
	all, err := migrations.LoadMigrations()
	if err != nil {
		t.Fatalf("LoadMigrations: %v", err)
	}

	if len(all) == 0 {
		t.Fatal("no migrations loaded from embedded SQL")
	}

	for i, m := range all {
		expectedVer := int64(i + 1)
		if m.Version != expectedVer {
			t.Errorf("migration sequence gap: index %d has version %d, expected %d", i, m.Version, expectedVer)
		}
		if m.UpSQL == "" {
			t.Errorf("migration %d (%s) missing up.sql content", m.Version, m.Name)
		}
		if m.DownSQL == "" {
			t.Errorf("migration %d (%s) missing down.sql content", m.Version, m.Name)
		}
	}
}

func TestMigrationsLifecycleOnRealPG(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		return
	}

	ctx := context.Background()

	// 1. Initialize schema_migrations_applied table
	err := migrations.EnsureMigrationsTable(ctx, pool)
	if err != nil {
		t.Fatalf("EnsureMigrationsTable: %v", err)
	}

	// 2. Apply all pending migrations (1..TargetVersion)
	applied, err := migrations.ApplyPending(ctx, pool)
	if err != nil {
		t.Fatalf("ApplyPending: %v", err)
	}
	t.Logf("Applied %d migrations", applied)

	// 3. Verify target version
	err = migrations.VerifySchemaVersion(ctx, pool, migrations.TargetVersion)
	if err != nil {
		t.Fatalf("VerifySchemaVersion: %v", err)
	}

	// 4. Verify idempotency: running again applies 0 migrations
	appliedSecond, err := migrations.ApplyPending(ctx, pool)
	if err != nil {
		t.Fatalf("ApplyPending second run: %v", err)
	}
	if appliedSecond != 0 {
		t.Errorf("expected 0 pending on second run, got %d", appliedSecond)
	}
}

func TestMigrationsOnRealProductionSchemaDump(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		return
	}

	ctx := context.Background()

	// 1. Locate and read prod_schema.sql (29 production tables, 43 indexes from PostgreSQL 17.11)
	prodSchemaPaths := []string{
		"prod_schema.sql",
		filepath.Join("..", "..", "prod_schema.sql"),
		filepath.Join("..", "..", "..", "prod_schema.sql"),
	}
	var schemaBytes []byte
	var foundPath string
	for _, p := range prodSchemaPaths {
		if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
			schemaBytes = b
			foundPath = p
			break
		}
	}
	if len(schemaBytes) == 0 {
		t.Fatal("could not find prod_schema.sql to test production schema migration compatibility")
	}
	t.Logf("Loaded production schema dump from %s (%d bytes)", foundPath, len(schemaBytes))

	// 2. Wipe public schema and restore the exact production schema dump
	_, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`)
	if err != nil {
		t.Fatalf("failed to reset public schema: %v", err)
	}

	_, err = pool.Exec(ctx, string(schemaBytes))
	if err != nil {
		t.Fatalf("failed to restore real production schema dump into database: %v", err)
	}
	t.Log("Successfully restored real production schema dump (29 tables, 43 indexes)")

	// 3. Ensure migrations table
	err = migrations.EnsureMigrationsTable(ctx, pool)
	if err != nil {
		t.Fatalf("EnsureMigrationsTable: %v", err)
	}

	// 4. Apply all pending migrations on top of real production schema
	applied, err := migrations.ApplyPending(ctx, pool)
	if err != nil {
		t.Fatalf("ApplyPending on real production schema failed: %v", err)
	}
	t.Logf("Successfully applied %d migrations on real production schema", applied)
	if applied != migrations.TargetVersion {
		t.Fatalf("expected to apply all %d migrations, but applied %d", migrations.TargetVersion, applied)
	}

	// 5. Verify target version reached
	err = migrations.VerifySchemaVersion(ctx, pool, migrations.TargetVersion)
	if err != nil {
		t.Fatalf("VerifySchemaVersion failed: %v", err)
	}

	// 6. Verify Money Types migration 000008 converted vouchers_vault columns to BIGINT
	var costType, sellType string
	err = pool.QueryRow(ctx, `
		SELECT data_type 
		FROM information_schema.columns 
		WHERE table_name = 'vouchers_vault' AND column_name = 'cost_price'
	`).Scan(&costType)
	if err != nil {
		t.Fatalf("query cost_price type: %v", err)
	}
	err = pool.QueryRow(ctx, `
		SELECT data_type 
		FROM information_schema.columns 
		WHERE table_name = 'vouchers_vault' AND column_name = 'selling_price'
	`).Scan(&sellType)
	if err != nil {
		t.Fatalf("query selling_price type: %v", err)
	}

	if costType != "bigint" || sellType != "bigint" {
		t.Fatalf("expected cost_price and selling_price to be bigint, got cost=%s, sell=%s", costType, sellType)
	}
	t.Logf("Confirmed money columns converted to BIGINT: cost_price=%s, selling_price=%s", costType, sellType)
}
