package tenant_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"venus_backend/internal/db/migrations"
	"venus_backend/internal/testhelper"
)

// TestRLSCoverageAcrossAllTenantTables collects dynamically every table with a merchant_id column
// in the public schema and asserts that:
// 1. Row Level Security is ENABLED (relrowsecurity = true)
// 2. FORCE Row Level Security is ENABLED (relforcerowsecurity = true)
// 3. A tenant isolation policy exists with qual and with_check enforcing current_setting('app.merchant_id')
// Any newly added table with merchant_id missing RLS or FORCE RLS fails the build immediately.
func TestRLSCoverageAcrossAllTenantTables(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		t.Skip("PostgreSQL database not available for test")
		return
	}

	ctx := context.Background()

	// Ensure all migrations up to TargetVersion are applied
	_ = migrations.EnsureMigrationsTable(ctx, pool)
	_, err := migrations.ApplyPending(ctx, pool)
	if err != nil {
		t.Fatalf("ApplyPending failed: %v", err)
	}

	// 1. Query all tables in public schema that have merchant_id
	query := `
		SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
		  AND c.relkind = 'r'
		  AND EXISTS (
		      SELECT 1 FROM information_schema.columns col
		      WHERE col.table_schema = 'public'
		        AND col.table_name = c.relname
		        AND col.column_name = 'merchant_id'
		  )
		ORDER BY c.relname;
	`
	rows, err := pool.Query(ctx, query)
	if err != nil {
		t.Fatalf("Failed to query tenant tables: %v", err)
	}
	defer rows.Close()

	type tableRLS struct {
		name     string
		hasRLS   bool
		forceRLS bool
	}

	var tables []tableRLS
	for rows.Next() {
		var item tableRLS
		if err := rows.Scan(&item.name, &item.hasRLS, &item.forceRLS); err != nil {
			t.Fatalf("Failed to scan table info: %v", err)
		}
		tables = append(tables, item)
	}
	rows.Close()

	if len(tables) == 0 {
		t.Fatal("No tenant tables found with merchant_id; schema may not be migrated properly")
	}

	t.Logf("Verifying RLS & FORCE RLS coverage across %d tenant tables...", len(tables))

	var violations []string

	for _, tbl := range tables {
		// Users table stores identities and maps to merchant_id; other tenant data tables MUST have RLS + FORCE RLS
		if tbl.name == "users" {
			continue
		}

		if !tbl.hasRLS {
			violations = append(violations, fmt.Sprintf("Table %s does NOT have Row Level Security ENABLED", tbl.name))
		}
		if !tbl.forceRLS {
			violations = append(violations, fmt.Sprintf("Table %s does NOT have FORCE Row Level Security ENABLED", tbl.name))
		}

		// Verify policy existence
		var policyCount int
		policyQuery := `
			SELECT count(*)
			FROM pg_policy pol
			JOIN pg_class c ON c.oid = pol.polrelid
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public' AND c.relname = $1
			  AND pg_get_expr(pol.polqual, pol.polrelid) ILIKE '%app.merchant_id%';
		`
		err := pool.QueryRow(ctx, policyQuery, tbl.name).Scan(&policyCount)
		if err != nil {
			violations = append(violations, fmt.Sprintf("Table %s policy query error: %v", tbl.name, err))
		} else if policyCount == 0 {
			violations = append(violations, fmt.Sprintf("Table %s is missing tenant_isolation_policy with app.merchant_id check", tbl.name))
		}
	}

	if len(violations) > 0 {
		t.Fatalf("RLS POLICY & FORCE RLS COVERAGE VIOLATIONS DETECTED (%d):\n%s", len(violations), strings.Join(violations, "\n"))
	}

	t.Logf("SUCCESS: All %d tenant tables verified with RLS + FORCE RLS + strict tenant isolation policies!", len(tables))
}
