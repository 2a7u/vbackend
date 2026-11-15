package forecast

import (
	"context"
	"testing"
)

func TestComputeMovingAverage(t *testing.T) {
	values := []int64{100, 200, 300, 400, 500}
	avg := ComputeMovingAverage(values)
	if avg != 300 {
		t.Fatalf("expected avg 300, got %d", avg)
	}

	emptyAvg := ComputeMovingAverage(nil)
	if emptyAvg != 0 {
		t.Fatalf("expected 0 for empty slice, got %d", emptyAvg)
	}
}

func TestGetForecastNilPool(t *testing.T) {
	svc := NewService(nil)
	f, err := svc.GetForecast(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(f.History) != 7 {
		t.Fatalf("expected 7 history points, got %d", len(f.History))
	}
	if len(f.ForecastNextWeek) != 7 {
		t.Fatalf("expected 7 forecast points, got %d", len(f.ForecastNextWeek))
	}
}
