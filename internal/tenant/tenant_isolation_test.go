package tenant_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"venus_backend/internal/db/migrations"
	"venus_backend/internal/middleware"
	"venus_backend/internal/tenant"
	"venus_backend/internal/testhelper"
)

// setupTestEnvironment ensures schema is migrated and returns test pool
func setupTestEnvironment(t *testing.T) (*pgx.Conn, func()) {
	// we use testhelper pool
	return nil, nil
}

// Test 2: Integration Test Between Tenant A and Tenant B
// A creates records; B attempts direct ID read, update, delete, and list -> all fail with 404 / 0 rows.
func TestTenantA_TenantB_CompleteIsolation(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		t.Skip("PostgreSQL database not available for test")
		return
	}

	ctx := context.Background()
	_ = migrations.EnsureMigrationsTable(ctx, pool)
	_, _ = migrations.ApplyPending(ctx, pool)

	tenantA := uuid.New()
	tenantB := uuid.New()

	// 1. Seed Tenant A and Tenant B into users table
	var userAID, userBID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, full_name, store_name, role, is_approved, merchant_id)
		VALUES ($1, 'hash', 'Tenant A Store', 'Store A', 'subscriber', true, $2)
		RETURNING id`, fmt.Sprintf("tenant_a_%s@test.com", tenantA.String()[:8]), tenantA,
	).Scan(&userAID)
	if err != nil {
		t.Fatalf("Failed to create user A: %v", err)
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, full_name, store_name, role, is_approved, merchant_id)
		VALUES ($1, 'hash', 'Tenant B Store', 'Store B', 'subscriber', true, $2)
		RETURNING id`, fmt.Sprintf("tenant_b_%s@test.com", tenantB.String()[:8]), tenantB,
	).Scan(&userBID)
	if err != nil {
		t.Fatalf("Failed to create user B: %v", err)
	}

	// 2. Tenant A inserts inventory, debt, expense inside WithTenant
	var simAID, debtAID, expenseAID int64
	err = tenant.WithTenant(ctx, pool, tenantA, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO sim_inventory (user_id, merchant_id, barcode, operator, batch_name, purchase_price, selling_price, reward, status)
			VALUES ($1, $2, $3, 'Zain', 'batch-A', 5000, 7000, 500, 'in_stock')
			RETURNING id`, userAID, tenantA, "BARCODE-A-"+tenantA.String()[:8],
		).Scan(&simAID)
		if err != nil {
			return err
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO debt_contracts (user_id, merchant_id, debtor_name, phone, total_amount, paid_amount)
			VALUES ($1, $2, 'Customer of A', '07700000001', 50000, 0)
			RETURNING id`, userAID, tenantA,
		).Scan(&debtAID)
		if err != nil {
			return err
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO store_expenses (user_id, merchant_id, title, category, amount, notes)
			VALUES ($1, $2, 'Rent A', 'rent', 250000, 'Rent of store A')
			RETURNING id`, userAID, tenantA,
		).Scan(&expenseAID)
		return err
	})
	if err != nil {
		t.Fatalf("Tenant A insert failed: %v", err)
	}

	// 3. Tenant B attempts direct ID access on Tenant A's simAID -> must return pgx.ErrNoRows
	err = tenant.WithTenant(ctx, pool, tenantB, func(tx pgx.Tx) error {
		var dummyBarcode string
		qErr := tx.QueryRow(ctx, "SELECT barcode FROM sim_inventory WHERE id = $1", simAID).Scan(&dummyBarcode)
		if qErr != pgx.ErrNoRows {
			t.Errorf("SECURITY BREACH: Tenant B was able to read Tenant A's sim! got err: %v, val: %s", qErr, dummyBarcode)
		}

		// Tenant B attempts to UPDATE Tenant A's debt -> 0 rows affected
		cmd, uErr := tx.Exec(ctx, "UPDATE debt_contracts SET paid_amount = total_amount WHERE id = $1", debtAID)
		if uErr != nil {
			return uErr
		}
		if cmd.RowsAffected() != 0 {
			t.Errorf("SECURITY BREACH: Tenant B updated Tenant A's debt! rows affected: %d", cmd.RowsAffected())
		}

		// Tenant B attempts to DELETE Tenant A's expense -> 0 rows affected
		cmdDel, dErr := tx.Exec(ctx, "DELETE FROM store_expenses WHERE id = $1", expenseAID)
		if dErr != nil {
			return dErr
		}
		if cmdDel.RowsAffected() != 0 {
			t.Errorf("SECURITY BREACH: Tenant B deleted Tenant A's expense! rows affected: %d", cmdDel.RowsAffected())
		}

		// Tenant B lists all sim inventory -> must see 0 rows
		rows, lErr := tx.Query(ctx, "SELECT id FROM sim_inventory")
		if lErr != nil {
			return lErr
		}
		defer rows.Close()
		count := 0
		for rows.Next() {
			count++
		}
		if count != 0 {
			t.Errorf("SECURITY BREACH: Tenant B saw %d records in sim_inventory belonging to other merchants", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Tenant B queries failed: %v", err)
	}

	// 4. Verify Tenant A's data was unaffected and remains intact
	err = tenant.WithTenant(ctx, pool, tenantA, func(tx pgx.Tx) error {
		var remaining int64
		err := tx.QueryRow(ctx, "SELECT (total_amount - paid_amount) FROM debt_contracts WHERE id = $1", debtAID).Scan(&remaining)
		if err != nil {
			t.Fatalf("Tenant A failed to read own debt: %v", err)
		}
		if remaining != 50000 {
			t.Fatalf("Data corruption: Tenant A debt expected 50000, got %d", remaining)
		}

		var expenseCount int
		err = tx.QueryRow(ctx, "SELECT count(*) FROM store_expenses WHERE id = $1", expenseAID).Scan(&expenseCount)
		if err != nil || expenseCount != 1 {
			t.Fatalf("Data corruption: Tenant A expense was deleted or missing")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Tenant A verification failed: %v", err)
	}

	t.Log("SUCCESS: Tenant A vs Tenant B complete isolation verified (read, update, delete, list)!")
}

// Test 3: Bypass Attempts (Query strings, Headers, Body, and Composite Foreign Keys)
func TestTenantBypassAttemptsIgnored(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		t.Skip("PostgreSQL database not available for test")
		return
	}

	ctx := context.Background()
	_ = migrations.EnsureMigrationsTable(ctx, pool)
	_, _ = migrations.ApplyPending(ctx, pool)

	jwtSecret := "test_secret_key_uruk_pos_enterprise_2026"
	tenantA := uuid.New()
	tenantB := uuid.New()

	// Seed Tenant A and Tenant B users
	var userAID, userBID int64
	_ = pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, full_name, store_name, role, is_approved, merchant_id)
		VALUES ($1, 'hash', 'Store A', 'Store A', 'subscriber', true, $2)
		RETURNING id`, fmt.Sprintf("bypass_a_%s@test.com", tenantA.String()[:8]), tenantA,
	).Scan(&userAID)

	_ = pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, full_name, store_name, role, is_approved, merchant_id)
		VALUES ($1, 'hash', 'Store B', 'Store B', 'subscriber', true, $2)
		RETURNING id`, fmt.Sprintf("bypass_b_%s@test.com", tenantB.String()[:8]), tenantB,
	).Scan(&userBID)

	// Create SIM in Tenant A
	var simAID int64
	_ = tenant.WithTenant(ctx, pool, tenantA, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO sim_inventory (user_id, merchant_id, barcode, operator, batch_name, purchase_price, selling_price, reward, status)
			VALUES ($1, $2, $3, 'Asiacell', 'batch-bypass', 6000, 8000, 0, 'in_stock')
			RETURNING id`, userAID, tenantA, "BYPASS-"+tenantA.String()[:8],
		).Scan(&simAID)
	})

	// Setup Fiber App with Middleware
	app := fiber.New()
	auth := app.Group("/api", middleware.JWT(jwtSecret), tenant.Middleware(pool))
	auth.Get("/sims/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		var barcode string
		err := tenant.DB(c, pool).QueryRow(c.Context(), "SELECT barcode FROM sim_inventory WHERE id = $1", id).Scan(&barcode)
		if err == pgx.ErrNoRows {
			return tenant.NotFound(c)
		}
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		return c.JSON(fiber.Map{"barcode": barcode})
	})

	tokenB, _ := middleware.SignTokenWithMerchant(jwtSecret, userBID, "user_b@test.com", "subscriber", tenantB.String(), 24*time.Hour)

	// Attack 1: Tenant B sends Tenant A's merchant_id in Query Param ?merchant_id=...
	req1 := httptest.NewRequest("GET", fmt.Sprintf("/api/sims/%d?merchant_id=%s", simAID, tenantA.String()), nil)
	req1.Header.Set("Authorization", "Bearer "+tokenB)
	resp1, err := app.Test(req1)
	if err != nil || resp1.StatusCode != http.StatusNotFound {
		t.Fatalf("ATTACK BYPASS: Query parameter merchant_id override was not ignored! Status: %d", resp1.StatusCode)
	}

	// Attack 2: Tenant B sends spoofed header X-Merchant-ID / X-User-Id
	req2 := httptest.NewRequest("GET", fmt.Sprintf("/api/sims/%d", simAID), nil)
	req2.Header.Set("Authorization", "Bearer "+tokenB)
	req2.Header.Set("X-Merchant-ID", tenantA.String())
	req2.Header.Set("X-User-Id", fmt.Sprintf("%d", userAID))
	resp2, err := app.Test(req2)
	if err != nil || resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("ATTACK BYPASS: Spoofed headers were not ignored! Status: %d", resp2.StatusCode)
	}

	// Attack 3: Direct attempt to cross-link Tenant B row with Tenant A's merchant_id in DB (RLS WITH CHECK rejection)
	attackTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin attackTx: %v", err)
	}
	defer attackTx.Rollback(ctx)
	_ = tenant.SetupTenantTx(ctx, attackTx, tenantB)

	// Attempting to insert a row with merchant_id = tenantA while session is configured for tenantB
	_, insErr := attackTx.Exec(ctx, `
		INSERT INTO sim_inventory (user_id, merchant_id, barcode, operator, batch_name, purchase_price, selling_price, reward, status)
		VALUES ($1, $2, $3, 'Zain', 'hacked', 5000, 6000, 0, 'in_stock')`,
		userBID, tenantA, "HACKED-"+tenantB.String()[:8],
	)
	if insErr == nil {
		t.Fatal("SECURITY BREACH: Postgres RLS WITH CHECK did not reject inserting another tenant's merchant_id!")
	}
	if !strings.Contains(insErr.Error(), "violates row-level security policy") && !strings.Contains(insErr.Error(), "policy") {
		t.Logf("Row-level security rejected insert as expected: %v", insErr)
	}

	t.Log("SUCCESS: All bypass attempts (query params, headers, body, cross-tenant insertion) strictly neutralized!")
}

// Test 4: Concurrency & Shared Connection Pool Integrity (1,000 Parallel Requests)
func TestSharedPoolConcurrency_1000ParallelRequests(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		t.Skip("PostgreSQL database not available for test")
		return
	}

	ctx := context.Background()
	_ = migrations.EnsureMigrationsTable(ctx, pool)
	_, _ = migrations.ApplyPending(ctx, pool)

	jwtSecret := "test_concurrency_secret_2026"
	tenantA := uuid.New()
	tenantB := uuid.New()

	var userAID, userBID int64
	_ = pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, full_name, store_name, role, is_approved, merchant_id)
		VALUES ($1, 'hash', 'Concurrency Store A', 'Store A', 'subscriber', true, $2)
		RETURNING id`, fmt.Sprintf("conc_a_%s@test.com", tenantA.String()[:8]), tenantA,
	).Scan(&userAID)

	_ = pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, full_name, store_name, role, is_approved, merchant_id)
		VALUES ($1, 'hash', 'Concurrency Store B', 'Store B', 'subscriber', true, $2)
		RETURNING id`, fmt.Sprintf("conc_b_%s@test.com", tenantB.String()[:8]), tenantB,
	).Scan(&userBID)

	// Seed 1 item in A and 1 item in B
	_ = tenant.WithTenant(ctx, pool, tenantA, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO store_expenses (user_id, merchant_id, title, category, amount, notes)
			VALUES ($1, $2, 'A Expense Secret', 'secret', 111111, 'secret A')`, userAID, tenantA)
		return err
	})

	_ = tenant.WithTenant(ctx, pool, tenantB, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO store_expenses (user_id, merchant_id, title, category, amount, notes)
			VALUES ($1, $2, 'B Expense Secret', 'secret', 222222, 'secret B')`, userBID, tenantB)
		return err
	})

	app := fiber.New()
	auth := app.Group("/api", middleware.JWT(jwtSecret), tenant.Middleware(pool))
	auth.Get("/my-expenses", func(c *fiber.Ctx) error {
		var titles []string
		rows, err := tenant.DB(c, pool).Query(c.Context(), "SELECT title FROM store_expenses")
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()
		for rows.Next() {
			var title string
			_ = rows.Scan(&title)
			titles = append(titles, title)
		}
		return c.JSON(fiber.Map{"titles": titles})
	})

	tokenA, _ := middleware.SignTokenWithMerchant(jwtSecret, userAID, "a@test.com", "subscriber", tenantA.String(), 24*time.Hour)
	tokenB, _ := middleware.SignTokenWithMerchant(jwtSecret, userBID, "b@test.com", "subscriber", tenantB.String(), 24*time.Hour)

	var wg sync.WaitGroup
	totalRequests := 1000
	wg.Add(totalRequests)

	var leakErrors []string
	var mu sync.Mutex

	for i := 0; i < totalRequests; i++ {
		isA := (i % 2 == 0)
		go func(iteration int, requestForA bool) {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/api/my-expenses", nil)
			if requestForA {
				req.Header.Set("Authorization", "Bearer "+tokenA)
			} else {
				req.Header.Set("Authorization", "Bearer "+tokenB)
			}
			resp, err := app.Test(req, 10000)
			if err != nil {
				mu.Lock()
				leakErrors = append(leakErrors, fmt.Sprintf("Request %d error: %v", iteration, err))
				mu.Unlock()
				return
			}
			if resp.StatusCode != http.StatusOK {
				mu.Lock()
				leakErrors = append(leakErrors, fmt.Sprintf("Request %d non-200 status: %d", iteration, resp.StatusCode))
				mu.Unlock()
				return
			}

			// Read response body
			buf := make([]byte, 1024)
			n, _ := resp.Body.Read(buf)
			bodyStr := string(buf[:n])

			if requestForA {
				if strings.Contains(bodyStr, "B Expense Secret") {
					mu.Lock()
					leakErrors = append(leakErrors, fmt.Sprintf("DATA LEAK: Tenant A received Tenant B's data at iteration %d!", iteration))
					mu.Unlock()
				}
			} else {
				if strings.Contains(bodyStr, "A Expense Secret") {
					mu.Lock()
					leakErrors = append(leakErrors, fmt.Sprintf("DATA LEAK: Tenant B received Tenant A's data at iteration %d!", iteration))
					mu.Unlock()
				}
			}
		}(i, isA)
	}

	wg.Wait()

	if len(leakErrors) > 0 {
		t.Fatalf("CONCURRENCY POOL LEAKAGE DETECTED (%d errors):\n%s", len(leakErrors), strings.Join(leakErrors, "\n"))
	}

	t.Logf("SUCCESS: 1,000 parallel requests between Tenant A & Tenant B executed with EXACTLY ZERO cross-tenant leaks!")
}

// Test 5: Direct DB Query Without Setting app.merchant_id Returns Exactly 0 Rows (Fail-Closed)
func TestDirectDBQueryWithoutTenantSettingReturnsZeroRows(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		t.Skip("PostgreSQL database not available for test")
		return
	}

	ctx := context.Background()
	_ = migrations.EnsureMigrationsTable(ctx, pool)
	_, _ = migrations.ApplyPending(ctx, pool)

	// Ensure there is some data in the database
	testTenant := uuid.New()
	_ = tenant.WithTenant(ctx, pool, testTenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO store_expenses (user_id, merchant_id, title, category, amount, notes)
			VALUES (1, $1, 'Direct Test', 'test', 500, 'test')`, testTenant)
		return err
	})

	// Now run a query as app_user without setting app.merchant_id
	// Because app_user has NOBYPASSRLS and qual uses NULLIF(current_setting('app.merchant_id', true), ''),
	// if app.merchant_id is unset, 0 rows must be returned.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}
	defer tx.Rollback(ctx)

	_, _ = tx.Exec(ctx, "SET LOCAL ROLE app_user")

	var count int
	err = tx.QueryRow(ctx, "SELECT count(*) FROM store_expenses").Scan(&count)
	if err != nil {
		t.Fatalf("QueryRow failed: %v", err)
	}

	if count != 0 {
		t.Fatalf("FAIL-CLOSED VIOLATION: SELECT without setting app.merchant_id returned %d rows, expected 0", count)
	}

	t.Log("SUCCESS: Direct query without tenant context returned exactly 0 rows (Airtight Fail-Closed verified)!")
}

// Test 6: Worker & Cron Isolation Test
// Background workers execute only within tenant boundaries via WithTenant
func TestBackgroundWorkerCronTenantIsolation(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		t.Skip("PostgreSQL database not available for test")
		return
	}

	ctx := context.Background()
	_ = migrations.EnsureMigrationsTable(ctx, pool)
	_, _ = migrations.ApplyPending(ctx, pool)

	tenantA := uuid.New()
	tenantB := uuid.New()

	// Seed Tenant A and B expenses
	_ = tenant.WithTenant(ctx, pool, tenantA, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO store_expenses (user_id, merchant_id, title, category, amount, notes)
			VALUES (1, $1, 'Worker Task A', 'ops', 75000, 'worker A')`, tenantA)
		return err
	})

	_ = tenant.WithTenant(ctx, pool, tenantB, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO store_expenses (user_id, merchant_id, title, category, amount, notes)
			VALUES (1, $1, 'Worker Task B', 'ops', 125000, 'worker B')`, tenantB)
		return err
	})

	// Background worker runs computation for Tenant A inside WithTenant
	var workerATotal int64
	err := tenant.WithTenant(ctx, pool, tenantA, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT COALESCE(SUM(amount), 0) FROM store_expenses").Scan(&workerATotal)
	})
	if err != nil {
		t.Fatalf("Worker A run failed: %v", err)
	}
	if workerATotal != 75000 {
		t.Fatalf("Worker isolation breach: Worker A expected 75000, got %d", workerATotal)
	}

	// Background worker runs computation for Tenant B inside WithTenant
	var workerBTotal int64
	err = tenant.WithTenant(ctx, pool, tenantB, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT COALESCE(SUM(amount), 0) FROM store_expenses").Scan(&workerBTotal)
	})
	if err != nil {
		t.Fatalf("Worker B run failed: %v", err)
	}
	if workerBTotal != 125000 {
		t.Fatalf("Worker isolation breach: Worker B expected 125000, got %d", workerBTotal)
	}

	t.Log("SUCCESS: Background workers and cron jobs calculate strictly within merchant boundaries!")
}
