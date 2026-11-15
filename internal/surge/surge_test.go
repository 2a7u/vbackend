package surge

import (
	"testing"
)

func TestSurgePricingCalculations(t *testing.T) {
	// Disabled by default: price remains unchanged
	q1 := ComputeSurge(10000, false, 10, 15, "Peak hours")
	if q1.IsSurgeOn || q1.FinalPrice != 10000 || q1.SurgeAmount != 0 {
		t.Fatalf("expected surge off, got %+v", q1)
	}

	// Enabled +5%
	q2 := ComputeSurge(10000, true, 5, 15, "Weekend rush")
	if !q2.IsSurgeOn || q2.FinalPrice != 10500 || q2.SurgeAmount != 500 {
		t.Fatalf("expected +500 IQD surge, got %+v", q2)
	}

	// Cap enforcement: multiplier 25% capped at 15%
	q3 := ComputeSurge(10000, true, 25, 15, "High demand")
	if q3.SurgePercent != 15 || q3.FinalPrice != 11500 || q3.SurgeAmount != 1500 {
		t.Fatalf("expected capped at 15%% (11,500 IQD), got %+v", q3)
	}
}
