package reconcile

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestExportValidBinaryHeaders(t *testing.T) {
	rep := &ReconciliationReport{
		ReportDate:             "2026-11-15",
		TotalSystemBalance:     5000000,
		TotalGatewayVolume:     2000000,
		ManualTopups:           3000000,
		CardsSold:              1000000,
		Refunds:                0,
		TotalLedgerCredits:     5000000,
		TotalLedgerDebits:      1000000,
		ExpectedClosingBalance: 4000000,
		ActualClosingBalance:   4000000,
		Discrepancy:            0,
		Status:                 "matched",
		GeneratedBy:            "test_runner",
	}

	t.Run("Excel OOXML XLSX Valid Structure", func(t *testing.T) {
		xlsxBytes, err := GenerateExcelXLSX(rep)
		if err != nil {
			t.Fatalf("GenerateExcelXLSX failed: %v", err)
		}

		if len(xlsxBytes) < 4 {
			t.Fatalf("Generated XLSX too short: %d bytes", len(xlsxBytes))
		}

		// Verify standard PK zip magic header (0x50, 0x4B, 0x03, 0x04)
		expectedPK := []byte{0x50, 0x4B, 0x03, 0x04}
		if !bytes.Equal(xlsxBytes[:4], expectedPK) {
			t.Errorf("XLSX does not start with PK ZIP header! Got %x, want %x", xlsxBytes[:4], expectedPK)
		}

		// Verify the binary can be parsed as a valid Excel workbook by excelize
		f, err := excelize.OpenReader(bytes.NewReader(xlsxBytes))
		if err != nil {
			t.Fatalf("Failed to open generated XLSX with excelize: %v", err)
		}
		defer f.Close()

		sheetList := f.GetSheetList()
		if len(sheetList) == 0 {
			t.Errorf("XLSX has no sheets")
		}
		val, _ := f.GetCellValue(sheetList[0], "B2")
		if val != "2026-11-15" {
			t.Errorf("Expected cell B2 to be 2026-11-15, got %s", val)
		}
	})

	t.Run("PDF Valid ISO Header and EOF", func(t *testing.T) {
		pdfBytes := GeneratePDF(rep)
		if len(pdfBytes) < 20 {
			t.Fatalf("Generated PDF too short: %d bytes", len(pdfBytes))
		}

		// Verify %PDF- magic header
		if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
			t.Errorf("PDF does not start with %%PDF- magic header! Got %q", string(pdfBytes[:10]))
		}

		// Verify %%EOF marker
		if !bytes.Contains(pdfBytes, []byte("%%EOF")) {
			t.Errorf("PDF does not contain %%%%EOF marker!")
		}
	})
}
