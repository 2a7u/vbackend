package radar_test

import (
	"strings"
	"testing"
	"venus_backend/internal/radar"
)

func TestRadarNoSalesHandlesZeroDivisionGracefully(t *testing.T) {
	availableStock := 100
	sales1h := 0
	sales6h := 0
	sales24h := 0

	_, _, _, estHours, estText := radar.ComputeExhaustionEstimate(availableStock, sales1h, sales6h, sales24h)

	if estHours != -1 {
		t.Fatalf("expected -1 estimated hours for zero sales, got %f", estHours)
	}

	if !strings.Contains(estText, "مستقر") {
		t.Fatalf("expected stable stock message, got %q", estText)
	}
}

func TestRadarBurnRateAndExhaustionCalculation(t *testing.T) {
	availableStock := 20
	sales1h := 10  // 10/hr
	sales6h := 30  // 5/hr
	sales24h := 72 // 3/hr

	_, _, _, estHours, estText := radar.ComputeExhaustionEstimate(availableStock, sales1h, sales6h, sales24h)

	if estHours <= 0 {
		t.Fatalf("expected positive estimated hours, got %f", estHours)
	}

	if !strings.Contains(estText, "ينفد خلال") {
		t.Fatalf("expected exhaustion warning message, got %q", estText)
	}
}

func TestRadarZeroStockExhaustion(t *testing.T) {
	availableStock := 0
	sales1h := 5
	sales6h := 10
	sales24h := 20

	_, _, _, estHours, estText := radar.ComputeExhaustionEstimate(availableStock, sales1h, sales6h, sales24h)

	if estHours != 0 {
		t.Fatalf("expected 0 estimated hours for zero stock, got %f", estHours)
	}

	if !strings.Contains(estText, "نفد") {
		t.Fatalf("expected out-of-stock message, got %q", estText)
	}
}
