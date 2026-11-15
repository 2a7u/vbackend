package hierarchy_test

import (
	"context"
	"testing"
	"venus_backend/internal/hierarchy"
)

func TestCalculateCommissionAmountFixedAndPercentage(t *testing.T) {
	// 1. Percentage: 250 basis points (2.50%) on 100,000 IQD sale, cost = 90,000 IQD (margin = 10,000)
	saleAmt := int64(100000)
	costPrice := int64(90000)
	commPct, capped := hierarchy.CalculateCommissionAmount("percentage", 250, 0, saleAmt, costPrice, true)
	if commPct != 2500 {
		t.Fatalf("expected 2,500 IQD commission, got %d", commPct)
	}
	if capped {
		t.Fatalf("expected capped=false when commission is well within margin")
	}

	// 2. Fixed amount: 1,500 IQD fixed on 100,000 IQD sale, cost = 95,000 IQD (margin = 5,000)
	commFixed, capped := hierarchy.CalculateCommissionAmount("fixed", 0, 1500, saleAmt, costPrice, true)
	if commFixed != 1500 {
		t.Fatalf("expected 1,500 IQD commission, got %d", commFixed)
	}
	if capped {
		t.Fatalf("expected capped=false when commission is within margin")
	}

	// 3. Margin capping test: Rule wants 5,000 IQD fixed commission, but sale is 100,000 and cost is 98,000 (margin = 2,000 IQD)
	// Platform MUST NOT pay more than 2,000 IQD!
	commCapped, wasCapped := hierarchy.CalculateCommissionAmount("fixed", 0, 5000, 100000, 98000, true)
	if !wasCapped {
		t.Fatalf("expected wasCapped=true when commission exceeds margin")
	}
	if commCapped != 2000 {
		t.Fatalf("expected commission capped at exact margin of 2,000 IQD, got %d", commCapped)
	}

	// 4. Zero margin or sale at loss: sale is 100,000 and cost is 105,000 (loss of 5,000)
	// Platform MUST pay 0 IQD commission!
	commLoss, wasCappedLoss := hierarchy.CalculateCommissionAmount("fixed", 0, 1000, 100000, 105000, true)
	if !wasCappedLoss {
		t.Fatalf("expected wasCappedLoss=true on loss sale")
	}
	if commLoss != 0 {
		t.Fatalf("expected 0 IQD commission on sale below cost, got %d", commLoss)
	}

	// 5. Half-up integer rounding test: 133 basis points on 10,000 IQD = 133 IQD
	// (10000 * 133 + 5000) / 10000 = (1330000 + 5000) / 10000 = 1335000 / 10000 = 133
	commRound, _ := hierarchy.CalculateCommissionAmount("percentage", 133, 0, 10000, 5000, false)
	if commRound != 133 {
		t.Fatalf("expected 133 IQD rounded commission, got %d", commRound)
	}
}

func TestHierarchySelfParentingPrevention(t *testing.T) {
	svc := hierarchy.NewService(nil, nil)

	// User cannot be their own parent
	err := svc.CheckCyclePreventsInfiniteLoop(context.Background(), 42, 42)
	if err != hierarchy.ErrSelfParenting {
		t.Fatalf("expected ErrSelfParenting, got %v", err)
	}
}
