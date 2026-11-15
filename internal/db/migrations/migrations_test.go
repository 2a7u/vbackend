package migrations_test

import (
	"context"
	"testing"
	"venus_backend/internal/db/migrations"
	"venus_backend/internal/testhelper"
)

func TestMigrationsFileIntegrity(t *testing.T) {
	all, err := migrations.LoadMigrations()
	if err != nil {
		t.Fatalf("LoadMigrations failed: %v", err)
	}

	if len(all) < migrations.TargetVersion {
		t.Fatalf("Expected at least %d migrations, loaded %d", migrations.TargetVersion, len(all))
	}

	for i, m := range all {
		expectedVer := int64(i + 1)
		if m.Version != expectedVer {
			t.Errorf("Migration index %d has version %d, expected %d", i, m.Version, expectedVer)
		}
		if m.UpSQL == "" {
			t.Errorf("Migration %d (%s) has empty UpSQL", m.Version, m.Name)
		}
		if m.DownSQL == "" {
			t.Errorf("Migration %d (%s) has empty DownSQL", m.Version, m.Name)
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

	// Ensure clean table
	err := migrations.EnsureMigrationsTable(ctx, pool)
	if err != nil {
		t.Fatalf("EnsureMigrationsTable failed: %v", err)
	}

	// Apply pending
	applied, err := migrations.ApplyPending(ctx, pool)
	if err != nil {
		t.Fatalf("ApplyPending failed: %v", err)
	}
	t.Logf("Applied %d migrations", applied)

	// Verify schema version
	err = migrations.VerifySchemaVersion(ctx, pool, migrations.TargetVersion)
	if err != nil {
		t.Fatalf("VerifySchemaVersion failed: %v", err)
	}

	currentVer, err := migrations.GetCurrentVersion(ctx, pool)
	if err != nil || currentVer < migrations.TargetVersion {
		t.Fatalf("Expected current version >= %d, got %d (err: %v)", migrations.TargetVersion, currentVer, err)
	}
}

// TestMigrationsOnSimulatedProductionSchemaDump verifies that migrations run successfully
// on a pre-existing legacy production database schema (simulated pg_dump --schema-only).
func TestMigrationsOnSimulatedProductionSchemaDump(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		return
	}

	ctx := context.Background()

	// 1. Simulate legacy production tables with bare minimal historical columns
	legacyDDL := `
	CREATE TABLE IF NOT EXISTS legacy_users_prod (
		id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		email TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'subscriber'
	);
	INSERT INTO legacy_users_prod (email, password_hash) VALUES ('legacy_subscriber@venus.test', 'hash123') ON CONFLICT DO NOTHING;

	-- Reconcile users
	ALTER TABLE users ADD COLUMN IF NOT EXISTS wallet_balance BIGINT NOT NULL DEFAULT 0;
	ALTER TABLE users ADD COLUMN IF NOT EXISTS credit_limit BIGINT NOT NULL DEFAULT 0;
	ALTER TABLE users ADD COLUMN IF NOT EXISTS wallet_status TEXT NOT NULL DEFAULT 'active';
	ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'subscriber';

	-- Reconcile vouchers_vault
	ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS is_sold BOOLEAN NOT NULL DEFAULT FALSE;
	ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'available';
	ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS sold_to_user_id BIGINT;
	ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS sold_price BIGINT NOT NULL DEFAULT 0;
	`
	_, err := pool.Exec(ctx, legacyDDL)
	if err != nil {
		t.Fatalf("failed to simulate legacy schema: %v", err)
	}

	// 2. Apply all migrations
	err = migrations.EnsureMigrationsTable(ctx, pool)
	if err != nil {
		t.Fatalf("EnsureMigrationsTable: %v", err)
	}

	applied, err := migrations.ApplyPending(ctx, pool)
	if err != nil {
		t.Fatalf("ApplyPending on legacy production schema failed: %v", err)
	}
	t.Logf("Successfully applied %d migrations on simulated production schema", applied)

	// 3. Verify target version
	err = migrations.VerifySchemaVersion(ctx, pool, migrations.TargetVersion)
	if err != nil {
		t.Fatalf("VerifySchemaVersion failed: %v", err)
	}
}
