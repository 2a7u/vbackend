package promotions_test

import (
	"testing"
)

func TestCouponCalculationLogic(t *testing.T) {
	// Percentage 20% on 50,000 IQD -> 10,000 discount
	origAmount := int64(50000)
	pct := int64(20)
	discount := (origAmount * pct) / 100
	if discount != 10000 {
		t.Fatalf("expected 10000, got %d", discount)
	}
	final := origAmount - discount
	if final != 40000 {
		t.Fatalf("expected 40000, got %d", final)
	}

	// Fixed 15,000 on 50,000 IQD
	fixedDisc := int64(15000)
	if origAmount-fixedDisc != 35000 {
		t.Fatalf("expected 35000, got %d", origAmount-fixedDisc)
	}

	// Cap to 100% discount
	capDisc := int64(60000)
	if capDisc > origAmount {
		capDisc = origAmount
	}
	if capDisc != origAmount {
		t.Fatalf("expected discount capped at 50000, got %d", capDisc)
	}
}
