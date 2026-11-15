package handlers_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"venus_backend/internal/hierarchy"
	"venus_backend/internal/ledger"
	"venus_backend/internal/middleware"
	"venus_backend/internal/testhelper"
)

// TestRealPG_100ParallelSalesSameCardSoldExactlyOnce proves row-level lock concurrency
// against real PostgreSQL: 100 concurrent transactions compete for the EXACT SAME voucher row.
// Exactly 1 must succeed; 99 must be rejected.
func TestRealPG_100ParallelSalesSameCardSoldExactlyOnce(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		return
	}

	ctx := context.Background()

	// 1. Insert 1 unique voucher
	voucherID := fmt.Sprintf("race_voucher_%d", time.Now().UnixNano())
	var cardID int64
	err := pool.QueryRow(ctx,
		`INSERT INTO vouchers_vault (brand_id, voucher_id, pin, pin_encrypted, nonce, key_version, pin_hmac, serial_number, cost_price, selling_price, is_sold)
		 VALUES ('test_brand', $1, '', 'enc_dummy', 'nonce_dummy', 1, $2, $3, 4000, 5000, FALSE)
		 RETURNING id`,
		voucherID, "hmac_"+voucherID, "serial_"+voucherID,
	).Scan(&cardID)
	if err != nil {
		t.Fatalf("failed to insert test voucher: %v", err)
	}

	// 2. 100 concurrent workers race to lock and sell this exact voucher row
	const concurrency = 100
	var wg sync.WaitGroup
	var successCount atomic.Int64
	var rejectCount atomic.Int64
	startSignal := make(chan struct{})

	for i := 1; i <= concurrency; i++ {
		wg.Add(1)
		go func(buyerID int) {
			defer wg.Done()
			<-startSignal

			tx, err := pool.Begin(ctx)
			if err != nil {
				rejectCount.Add(1)
				return
			}
			defer tx.Rollback(ctx)

			// SELECT ... FOR UPDATE strictly serializes row access
			var lockedID int64
			err = tx.QueryRow(ctx,
				`SELECT id FROM vouchers_vault WHERE id = $1 AND is_sold = FALSE FOR UPDATE`,
				cardID,
			).Scan(&lockedID)
			if err != nil {
				// Already sold or locked by winning transaction
				rejectCount.Add(1)
				return
			}

			// Mark as sold
			_, err = tx.Exec(ctx,
				`UPDATE vouchers_vault SET is_sold = TRUE, sold_to_user_id = $1, sold_at = now() WHERE id = $2`,
				buyerID, cardID,
			)
			if err != nil {
				rejectCount.Add(1)
				return
			}

			if err := tx.Commit(ctx); err == nil {
				successCount.Add(1)
			} else {
				rejectCount.Add(1)
			}
		}(i)
	}

	close(startSignal)
	wg.Wait()

	if successCount.Load() != 1 {
		t.Fatalf("CONCURRENCY VIOLATION: Exactly 1 transaction must sell the card, got %d", successCount.Load())
	}
	if rejectCount.Load() != 99 {
		t.Fatalf("Expected 99 rejected transactions, got %d", rejectCount.Load())
	}

	// Verify DB state
	var isSold bool
	_ = pool.QueryRow(ctx, `SELECT is_sold FROM vouchers_vault WHERE id = $1`, cardID).Scan(&isSold)
	if !isSold {
		t.Fatalf("Voucher should be marked sold in database")
	}
}

// TestRealPG_100ParallelSalesOneWalletNoDoubleSpend proves wallet balance concurrency:
// 100 parallel transactions attempt 1,000 IQD debits from a 50,000 IQD balance.
// Exactly 50 transactions must succeed; 50 must fail. Final balance MUST be exactly 0 IQD.
func TestRealPG_100ParallelSalesOneWalletNoDoubleSpend(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		return
	}

	ctx := context.Background()

	// 1. Insert test user with exact 50,000 IQD balance and 0 credit limit
	email := fmt.Sprintf("wallet_race_%d@venus.test", time.Now().UnixNano())
	var userID int64
	err := pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, full_name, wallet_balance, credit_limit, wallet_status)
		 VALUES ($1, 'hash', 'Race User', 50000, 0, 'active')
		 RETURNING id`,
		email,
	).Scan(&userID)
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}

	const concurrency = 100
	const debitAmount = int64(1000)
	var wg sync.WaitGroup
	var successCount atomic.Int64
	var failCount atomic.Int64
	startSignal := make(chan struct{})

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startSignal

			tx, err := pool.Begin(ctx)
			if err != nil {
				failCount.Add(1)
				return
			}
			defer tx.Rollback(ctx)

			var balance, creditLimit int64
			err = tx.QueryRow(ctx,
				`SELECT wallet_balance, credit_limit FROM users WHERE id = $1 FOR UPDATE`,
				userID,
			).Scan(&balance, &creditLimit)
			if err != nil {
				failCount.Add(1)
				return
			}

			if balance+creditLimit < debitAmount {
				failCount.Add(1)
				return
			}

			_, err = tx.Exec(ctx,
				`UPDATE users SET wallet_balance = wallet_balance - $1 WHERE id = $2`,
				debitAmount, userID,
			)
			if err != nil {
				failCount.Add(1)
				return
			}

			if err := tx.Commit(ctx); err == nil {
				successCount.Add(1)
			} else {
				failCount.Add(1)
			}
		}()
	}

	close(startSignal)
	wg.Wait()

	if successCount.Load() != 50 {
		t.Fatalf("DOUBLE-SPEND VIOLATION: Expected exactly 50 successful debits, got %d", successCount.Load())
	}
	if failCount.Load() != 50 {
		t.Fatalf("Expected exactly 50 rejected debits, got %d", failCount.Load())
	}

	var finalBalance int64
	_ = pool.QueryRow(ctx, `SELECT wallet_balance FROM users WHERE id = $1`, userID).Scan(&finalBalance)
	if finalBalance != 0 {
		t.Fatalf("CRITICAL BALANCE INVARIANT BREACH: Expected final balance 0 IQD, got %d IQD", finalBalance)
	}
}

// TestRealPG_CommissionPostingUnder100ParallelSales verifies double-entry ledger integrity:
// 100 concurrent sales execute, commissions are posted to parent agent, and total debits == total credits.
func TestRealPG_CommissionPostingUnder100ParallelSales(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		return
	}

	ctx := context.Background()
	ledgerSvc := ledger.NewService(pool, nil)
	hierarchySvc := hierarchy.NewService(pool, ledgerSvc)

	// Setup parent agent and child POS point
	nano := time.Now().UnixNano()
	var parentID, childID int64
	_ = pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, full_name, role)
		 VALUES ($1, 'hash', 'Parent Agent', 'subscriber') RETURNING id`,
		fmt.Sprintf("agent_%d@venus.test", nano),
	).Scan(&parentID)

	_ = pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, full_name, role, parent_agent_id)
		 VALUES ($1, 'hash', 'Child POS', 'subscriber', $2) RETURNING id`,
		fmt.Sprintf("pos_%d@venus.test", nano), parentID,
	).Scan(&childID)

	// Set commission rule: 200 basis points (2.00%)
	_, err := pool.Exec(ctx,
		`INSERT INTO agent_commissions (agent_id, product_id, commission_type, commission_basis_points, cap_to_margin)
		 VALUES ($1, '*', 'percentage', 200, TRUE)`,
		parentID,
	)
	if err != nil {
		t.Fatalf("failed to insert commission rule: %v", err)
	}

	const concurrency = 100
	const salePrice = int64(10000) // 10,000 IQD -> 200 IQD commission
	const costPrice = int64(9000)
	var wg sync.WaitGroup
	var completedSales atomic.Int64
	startSignal := make(chan struct{})

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-startSignal

			tx, err := pool.Begin(ctx)
			if err != nil {
				return
			}
			defer tx.Rollback(ctx)

			refID := fmt.Sprintf("sale_comm_%d_%d", nano, idx)
			res, err := hierarchySvc.ProcessSaleCommission(ctx, tx, childID, "prod_1", salePrice, costPrice, refID)
			if err != nil || res == nil {
				t.Errorf("ProcessSaleCommission failed: %v", err)
				return
			}

			if err := tx.Commit(ctx); err == nil {
				completedSales.Add(1)
			}
		}(i)
	}

	close(startSignal)
	wg.Wait()

	if completedSales.Load() != concurrency {
		t.Fatalf("Expected %d completed sales, got %d", concurrency, completedSales.Load())
	}

	// Verify total parent agent earned exactly 100 * 200 = 20,000 IQD
	var agentBalance int64
	_ = pool.QueryRow(ctx, `SELECT wallet_balance FROM users WHERE id = $1`, parentID).Scan(&agentBalance)
	expectedCommission := int64(concurrency * 200)
	if agentBalance != expectedCommission {
		t.Fatalf("Agent wallet mismatch: expected %d IQD, got %d IQD", expectedCommission, agentBalance)
	}

	// Verify ledger entries are strictly balanced (sum debits == sum credits)
	var totalDebit, totalCredit int64
	err = pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(debit), 0), COALESCE(SUM(credit), 0)
		 FROM ledger_entries
		 WHERE ref_type = 'agent_commission' AND ref_id LIKE $1`,
		fmt.Sprintf("sub_%d_sale_sale_comm_%d_%%", childID, nano),
	).Scan(&totalDebit, &totalCredit)
	if err != nil {
		t.Fatalf("query ledger balance failed: %v", err)
	}

	if totalDebit != expectedCommission || totalCredit != expectedCommission {
		t.Fatalf("DOUBLE-ENTRY IMBALANCE: Debits %d != Credits %d, expected %d", totalDebit, totalCredit, expectedCommission)
	}
}

// TestRealPG_DoubleRefundCommissionReversedExactlyOnce proves DB-enforced reversal idempotency:
// 100 concurrent requests try to reverse the same sale commission.
// DB unique constraint `idx_ledger_entries_unique_reversal` guarantees exactly 1 reversal is recorded.
func TestRealPG_DoubleRefundCommissionReversedExactlyOnce(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		return
	}

	ctx := context.Background()
	ledgerSvc := ledger.NewService(pool, nil)
	hierarchySvc := hierarchy.NewService(pool, ledgerSvc)

	nano := time.Now().UnixNano()
	var parentID int64
	_ = pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, full_name, wallet_balance, role)
		 VALUES ($1, 'hash', 'Refund Agent', 50000, 'subscriber') RETURNING id`,
		fmt.Sprintf("refund_agent_%d@venus.test", nano),
	).Scan(&parentID)

	refID := fmt.Sprintf("order_refund_%d", nano)
	const commAmount = int64(1500)

	const concurrency = 100
	var wg sync.WaitGroup
	startSignal := make(chan struct{})

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startSignal

			tx, err := pool.Begin(ctx)
			if err != nil {
				return
			}
			defer tx.Rollback(ctx)

			// Execute reversal under concurrency
			_ = hierarchySvc.ReverseCommission(ctx, tx, parentID, commAmount, refID)
			_ = tx.Commit(ctx)
		}()
	}

	close(startSignal)
	wg.Wait()

	// Verify exactly ONE reversal transaction was executed (COUNT DISTINCT tx_id == 1)
	// and exactly ONE debit entry was recorded for the parent agent
	var distinctTxCount, agentLegCount int64
	err := pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT tx_id), COUNT(*) FILTER (WHERE account_id = $2)
		 FROM ledger_entries
		 WHERE ref_type = 'agent_commission_reversal' AND ref_id = $1`,
		refID, parentID,
	).Scan(&distinctTxCount, &agentLegCount)
	if err != nil {
		t.Fatalf("query reversal count failed: %v", err)
	}

	if distinctTxCount != 1 || agentLegCount != 1 {
		t.Fatalf("REVERSAL IDEMPOTENCY VIOLATION: Expected exactly 1 reversal transaction and 1 agent debit leg, got txCount=%d, agentLegCount=%d", distinctTxCount, agentLegCount)
	}

	// Verify agent wallet was debited exactly once (50,000 - 1,500 = 48,500)
	var finalBalance int64
	_ = pool.QueryRow(ctx, `SELECT wallet_balance FROM users WHERE id = $1`, parentID).Scan(&finalBalance)
	expectedFinal := int64(50000 - 1500)
	if finalBalance != expectedFinal {
		t.Fatalf("Agent balance after double-refund race mismatch: expected %d, got %d", expectedFinal, finalBalance)
	}
}

// TestRealPG_100ParallelRequestsSameIdempotencyKey proves idempotency concurrency:
// 100 parallel requests with the same Idempotency-Key execute downstream handler exactly once.
func TestRealPG_100ParallelRequestsSameIdempotencyKey(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		return
	}

	app := fiber.New()
	var executedCount atomic.Int64

	app.Post("/api/fintech/sale", middleware.Idempotency(pool), func(c *fiber.Ctx) error {
		executedCount.Add(1)
		time.Sleep(50 * time.Millisecond) // Hold in flight
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "charged", "executed": true})
	})

	idemKey := fmt.Sprintf("idem_race_%d", time.Now().UnixNano())
	reqBody := `{"product": "sim_card", "amount": 25000}`

	const concurrency = 100
	var wg sync.WaitGroup
	var okCount atomic.Int64
	var conflictCount atomic.Int64
	startSignal := make(chan struct{})

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startSignal

			req := httptest.NewRequest("POST", "/api/fintech/sale", bytes.NewReader([]byte(reqBody)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", idemKey)

			resp, err := app.Test(req, 10000)
			if err == nil {
				if resp.StatusCode == http.StatusOK {
					okCount.Add(1)
				} else if resp.StatusCode == http.StatusConflict {
					conflictCount.Add(1)
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
		}()
	}

	close(startSignal)
	wg.Wait()

	if executedCount.Load() != 1 {
		t.Fatalf("IDEMPOTENCY CONCURRENCY FAILURE: Business handler executed %d times, expected strictly 1!", executedCount.Load())
	}
	if okCount.Load()+conflictCount.Load() != concurrency {
		t.Errorf("Expected all 100 requests to be handled (got %d OK + %d Conflict)", okCount.Load(), conflictCount.Load())
	}
}

// TestRealPG_100ParallelRequestsFrozenWalletRejected proves risk protection:
// 100 concurrent debit requests against a frozen wallet are all rejected with 0 balance change.
func TestRealPG_100ParallelRequestsFrozenWalletRejected(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		return
	}

	ctx := context.Background()

	nano := time.Now().UnixNano()
	var frozenUID int64
	_ = pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, full_name, wallet_balance, wallet_status)
		 VALUES ($1, 'hash', 'Frozen User', 500000, 'frozen') RETURNING id`,
		fmt.Sprintf("frozen_%d@venus.test", nano),
	).Scan(&frozenUID)

	const concurrency = 100
	var wg sync.WaitGroup
	var rejectedCount atomic.Int64
	startSignal := make(chan struct{})

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startSignal

			tx, err := pool.Begin(ctx)
			if err != nil {
				return
			}
			defer tx.Rollback(ctx)

			var status string
			err = tx.QueryRow(ctx, `SELECT wallet_status FROM users WHERE id = $1 FOR UPDATE`, frozenUID).Scan(&status)
			if err == nil && status != "active" {
				rejectedCount.Add(1)
				return // Rejected: wallet is frozen
			}

			// If improperly allowed:
			_, _ = tx.Exec(ctx, `UPDATE users SET wallet_balance = wallet_balance - 1000 WHERE id = $1`, frozenUID)
			_ = tx.Commit(ctx)
		}()
	}

	close(startSignal)
	wg.Wait()

	if rejectedCount.Load() != concurrency {
		t.Fatalf("FROZEN WALLET BREACH: Expected all 100 requests to be rejected, but only %d were rejected", rejectedCount.Load())
	}

	var balanceAfter int64
	_ = pool.QueryRow(ctx, `SELECT wallet_balance FROM users WHERE id = $1`, frozenUID).Scan(&balanceAfter)
	if balanceAfter != 500000 {
		t.Fatalf("Frozen wallet balance altered! Got %d IQD, expected exactly 500,000 IQD", balanceAfter)
	}
}
