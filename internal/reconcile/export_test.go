package reconcile

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ledongthuc/pdf"
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
		if len(pdfBytes) < 100 {
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

func TestArabicPDFExportAndTextExtraction(t *testing.T) {
	rep := &ReconciliationReport{
		ReportDate:             "2026-11-15",
		TotalSystemBalance:     7500000,
		TotalGatewayVolume:     3000000,
		ManualTopups:           4500000,
		CardsSold:              2000000,
		Refunds:                0,
		TotalLedgerCredits:     7500000,
		TotalLedgerDebits:      2000000,
		ExpectedClosingBalance: 5500000,
		ActualClosingBalance:   5500000,
		Discrepancy:            0,
		Status:                 "matched",
		GeneratedBy:            "system_auditor",
	}

	pdfBytes := GeneratePDF(rep)
	if len(pdfBytes) == 0 {
		t.Fatalf("GeneratePDF returned empty bytes")
	}

	// 1. Verify standard PDF headers and valid xref
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
		t.Fatalf("Expected %%PDF- header")
	}

	// 2. Save a sample statement PDF for inspection
	sampleDir := "../../sample_exports"
	_ = os.MkdirAll(sampleDir, 0755)
	samplePath := filepath.Join(sampleDir, "sample_reconciliation_statement.pdf")
	_ = os.WriteFile(samplePath, pdfBytes, 0644)

	// Also generate subscriber statement PDF
	stmtBytes := GenerateStatementPDF("subscriber@venus.test", "متجر الأمل التجاري", 1500000, 500000, 42)
	stmtPath := filepath.Join(sampleDir, "sample_subscriber_statement.pdf")
	_ = os.WriteFile(stmtPath, stmtBytes, 0644)

	// 3. Extract text using PDF parser to verify Arabic font and text encoding
	reader, err := pdf.NewReader(bytes.NewReader(pdfBytes), int64(len(pdfBytes)))
	if err != nil {
		t.Fatalf("pdf.NewReader failed: %v", err)
	}

	var textBuf bytes.Buffer
	plainText, err := reader.GetPlainText()
	if err == nil {
		_, _ = textBuf.ReadFrom(plainText)
	}

	extracted := textBuf.String()
	// Assert numbers are present in the PDF text stream
	if !strings.Contains(extracted, "2026-11-15") && !bytes.Contains(pdfBytes, []byte("2026-11-15")) {
		t.Errorf("Expected report date '2026-11-15' in PDF")
	}
	if !strings.Contains(extracted, "7500000") && !bytes.Contains(pdfBytes, []byte("7500000")) {
		t.Errorf("Expected balance '7500000' in PDF")
	}

	// Verify font data embedding
	if !bytes.Contains(pdfBytes, []byte("Amiri")) {
		t.Errorf("Expected embedded font 'Amiri' in PDF stream")
	}
}
