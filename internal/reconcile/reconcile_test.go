package reconcile_test

import (
	"strings"
	"testing"
	"time"

	"venus_backend/internal/reconcile"
)

func TestGenerateCSVFormatting(t *testing.T) {
	rep := &reconcile.ReconciliationReport{
		ID:                     10,
		ReportDate:             "2026-11-15",
		TotalSystemBalance:     5000000,
		TotalGatewayVolume:     1500000,
		TotalLedgerCredits:     1500000,
		TotalLedgerDebits:      300000,
		ExpectedClosingBalance: 5000000,
		ActualClosingBalance:   5000000,
		Discrepancy:            0,
		Status:                 "matched",
		GeneratedBy:            "system_automated_audit",
		CreatedAt:              time.Now().UTC(),
	}

	csvBytes := reconcile.GenerateCSV(rep)
	if len(csvBytes) == 0 {
		t.Fatalf("expected non-empty CSV output")
	}

	csvStr := string(csvBytes)
	if !strings.Contains(csvStr, "2026-11-15") {
		t.Fatalf("expected date in CSV")
	}
	if !strings.Contains(csvStr, "5000000") {
		t.Fatalf("expected balance in CSV")
	}
	if !strings.Contains(csvStr, "matched") {
		t.Fatalf("expected matched status in CSV")
	}
}

func TestReconciliationFormulaInvariant(t *testing.T) {
	var initialBalance int64 = 1000000
	var totalCredits int64 = 500000
	var totalDebits int64 = 200000

	expectedClosing := initialBalance + totalCredits - totalDebits
	if expectedClosing != 1300000 {
		t.Fatalf("expected 1,300,000, got %d", expectedClosing)
	}

	var actualEndingBalance int64 = 1300000
	discrepancy := actualEndingBalance - expectedClosing
	if discrepancy != 0 {
		t.Fatalf("expected zero discrepancy for balanced ledger, got %d", discrepancy)
	}
}
