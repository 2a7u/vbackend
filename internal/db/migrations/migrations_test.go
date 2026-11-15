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
