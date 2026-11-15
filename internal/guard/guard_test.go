package guard

import (
	"errors"
	"testing"
)

type mockGuardCheck struct {
	costPrice   int64
	minPercent  int
	minIQD      int64
	sellPrice   int64
	expectError bool
}

func TestGuardLogicCalculations(t *testing.T) {
	tests := []struct {
		name        string
		cost        int64
		minIQD      int64
		minPct      int
		sell        int64
		expectBelow bool
	}{
		{
			name:        "sell equal to cost without margin",
			cost:        10000,
			minIQD:      0,
			minPct:      0,
			sell:        10000,
			expectBelow: false,
		},
		{
			name:        "sell below cost",
			cost:        10000,
			minIQD:      0,
			minPct:      0,
			sell:        9999,
			expectBelow: true,
		},
		{
			name:        "sell below cost plus IQD margin",
			cost:        10000,
			minIQD:      500,
			minPct:      0,
			sell:        10400,
			expectBelow: true,
		},
		{
			name:        "sell at or above cost plus IQD margin",
			cost:        10000,
			minIQD:      500,
			minPct:      0,
			sell:        10500,
			expectBelow: false,
		},
		{
			name:        "sell below cost plus percentage margin",
			cost:        10000,
			minIQD:      0,
			minPct:      10, // requires 11,000
			sell:        10900,
			expectBelow: true,
		},
		{
			name:        "sell above cost plus percentage margin",
			cost:        10000,
			minIQD:      0,
			minPct:      10, // requires 11,000
			sell:        11000,
			expectBelow: false,
		},
		{
			name:        "takes the maximum of IQD and percent margin",
			cost:        10000,
			minIQD:      1500, // 1500 > 1000
			minPct:      10,   // 1000
			sell:        11200,
			expectBelow: true, // requires 11500
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pctMargin := (tt.cost * int64(tt.minPct)) / 100
			effMargin := pctMargin
			if tt.minIQD > effMargin {
				effMargin = tt.minIQD
			}
			minAllowed := tt.cost + effMargin
			isBelow := tt.sell < minAllowed
			if isBelow != tt.expectBelow {
				t.Fatalf("expected below=%v, got %v for cost=%d, minIQD=%d, minPct=%d, sell=%d",
					tt.expectBelow, isBelow, tt.cost, tt.minIQD, tt.minPct, tt.sell)
			}
		})
	}
}

func TestGuardErrors(t *testing.T) {
	if !errors.Is(ErrPriceBelowCost, ErrPriceBelowCost) {
		t.Fatalf("ErrPriceBelowCost should match itself")
	}
}
