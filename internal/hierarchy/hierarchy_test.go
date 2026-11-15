package hierarchy_test

import (
	"context"
	"testing"
	"venus_backend/internal/hierarchy"
)

func TestCalculateCommissionAmountFixedAndPercentage(t *testing.T) {
	// 1. Percentage: 2.5% on 100,000 IQD sale
	saleAmt := int64(100000)
	commPct := hierarchy.CalculateCommissionAmount("percentage", 2.5, saleAmt)
	if commPct != 2500 {
		t.Fatalf("expected 2,500 IQD commission, got %d", commPct)
	}

	// 2. Fixed amount: 1,500 IQD fixed on any sale
	commFixed := hierarchy.CalculateCommissionAmount("fixed", 1500, saleAmt)
	if commFixed != 1500 {
		t.Fatalf("expected 1,500 IQD commission, got %d", commFixed)
	}

	// 3. Zero sale amount
	commZero := hierarchy.CalculateCommissionAmount("percentage", 5.0, 0)
	if commZero != 0 {
		t.Fatalf("expected 0 commission on 0 sale, got %d", commZero)
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
