package handlers_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"venus_backend/internal/vault"
)

// getTestPool connects to PostgreSQL if DATABASE_URL or TEST_DATABASE_URL is defined.
func getTestPool(t *testing.T) *pgxpool.Pool {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("skipping live PostgreSQL test: TEST_DATABASE_URL or DATABASE_URL not set")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("skipping live PostgreSQL test: failed to parse DATABASE_URL: %v", err)
		return nil
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping live PostgreSQL test: could not connect to PostgreSQL: %v", err)
		return nil
	}

	return pool
}

// TestPostgreSQLConcurrent100SalesOneWallet runs against real PostgreSQL when configured.
func TestPostgreSQLConcurrent100SalesOneWallet(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	defer pool.Close()

	ctx := context.Background()

	// Create test user with 50,000 balance and 0 credit limit
	var testUserID int64
	err := pool.QueryRow(ctx,
		`INSERT INTO users (email, full_name, role, wallet_balance, credit_limit)
		 VALUES ($1, $2, 'subscriber', 50000, 0)
		 RETURNING id`,
		fmt.Sprintf("test_concur_%d@venus.iq", time.Now().UnixNano()), "Concurrent Test User",
	).Scan(&testUserID)
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}

	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, testUserID)
	}()

	// 100 parallel buyers attempting to debit 1,000 IQD each
	// Since initial balance is 50,000, EXACTLY 50 must succeed and 50 must fail with insufficient funds.
	const totalParallel = 100
	const debitPerSale = int64(1000)

	var wg sync.WaitGroup
	var successCount atomic.Int64
	var failureCount atomic.Int64

	for i := 0; i < totalParallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := pool.Begin(ctx)
			if err != nil {
				failureCount.Add(1)
				return
			}
			defer tx.Rollback(ctx)

			var currentBal, creditLim int64
			err = tx.QueryRow(ctx,
				`SELECT wallet_balance, credit_limit FROM users WHERE id = $1 FOR UPDATE`,
				testUserID,
			).Scan(&currentBal, &creditLim)
			if err != nil {
				failureCount.Add(1)
				return
			}

			if currentBal+creditLim < debitPerSale {
				failureCount.Add(1)
				return
			}

			_, err = tx.Exec(ctx,
				`UPDATE users SET wallet_balance = wallet_balance - $1 WHERE id = $2`,
				debitPerSale, testUserID,
			)
			if err != nil {
				failureCount.Add(1)
				return
			}

			if err := tx.Commit(ctx); err == nil {
				successCount.Add(1)
			} else {
				failureCount.Add(1)
			}
		}()
	}

	wg.Wait()

	if successCount.Load() != 50 {
		t.Fatalf("expected exactly 50 debits to succeed, got %d", successCount.Load())
	}
	if failureCount.Load() != 50 {
		t.Fatalf("expected exactly 50 debits to fail, got %d", failureCount.Load())
	}

	var finalBal int64
	_ = pool.QueryRow(ctx, `SELECT wallet_balance FROM users WHERE id = $1`, testUserID).Scan(&finalBal)
	if finalBal != 0 {
		t.Fatalf("expected final balance 0, got %d", finalBal)
	}
}

// TestPostgreSQLConcurrentSameCardSale runs against real PostgreSQL when configured.
func TestPostgreSQLConcurrentSameCardSale(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	defer pool.Close()

	ctx := context.Background()

	// Insert 1 test card
	var cardID int64
	err := pool.QueryRow(ctx,
		`INSERT INTO vouchers_vault (brand_id, voucher_id, pin, pin_encrypted, nonce, key_version, pin_hmac, serial_number, is_sold)
		 VALUES ('test_brand', 'test_card_1', '***1234', 'enc_test', 'nonce_test', 1, $1, $2, FALSE)
		 RETURNING id`,
		fmt.Sprintf("hmac_test_%d", time.Now().UnixNano()),
		fmt.Sprintf("SN_TEST_%d", time.Now().UnixNano()),
	).Scan(&cardID)
	if err != nil {
		t.Fatalf("failed to insert test card: %v", err)
	}

	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM vouchers_vault WHERE id = $1`, cardID)
	}()

	const parallelAttempts = 100
	var wg sync.WaitGroup
	var soldCount atomic.Int64
	var rejectCount atomic.Int64

	for i := 0; i < parallelAttempts; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			tx, err := pool.Begin(ctx)
			if err != nil {
				rejectCount.Add(1)
				return
			}
			defer tx.Rollback(ctx)

			var claimedID int64
			err = tx.QueryRow(ctx,
				`SELECT id FROM vouchers_vault WHERE id = $1 AND is_sold = FALSE FOR UPDATE SKIP LOCKED`,
				cardID,
			).Scan(&claimedID)
			if err != nil {
				rejectCount.Add(1)
				return
			}

			_, err = tx.Exec(ctx,
				`UPDATE vouchers_vault SET is_sold = TRUE, sold_at = now() WHERE id = $1`,
				claimedID,
			)
			if err != nil {
				rejectCount.Add(1)
				return
			}

			if err := tx.Commit(ctx); err == nil {
				soldCount.Add(1)
			} else {
				rejectCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	if soldCount.Load() != 1 {
		t.Fatalf("CRITICAL FAILURE: Card claimed %d times in real PostgreSQL (expected exactly 1)!", soldCount.Load())
	}
	if rejectCount.Load() != parallelAttempts-1 {
		t.Fatalf("expected %d rejections, got %d", parallelAttempts-1, rejectCount.Load())
	}
}

// TestPostgreSQLConcurrentBulkUploadDuplicateHMAC runs against real PostgreSQL when configured.
func TestPostgreSQLConcurrentBulkUploadDuplicateHMAC(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	defer pool.Close()

	ctx := context.Background()
	mgr := vault.Default()
	sharedPIN := fmt.Sprintf("SHARED_PIN_%d", time.Now().UnixNano())
	sharedHMAC := mgr.ComputeHMAC(sharedPIN)

	const parallelInserts = 100
	var wg sync.WaitGroup
	var successInserts atomic.Int64
	var duplicateRejections atomic.Int64

	for i := 0; i < parallelInserts; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			_, err := pool.Exec(ctx,
				`INSERT INTO vouchers_vault (brand_id, voucher_id, pin, pin_encrypted, nonce, key_version, pin_hmac, serial_number, is_sold)
				 VALUES ('brand', 'v1', '***0000', 'ct', 'nonce', 1, $1, $2, FALSE)`,
				sharedHMAC, fmt.Sprintf("SN_%d_%d", workerID, time.Now().UnixNano()),
			)
			if err == nil {
				successInserts.Add(1)
			} else {
				duplicateRejections.Add(1)
			}
		}(i)
	}

	wg.Wait()

	// Clean up inserted row
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM vouchers_vault WHERE pin_hmac = $1`, sharedHMAC)
	}()

	if successInserts.Load() != 1 {
		t.Fatalf("expected exactly 1 insert to succeed, got %d", successInserts.Load())
	}
	if duplicateRejections.Load() != parallelInserts-1 {
		t.Fatalf("expected %d duplicate rejections by PostgreSQL unique index, got %d", parallelInserts-1, duplicateRejections.Load())
	}
}
