package reconcile

import (
	"bytes"
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"
)

// GenerateExcelXLSX builds a genuine Microsoft Excel (.xlsx) OOXML binary workbook
// with Arabic Right-To-Left (RTL) worksheet view and styled headers.
func GenerateExcelXLSX(report *ReconciliationReport) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	sheetName := "المطابقة المالية"
	defaultSheet := "Sheet1"
	f.SetSheetName(defaultSheet, sheetName)

	// Enable Right-to-Left (RTL) for Arabic layout
	rtl := true
	if err := f.SetSheetView(sheetName, 0, &excelize.ViewOptions{
		RightToLeft: &rtl,
	}); err != nil {
		return nil, fmt.Errorf("set sheet rtl view: %w", err)
	}

	// Create header style
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "#FFFFFF", Size: 12, Family: "Segoe UI"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#1E3A8A"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return nil, fmt.Errorf("create header style: %w", err)
	}

	// Header row
	headers := []string{"البند المحاسبي والمالي", "القيمة (دينار عراقي IQD)", "الملاحظات والتفاصيل"}
	for colIdx, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
		_ = f.SetCellValue(sheetName, cell, h)
		_ = f.SetCellStyle(sheetName, cell, cell, headerStyle)
	}

	// Data rows
	rows := [][]any{
		{"تاريخ المطابقة المحاسبية", report.ReportDate, "بتوقيت بغداد UTC+3"},
		{"واردات بوابة زين كاش", report.TotalGatewayVolume, "المدفوعات الإلكترونية الناجحة"},
		{"الشحن اليدوي والتسويات", report.ManualTopups, "الشحن النقدي المعتمد من الإدارة"},
		{"مبيعات البطاقات الرقمية", report.CardsSold, "سحب بطاقات التعبئة الرقمية"},
		{"المبالغ المسترجعة (Refunds)", report.Refunds, "عمليات الإرجاع المعتمدة إلى المحافظ"},
		{"إجمالي إيداعات دفتر الأستاذ (Credits)", report.TotalLedgerCredits, "إجمالي الحركات الدائنة"},
		{"إجمالي سحوبات دفتر الأستاذ (Debits)", report.TotalLedgerDebits, "إجمالي الحركات المدينة"},
		{"التزام المنصة (إجمالي أرصدة المحافظ)", report.TotalSystemBalance, "رصيد المحافظ الفعلي المعتمد"},
		{"الفارق المحاسبي (Discrepancy)", report.Discrepancy, "الفرق بين الأرصدة ودفتر الأستاذ"},
		{"حالة المطابقة المحاسبية", report.Status, "نتيجة التدقيق الآلي"},
		{"المولد بواسطة", report.GeneratedBy, "النظام الآلي للرقابة المالية"},
	}

	for rowIdx, row := range rows {
		for colIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			_ = f.SetCellValue(sheetName, cell, val)
		}
	}

	// Set column widths
	_ = f.SetColWidth(sheetName, "A", "A", 38)
	_ = f.SetColWidth(sheetName, "B", "B", 25)
	_ = f.SetColWidth(sheetName, "C", "C", 40)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("write xlsx buffer: %w", err)
	}

	return buf.Bytes(), nil
}

// GenerateStatementExcelXLSX builds an OOXML (.xlsx) workbook for subscriber account statements with RTL layout.
func GenerateStatementExcelXLSX(userEmail, userName string, balance, creditLimit int64, entries []map[string]any) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	sheetName := "كشف الحساب"
	defaultSheet := "Sheet1"
	f.SetSheetName(defaultSheet, sheetName)

	rtl := true
	_ = f.SetSheetView(sheetName, 0, &excelize.ViewOptions{
		RightToLeft: &rtl,
	})

	// Title
	_ = f.SetCellValue(sheetName, "A1", fmt.Sprintf("كشف حساب: %s (%s)", userName, userEmail))
	_ = f.SetCellValue(sheetName, "A2", fmt.Sprintf("الرصيد النهائي: %d د.ع | سقف الائتمان: %d د.ع | تاريخ الاستخراج: %s",
		balance, creditLimit, time.Now().Format("2006-01-02 15:04:05")))

	// Headers
	headers := []string{"رقم الحركة (Tx ID)", "التاريخ والوقت", "نوع الحركة", "مدين (Debit)", "دائن (Credit)", "الرصيد بعد الحركة", "الهاش التشفيري"}
	for colIdx, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 4)
		_ = f.SetCellValue(sheetName, cell, h)
	}

	for rowIdx, e := range entries {
		rowNum := rowIdx + 5
		cAtStr := ""
		if t, ok := e["created_at"].(time.Time); ok {
			cAtStr = t.Format("2006-01-02 15:04:05")
		}
		_ = f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowNum), fmt.Sprint(e["tx_id"]))
		_ = f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowNum), cAtStr)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("C%d", rowNum), fmt.Sprint(e["ref_type"]))
		_ = f.SetCellValue(sheetName, fmt.Sprintf("D%d", rowNum), e["debit"])
		_ = f.SetCellValue(sheetName, fmt.Sprintf("E%d", rowNum), e["credit"])
		_ = f.SetCellValue(sheetName, fmt.Sprintf("F%d", rowNum), e["balance_after"])
		_ = f.SetCellValue(sheetName, fmt.Sprintf("G%d", rowNum), fmt.Sprint(e["hash"]))
	}

	_ = f.SetColWidth(sheetName, "A", "A", 38)
	_ = f.SetColWidth(sheetName, "B", "B", 20)
	_ = f.SetColWidth(sheetName, "C", "C", 25)
	_ = f.SetColWidth(sheetName, "D", "F", 18)
	_ = f.SetColWidth(sheetName, "G", "G", 66)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("write statement xlsx: %w", err)
	}

	return buf.Bytes(), nil
}

// GeneratePDF generates a valid PDF 1.4 binary document for a reconciliation report.
// Conforms strictly to ISO 32000-1 specification with valid magic header %PDF-1.4,
// xref table, catalog, page objects, and %%EOF marker.
func GeneratePDF(report *ReconciliationReport) []byte {
	var buf bytes.Buffer

	// PDF header
	buf.WriteString("%PDF-1.4\n")
	buf.WriteString("%\xe2\xe3\xcf\xd3\n") // Binary comment

	var offsets []int

	// Object 1: Catalog
	offsets = append(offsets, buf.Len())
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	// Object 2: Pages
	offsets = append(offsets, buf.Len())
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")

	// Object 3: Page
	offsets = append(offsets, buf.Len())
	buf.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n")

	// Stream contents
	streamContent := fmt.Sprintf(
		"BT\n/F1 16 Tf\n50 780 Td\n(VENUS POS - DAILY RECONCILIATION REPORT) Tj\n"+
			"/F1 12 Tf\n0 -30 Td\n(Report Date: %s | Baghdad UTC+3) Tj\n"+
			"0 -20 Td\n(Status: %s | Discrepancy: %d IQD) Tj\n"+
			"0 -30 Td\n(Total System Balance: %d IQD) Tj\n"+
			"0 -20 Td\n(Total Gateway Volume: %d IQD) Tj\n"+
			"0 -20 Td\n(Manual Topups: %d IQD) Tj\n"+
			"0 -20 Td\n(Cards Sold: %d IQD) Tj\n"+
			"0 -20 Td\n(Refunds Reversed: %d IQD) Tj\n"+
			"0 -20 Td\n(Total Ledger Credits: %d IQD) Tj\n"+
			"0 -20 Td\n(Total Ledger Debits: %d IQD) Tj\n"+
			"0 -30 Td\n(Generated By: %s) Tj\nET\n",
		report.ReportDate, report.Status, report.Discrepancy,
		report.TotalSystemBalance, report.TotalGatewayVolume,
		report.ManualTopups, report.CardsSold, report.Refunds,
		report.TotalLedgerCredits, report.TotalLedgerDebits,
		report.GeneratedBy,
	)

	// Object 4: Contents Stream
	offsets = append(offsets, buf.Len())
	buf.WriteString(fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(streamContent), streamContent))

	// Object 5: Standard Base-14 Font (Helvetica)
	offsets = append(offsets, buf.Len())
	buf.WriteString("5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")

	// Cross-Reference Table
	xrefOffset := buf.Len()
	buf.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for _, off := range offsets {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}

	// Trailer
	buf.WriteString("trailer\n<< /Size 6 /Root 1 0 R >>\n")
	buf.WriteString(fmt.Sprintf("startxref\n%d\n%%%%EOF\n", xrefOffset))

	return buf.Bytes()
}

// GenerateStatementPDF generates a valid PDF 1.4 binary document for a subscriber statement.
func GenerateStatementPDF(userEmail, userName string, balance, creditLimit int64, entriesCount int) []byte {
	var buf bytes.Buffer

	buf.WriteString("%PDF-1.4\n")
	buf.WriteString("%\xe2\xe3\xcf\xd3\n")

	var offsets []int

	// 1: Catalog
	offsets = append(offsets, buf.Len())
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	// 2: Pages
	offsets = append(offsets, buf.Len())
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")

	// 3: Page
	offsets = append(offsets, buf.Len())
	buf.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n")

	// Stream
	content := fmt.Sprintf(
		"BT\n/F1 16 Tf\n50 780 Td\n(VENUS POS - ACCOUNT STATEMENT) Tj\n"+
			"/F1 12 Tf\n0 -30 Td\n(User: %s <%s>) Tj\n"+
			"0 -20 Td\n(Current Balance: %d IQD | Credit Limit: %d IQD) Tj\n"+
			"0 -20 Td\n(Total Verified Ledger Entries: %d) Tj\n"+
			"0 -20 Td\n(Statement Generated: %s UTC) Tj\nET\n",
		userName, userEmail, balance, creditLimit, entriesCount, time.Now().UTC().Format(time.RFC3339),
	)

	// 4: Stream
	offsets = append(offsets, buf.Len())
	buf.WriteString(fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content), content))

	// 5: Font
	offsets = append(offsets, buf.Len())
	buf.WriteString("5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")

	// Xref
	xrefOffset := buf.Len()
	buf.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for _, off := range offsets {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}

	buf.WriteString("trailer\n<< /Size 6 /Root 1 0 R >>\n")
	buf.WriteString(fmt.Sprintf("startxref\n%d\n%%%%EOF\n", xrefOffset))

	return buf.Bytes()
}
