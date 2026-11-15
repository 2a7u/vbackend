package reports

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestReportsPDFGeneration(t *testing.T) {
	svc := NewService(nil, nil)
	summary, err := svc.GenerateDailySummary(context.Background(), 1, time.Now())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	pdf := svc.GeneratePDFReport(summary)
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.4")) {
		t.Fatal("expected PDF header %PDF-1.4")
	}
	if !bytes.Contains(pdf, []byte("Total Sales:")) {
		t.Fatal("expected Total Sales in PDF contents")
	}
}
