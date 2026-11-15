package pricing_test

import (
	"context"
	"testing"
	"venus_backend/internal/pricing"
)

func TestPricingResolutionOrderFallbackToCatalog(t *testing.T) {
	svc := pricing.NewService(nil) // nil pool simulates no DB records, strictly testing fallback & cache

	res, err := svc.ResolvePrice(context.Background(), 101, "asia_5000", 5000)
	if err != nil {
		t.Fatalf("resolve price failed: %v", err)
	}

	if res.Price != 5000 {
		t.Fatalf("expected price 5000, got %d", res.Price)
	}
	if res.Source != "catalog_default" {
		t.Fatalf("expected source catalog_default, got %s", res.Source)
	}
}

func TestPricingProfitComputation(t *testing.T) {
	costPrice := int64(4500)
	sellPrice := int64(5000)

	profit := pricing.ComputeProfit(costPrice, sellPrice)
	if profit != 500 {
		t.Fatalf("expected net profit 500 IQD, got %d", profit)
	}

	// Loss / break-even check
	breakEven := pricing.ComputeProfit(5000, 5000)
	if breakEven != 0 {
		t.Fatalf("expected 0 profit, got %d", breakEven)
	}
}
