package risk

import (
	"testing"
)

func TestComputeRiskScore(t *testing.T) {
	// Normal daytime low velocity: score = 0
	score, flags := ComputeRisk(14, 2, 0, false)
	if score != 0 || len(flags) != 0 {
		t.Fatalf("expected score 0 and no flags, got %d, flags: %v", score, flags)
	}

	// Off-hours + high velocity + failed attempts
	score, flags = ComputeRisk(3, 20, 6, true)
	// 20 (off-hours) + 30 (velocity) + 30 (failures) + 25 (abnormal) = 105 -> clamped to 100
	if score != 100 {
		t.Fatalf("expected clamped score 100, got %d", score)
	}
	if len(flags) != 4 {
		t.Fatalf("expected 4 triggered flags, got %d", len(flags))
	}
}
