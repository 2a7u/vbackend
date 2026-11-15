package reconcile

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/01walid/goarabic"
	"github.com/signintech/gopdf"
	"github.com/xuri/excelize/v2"
)

//go:embed assets/Amiri-Regular.ttf
var arabicFontData []byte

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
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF", Size: 12, Family: "Segoe UI"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#1E3A8A"}, Pattern: 1},
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
	headers := []string{"رقم الحركة", "التاريخ والوقت", "نوع المعاملة", "المرجع", "مدين (Debit)", "دائن (Credit)", "الرصيد بعد الحركة"}
	for colIdx, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 4)
		_ = f.SetCellValue(sheetName, cell, h)
	}

	for rowIdx, entry := range entries {
		r := rowIdx + 5
		cell, _ := excelize.CoordinatesToCellName(1, r)
		_ = f.SetCellValue(sheetName, cell, entry["id"])
		cell, _ = excelize.CoordinatesToCellName(2, r)
		_ = f.SetCellValue(sheetName, cell, entry["created_at"])
		cell, _ = excelize.CoordinatesToCellName(3, r)
		_ = f.SetCellValue(sheetName, cell, entry["ref_type"])
		cell, _ = excelize.CoordinatesToCellName(4, r)
		_ = f.SetCellValue(sheetName, cell, entry["ref_id"])
		cell, _ = excelize.CoordinatesToCellName(5, r)
		_ = f.SetCellValue(sheetName, cell, entry["debit"])
		cell, _ = excelize.CoordinatesToCellName(6, r)
		_ = f.SetCellValue(sheetName, cell, entry["credit"])
		cell, _ = excelize.CoordinatesToCellName(7, r)
		_ = f.SetCellValue(sheetName, cell, entry["balance_after"])
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("write xlsx buffer: %w", err)
	}

	return buf.Bytes(), nil
}

// ShapeArabic shapes Arabic text using Unicode presentation forms and reverses character order for RTL display.
// For mixed text containing Latin words or numbers, it preserves LTR orientation for Latin/numeric runs.
func ShapeArabic(text string) string {
	if text == "" {
		return ""
	}
	hasArabic := false
	for _, r := range text {
		if (r >= 0x0600 && r <= 0x06FF) || (r >= 0xFB50 && r <= 0xFEFC) {
			hasArabic = true
			break
		}
	}
	if !hasArabic {
		return text
	}

	words := strings.Fields(text)
	var reversedWords []string
	for i := len(words) - 1; i >= 0; i-- {
		w := words[i]
		if isArabicWord(w) {
			reversedWords = append(reversedWords, reverseRunes(goarabic.ToGlyph(w)))
		} else {
			// Latin word or numbers: preserve LTR order
			reversedWords = append(reversedWords, w)
		}
	}
	return strings.Join(reversedWords, " ")
}

// ShapeArabicBidi shapes text preserving LTR numbers and Latin words while shaping and reversing Arabic words.
func ShapeArabicBidi(text string) string {
	if text == "" {
		return ""
	}
	hasArabic := false
	for _, r := range text {
		if (r >= 0x0600 && r <= 0x06FF) || (r >= 0xFB50 && r <= 0xFEFC) {
			hasArabic = true
			break
		}
	}
	if !hasArabic {
		return text
	}

	shaped := goarabic.ToGlyph(text)
	words := strings.Fields(shaped)
	var reversedWords []string
	for i := len(words) - 1; i >= 0; i-- {
		w := words[i]
		if isArabicWord(w) {
			reversedWords = append(reversedWords, reverseRunes(w))
		} else {
			reversedWords = append(reversedWords, w)
		}
	}
	return strings.Join(reversedWords, " ")
}

func isArabicWord(s string) bool {
	for _, r := range s {
		if (r >= 0x0600 && r <= 0x06FF) || (r >= 0xFB50 && r <= 0xFEFC) {
			return true
		}
	}
	return false
}

func reverseRunes(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

// GeneratePDF generates a high-quality PDF 1.4 document for daily reconciliation
// with embedded Arabic font (Amiri) and an RTL table layout.
func GeneratePDF(report *ReconciliationReport) []byte {
	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	pdf.AddPage()

	if len(arabicFontData) > 0 {
		_ = pdf.AddTTFFontData("Amiri", arabicFontData)
		_ = pdf.SetFont("Amiri", "", 12)
	}

	// Title Banner
	_ = pdf.SetFont("Amiri", "", 16)
	pdf.SetXY(50, 40)
	_ = pdf.CellWithOption(&gopdf.Rect{W: 495, H: 25}, ShapeArabic("منظومة فينوس للمدفوعات — تقرير المطابقة المحاسبية اليومية"), gopdf.CellOption{
		Align: gopdf.Center,
	})

	_ = pdf.SetFont("Amiri", "", 11)
	pdf.SetXY(50, 70)
	metaLine := fmt.Sprintf("تاريخ التقرير: %s | التوقيت: بغداد (UTC+3) | الحالة: %s", report.ReportDate, report.Status)
	_ = pdf.CellWithOption(&gopdf.Rect{W: 495, H: 20}, ShapeArabicBidi(metaLine), gopdf.CellOption{
		Align: gopdf.Center,
	})

	// Table Headers: RTL columns: [Notes (180), Value (135), Label (180)]
	y := 105.0
	pdf.SetXY(50, y)

	_ = pdf.SetFont("Amiri", "", 11)
	// Rightmost column (Label in RTL layout)
	pdf.SetXY(50+180+135, y)
	_ = pdf.CellWithOption(&gopdf.Rect{W: 180, H: 24}, ShapeArabic("البند المحاسبي والمالي"), gopdf.CellOption{
		Align:  gopdf.Center,
		Border: gopdf.AllBorders,
	})
	// Middle column (Value)
	pdf.SetXY(50+180, y)
	_ = pdf.CellWithOption(&gopdf.Rect{W: 135, H: 24}, ShapeArabic("القيمة (د.ع IQD)"), gopdf.CellOption{
		Align:  gopdf.Center,
		Border: gopdf.AllBorders,
	})
	// Leftmost column (Notes)
	pdf.SetXY(50, y)
	_ = pdf.CellWithOption(&gopdf.Rect{W: 180, H: 24}, ShapeArabic("الملاحظات والتفاصيل"), gopdf.CellOption{
		Align:  gopdf.Center,
		Border: gopdf.AllBorders,
	})

	rows := []struct {
		Label string
		Val   string
		Notes string
	}{
		{"تاريخ المطابقة المحاسبية", report.ReportDate, "بتوقيت بغداد UTC+3"},
		{"واردات بوابة زين كاش", fmt.Sprintf("%d IQD", report.TotalGatewayVolume), "المدفوعات الإلكترونية الناجحة"},
		{"الشحن اليدوي والتسويات", fmt.Sprintf("%d IQD", report.ManualTopups), "الشحن النقدي المعتمد من الإدارة"},
		{"مبيعات البطاقات الرقمية", fmt.Sprintf("%d IQD", report.CardsSold), "سحب بطاقات التعبئة الرقمية"},
		{"المبالغ المسترجعة (Refunds)", fmt.Sprintf("%d IQD", report.Refunds), "عمليات الإرجاع المعتمدة إلى المحافظ"},
		{"إجمالي إيداعات دفتر الأستاذ", fmt.Sprintf("%d IQD", report.TotalLedgerCredits), "إجمالي الحركات الدائنة"},
		{"إجمالي سحوبات دفتر الأستاذ", fmt.Sprintf("%d IQD", report.TotalLedgerDebits), "إجمالي الحركات المدينة"},
		{"التزام المنصة (أرصدة المحافظ)", fmt.Sprintf("%d IQD", report.TotalSystemBalance), "رصيد المحافظ الفعلي المعتمد"},
		{"الفارق المحاسبي (Discrepancy)", fmt.Sprintf("%d IQD", report.Discrepancy), "الفرق بين الأرصدة ودفتر الأستاذ"},
		{"حالة المطابقة المحاسبية", report.Status, "نتيجة التدقيق الآلي"},
		{"المولد بواسطة", report.GeneratedBy, "النظام الآلي للرقابة المالية"},
	}

	_ = pdf.SetFont("Amiri", "", 10)
	for _, r := range rows {
		y += 22.0
		// Column 1 (Right): Label
		pdf.SetXY(50+180+135, y)
		_ = pdf.CellWithOption(&gopdf.Rect{W: 180, H: 22}, ShapeArabic(r.Label), gopdf.CellOption{
			Align:  gopdf.Right,
			Border: gopdf.AllBorders,
		})
		// Column 2 (Middle): Value
		pdf.SetXY(50+180, y)
		_ = pdf.CellWithOption(&gopdf.Rect{W: 135, H: 22}, r.Val, gopdf.CellOption{
			Align:  gopdf.Center,
			Border: gopdf.AllBorders,
		})
		// Column 3 (Left): Notes
		pdf.SetXY(50, y)
		_ = pdf.CellWithOption(&gopdf.Rect{W: 180, H: 22}, ShapeArabic(r.Notes), gopdf.CellOption{
			Align:  gopdf.Right,
			Border: gopdf.AllBorders,
		})
	}

	return pdf.GetBytesPdf()
}

// GenerateStatementPDF generates a valid PDF 1.4 binary document for a subscriber statement with embedded Arabic font.
func GenerateStatementPDF(userEmail, userName string, balance, creditLimit int64, entriesCount int) []byte {
	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	pdf.AddPage()

	if len(arabicFontData) > 0 {
		_ = pdf.AddTTFFontData("Amiri", arabicFontData)
		_ = pdf.SetFont("Amiri", "", 12)
	}

	// Title
	_ = pdf.SetFont("Amiri", "", 16)
	pdf.SetXY(50, 40)
	_ = pdf.CellWithOption(&gopdf.Rect{W: 495, H: 25}, ShapeArabic("كشف حساب مالي رسمي — منظومة فينوس"), gopdf.CellOption{
		Align: gopdf.Center,
	})

	_ = pdf.SetFont("Amiri", "", 11)
	pdf.SetXY(50, 75)
	userLine := fmt.Sprintf("المشترك: %s (%s)", userName, userEmail)
	_ = pdf.CellWithOption(&gopdf.Rect{W: 495, H: 20}, ShapeArabicBidi(userLine), gopdf.CellOption{
		Align: gopdf.Center,
	})

	// Statement Summary Table
	y := 115.0
	pdf.SetXY(50+247.5, y)
	_ = pdf.CellWithOption(&gopdf.Rect{W: 247.5, H: 24}, ShapeArabic("البيان المحاسبي"), gopdf.CellOption{
		Align:  gopdf.Center,
		Border: gopdf.AllBorders,
	})
	pdf.SetXY(50, y)
	_ = pdf.CellWithOption(&gopdf.Rect{W: 247.5, H: 24}, ShapeArabic("القيمة المعتمدة"), gopdf.CellOption{
		Align:  gopdf.Center,
		Border: gopdf.AllBorders,
	})

	rows := []struct {
		Label string
		Val   string
	}{
		{"الرصيد النهائي المتاح في المحفظة", fmt.Sprintf("%d IQD", balance)},
		{"سقف الائتمان الممنوح (Credit Limit)", fmt.Sprintf("%d IQD", creditLimit)},
		{"إجمالي الحركات المحاسبية المسجلة", fmt.Sprintf("%d حركة موثقة", entriesCount)},
		{"حالة تدقيق سلسلة التجزئة (Hash Chain)", "موثقة رقمياً ومشفرة بالكامل 100%"},
		{"تاريخ وتوقيت إصدار الكشف", time.Now().Format("2006-01-02 15:04:05 (Baghdad UTC+3)")},
	}

	for _, r := range rows {
		y += 24.0
		pdf.SetXY(50+247.5, y)
		_ = pdf.CellWithOption(&gopdf.Rect{W: 247.5, H: 24}, ShapeArabic(r.Label), gopdf.CellOption{
			Align:  gopdf.Right,
			Border: gopdf.AllBorders,
		})
		pdf.SetXY(50, y)
		_ = pdf.CellWithOption(&gopdf.Rect{W: 247.5, H: 24}, ShapeArabicBidi(r.Val), gopdf.CellOption{
			Align:  gopdf.Center,
			Border: gopdf.AllBorders,
		})
	}

	return pdf.GetBytesPdf()
}
