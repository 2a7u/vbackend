package rewards

import (
	"strings"
	"testing"
	"time"
)

func TestRewardsPeriodKeyGeneration(t *testing.T) {
	testTime := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

	weeklyKey := CurrentPeriodKey("weekly", testTime)
	if !strings.HasPrefix(weeklyKey, "2026-W") {
		t.Fatalf("expected weekly prefix 2026-W, got %s", weeklyKey)
	}

	monthlyKey := CurrentPeriodKey("monthly", testTime)
	if monthlyKey != "2026-11" {
		t.Fatalf("expected monthly key 2026-11, got %s", monthlyKey)
	}

	dailyKey := CurrentPeriodKey("daily", testTime)
	if dailyKey != "2026-11-16" {
		t.Fatalf("expected daily key 2026-11-16, got %s", dailyKey)
	}
}
