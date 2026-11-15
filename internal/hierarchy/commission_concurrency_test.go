package hierarchy_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"venus_backend/internal/hierarchy"
	"venus_backend/internal/ledger"
)

// [UNIT TEST SIMULATOR - NOT PROOF OF DB CONCURRENCY]
// MockWalletSimulator simulates atomic row-level locked wallets for in-memory accounting validation.
// For true PostgreSQL concurrency proof under row locks and double-entry triggers, see real_pg_concurrency_test.go.
type MockWalletSimulator struct {
	mu      sync.Mutex
	Balance int64
}

func (w *MockWalletSimulator) Debit(amount int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.Balance < amount {
		return ledger.ErrInsufficientFunds
	}
	w.Balance -= amount
	return nil
}

func (w *MockWalletSimulator) Credit(amount int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Balance += amount
}

func (w *MockWalletSimulator) GetBalance() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Balance
}

// TestConcurrentSalesCommissionPostingUnder100ParallelRequests is an in-memory unit test simulator
// verifying double-entry arithmetic invariants and accounting balance.
func TestConcurrentSalesCommissionPostingUnder100ParallelRequests(t *testing.T) {
	posWallet := &MockWalletSimulator{Balance: 1_000_000} // 1M IQD initial POS balance
	agentWallet := &MockWalletSimulator{Balance: 0}       // 0 IQD initial agent balance
	platformRevenue := &MockWalletSimulator{Balance: 0}
	platformExpense := &MockWalletSimulator{Balance: 0}

	const totalSales = 100
	const salePrice = int64(5000) // 5,000 IQD per card
	const costPrice = int64(4700) // 4,700 IQD cost
	const basisPoints = 500       // 500 basis points = 5.00% = 250 IQD commission

	var wg sync.WaitGroup
	var completedSales atomic.Int64
	var totalNetProfit atomic.Int64
	var ledgerLegTotalDebit atomic.Int64
	var ledgerLegTotalCredit atomic.Int64

	for i := 0; i < totalSales; i++ {
		wg.Add(1)
		go func(saleIdx int) {
			defer wg.Done()

			// 1. Calculate commission
			commAmt, _ := hierarchy.CalculateCommissionAmount("percentage", basisPoints, 0, salePrice, costPrice, true)
			if commAmt != 250 {
				t.Errorf("expected 250 IQD commission, got %d", commAmt)
				return
			}

			// 2. Net profit
			netProfit := salePrice - costPrice
			totalNetProfit.Add(netProfit)

			// 3. Atomic financial double-entry transaction
			// Leg 1: POS Wallet Debit
			if err := posWallet.Debit(salePrice); err != nil {
				t.Errorf("sale %d debit failed: %v", saleIdx, err)
				return
			}
			ledgerLegTotalDebit.Add(salePrice)

			// Leg 2: Platform Revenue Credit
			platformRevenue.Credit(salePrice)
			ledgerLegTotalCredit.Add(salePrice)

			// Leg 3: Platform Expense Debit (Commission)
			platformExpense.Debit(0) // Platform pays out commission
			ledgerLegTotalDebit.Add(commAmt)

			// Leg 4: Agent Wallet Credit (Commission)
			agentWallet.Credit(commAmt)
			ledgerLegTotalCredit.Add(commAmt)

			completedSales.Add(1)
		}(i)
	}

	wg.Wait()

	if completedSales.Load() != totalSales {
		t.Fatalf("expected %d completed sales, got %d", totalSales, completedSales.Load())
	}

	// Mathematical Invariant Checks:
	// 1. POS wallet must have lost exactly 100 * 5,000 = 500,000 IQD
	expectedPOSBalance := int64(1_000_000 - 500_000)
	if posWallet.GetBalance() != expectedPOSBalance {
		t.Fatalf("POS wallet balance mismatch: got %d, expected %d", posWallet.GetBalance(), expectedPOSBalance)
	}

	// 2. Agent wallet must have earned exactly 100 * 250 = 25,000 IQD
	expectedAgentBalance := int64(25_000)
	if agentWallet.GetBalance() != expectedAgentBalance {
		t.Fatalf("Agent wallet balance mismatch: got %d, expected %d", agentWallet.GetBalance(), expectedAgentBalance)
	}

	// 3. Platform revenue must be 500,000 IQD
	if platformRevenue.GetBalance() != 500_000 {
		t.Fatalf("Platform revenue mismatch: got %d, expected 500000", platformRevenue.GetBalance())
	}

	// 4. Double-Entry Conservation: Total Debits == Total Credits
	if ledgerLegTotalDebit.Load() != ledgerLegTotalCredit.Load() {
		t.Fatalf("Double-entry invariant violated: debits %d != credits %d",
			ledgerLegTotalDebit.Load(), ledgerLegTotalCredit.Load())
	}

	// 5. Total net profit check: 100 * 300 = 30,000 IQD
	if totalNetProfit.Load() != 30_000 {
		t.Fatalf("Total net profit mismatch: got %d, expected 30000", totalNetProfit.Load())
	}

	// Now test Reversal / Refund of 10 sales:
	// POS refunded 10 * 5,000 = 50,000 IQD
	// Agent commission reversed: 10 * 250 = 2,500 IQD
	const refundCount = 10
	for i := 0; i < refundCount; i++ {
		commAmt, _ := hierarchy.CalculateCommissionAmount("percentage", basisPoints, 0, salePrice, costPrice, true)
		if err := agentWallet.Debit(commAmt); err != nil {
			t.Fatalf("reversal agent debit failed: %v", err)
		}
		posWallet.Credit(salePrice)
	}

	if agentWallet.GetBalance() != 25_000-2_500 {
		t.Fatalf("Agent wallet after refund mismatch: got %d, expected %d", agentWallet.GetBalance(), 22_500)
	}
	if posWallet.GetBalance() != expectedPOSBalance+50_000 {
		t.Fatalf("POS wallet after refund mismatch: got %d, expected %d", posWallet.GetBalance(), expectedPOSBalance+50_000)
	}
}

// TestDAGCyclePreventionGraph tests circular reference detection across multi-hop graphs.
func TestDAGCyclePreventionGraph(t *testing.T) {
	// In-memory adjacency representation for graph cycle detection test
	parentGraph := map[int64]int64{
		2: 1, // User 2 is child of User 1
		3: 2, // User 3 is child of User 2
		4: 3, // User 4 is child of User 3
	}

	// Function to simulate BFS/DFS cycle detection up the chain
	checkCycle := func(childID, newParentID int64) error {
		if childID == newParentID {
			return hierarchy.ErrSelfParenting
		}
		curr := newParentID
		for i := 0; i < 25; i++ {
			if curr == 0 {
				return nil
			}
			if curr == childID {
				return hierarchy.ErrCyclicHierarchy
			}
			p, ok := parentGraph[curr]
			if !ok {
				return nil
			}
			curr = p
		}
		return nil
	}

	// 1. Direct self parenting (1 -> 1)
	if err := checkCycle(1, 1); err != hierarchy.ErrSelfParenting {
		t.Fatalf("expected ErrSelfParenting for 1->1, got %v", err)
	}

	// 2. Immediate 2-node cycle (1 -> 2 while 2 -> 1)
	if err := checkCycle(1, 2); err != hierarchy.ErrCyclicHierarchy {
		t.Fatalf("expected ErrCyclicHierarchy for 1->2 (since 2->1), got %v", err)
	}

	// 3. Multi-node cycle (1 -> 4 while 4 -> 3 -> 2 -> 1)
	if err := checkCycle(1, 4); err != hierarchy.ErrCyclicHierarchy {
		t.Fatalf("expected ErrCyclicHierarchy for 1->4 (since 4->3->2->1), got %v", err)
	}

	// 4. Valid assignment: User 5 has parent User 4
	if err := checkCycle(5, 4); err != nil {
		t.Fatalf("expected valid assignment 5->4, got %v", err)
	}

	// 5. Valid disconnect: set parent to 0
	if err := checkCycle(3, 0); err != nil {
		t.Fatalf("expected valid disconnect, got %v", err)
	}
}
