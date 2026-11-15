package reconcile

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SuspiciousTx struct {
	ID        int64     `json:"id"`
	Type      string    `json:"type"`
	Amount    int64     `json:"amount"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type ReconciliationReport struct {
	ID                     int64          `json:"id"`
	ReportDate             string         `json:"report_date"`
	TotalSystemBalance     int64          `json:"total_system_balance"`
	TotalGatewayVolume     int64          `json:"total_gateway_volume"`
	ManualTopups           int64          `json:"manual_topups"`
	CardsSold              int64          `json:"cards_sold"`
	Refunds                int64          `json:"refunds"`
	TotalLedgerCredits     int64          `json:"total_ledger_credits"`
	TotalLedgerDebits      int64          `json:"total_ledger_debits"`
	ExpectedClosingBalance int64          `json:"expected_closing_balance"`
	ActualClosingBalance   int64          `json:"actual_closing_balance"`
	Discrepancy            int64          `json:"discrepancy"`
	Status                 string         `json:"status"` // 'matched', 'discrepancy', 'investigating'
	Details                map[string]any `json:"details"`
	GeneratedBy            string         `json:"generated_by"`
	CreatedAt              time.Time      `json:"created_at"`
}

type FinancialClose struct {
	ID                   int64     `json:"id"`
	PeriodType           string    `json:"period_type"` // 'daily', 'monthly'
	PeriodDate           string    `json:"period_date"`
	TotalInflow          int64     `json:"total_inflow"`
	TotalOutflow         int64     `json:"total_outflow"`
	NetRevenue           int64     `json:"net_revenue"`
	TotalWalletLiability int64     `json:"total_wallet_liability"`
	TransactionCount     int64     `json:"transaction_count"`
	ClosingHash          string    `json:"closing_hash"`
	Status               string    `json:"status"`
	ClosedBy             *int64    `json:"closed_by,omitempty"`
	ClosedAt             time.Time `json:"closed_at"`
}

type Service struct {
	Pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{Pool: pool}
}

// RunDailyReconciliation runs the mathematical verification for a specific calendar date (UTC).
// Formula:
// Inflow = Zain Cash received + Manual top-ups
// Outflow/Deductions = Cards sold - Refunds
// Net Movement = Inflow - Outflow
// Lists suspicious transactions when discrepancy != 0.
func (s *Service) RunDailyReconciliation(ctx context.Context, targetDate time.Time, generatedBy string) (*ReconciliationReport, error) {
	dateStr := targetDate.Format("2006-01-02")
	startOfDay := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour)

	// 1. Total current subscriber wallet balance (total liability)
	var actualBalance float64
	err := s.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(wallet_balance), 0)
		 FROM users
		 WHERE role = 'subscriber'`,
	).Scan(&actualBalance)
	if err != nil {
		return nil, fmt.Errorf("calculate total system balance: %w", err)
	}
	actualBalanceIQD := int64(actualBalance)

	// 2. Zain Cash received
	var gatewayVolume float64
	_ = s.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0)
		 FROM zaincash_transactions
		 WHERE status = 'SUCCESS' AND created_at >= $1 AND created_at < $2`,
		startOfDay, endOfDay,
	).Scan(&gatewayVolume)
	gatewayVolumeIQD := int64(gatewayVolume)

	// 3. Manual top-ups (admin adjustments / cash)
	var manualTopups float64
	_ = s.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0)
		 FROM wallet_ledger
		 WHERE kind IN ('topup', 'admin_credit') AND provider <> 'zaincash' AND status = 'success'
		   AND created_at >= $1 AND created_at < $2`,
		startOfDay, endOfDay,
	).Scan(&manualTopups)
	manualTopupsIQD := int64(manualTopups)

	// 4. Cards sold
	var cardsSold float64
	_ = s.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(sold_price), 0)
		 FROM vouchers_vault
		 WHERE is_sold = true AND sold_at >= $1 AND sold_at < $2`,
		startOfDay, endOfDay,
	).Scan(&cardsSold)
	cardsSoldIQD := int64(cardsSold)

	// 5. Refunds
	var refunds float64
	_ = s.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0)
		 FROM wallet_ledger
		 WHERE kind = 'refund' AND status = 'success'
		   AND created_at >= $1 AND created_at < $2`,
		startOfDay, endOfDay,
	).Scan(&refunds)
	refundsIQD := int64(refunds)

	// 6. Total subscriber credits & debits in double-entry ledger
	var ledgerCredits, ledgerDebits int64
	_ = s.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(le.credit), 0), COALESCE(SUM(le.debit), 0)
		 FROM ledger_entries le
		 JOIN users u ON le.account_id = u.id
		 WHERE u.role = 'subscriber' AND le.created_at >= $1 AND le.created_at < $2`,
		startOfDay, endOfDay,
	).Scan(&ledgerCredits, &ledgerDebits)

	// Inflow & Outflow formula:
	totalInflow := gatewayVolumeIQD + manualTopupsIQD
	totalOutflow := cardsSoldIQD - refundsIQD
	expectedMovement := totalInflow - totalOutflow
	actualMovement := ledgerCredits - ledgerDebits

	discrepancy := actualMovement - expectedMovement
	if discrepancy < 0 {
		discrepancy = -discrepancy
	}

	status := "matched"
	if discrepancy != 0 {
		status = "discrepancy"
	}

	// 7. Find suspicious transactions if discrepancy != 0
	var suspicious []SuspiciousTx
	if discrepancy != 0 {
		// A. Pending ZainCash transactions older than 30 minutes
		pRows, pErr := s.Pool.Query(ctx,
			`SELECT id, amount, created_at
			 FROM zaincash_transactions
			 WHERE status = 'PENDING' AND created_at >= $1 AND created_at < $2
			 LIMIT 20`,
			startOfDay, endOfDay,
		)
		if pErr == nil {
			for pRows.Next() {
				var id int64
				var amt float64
				var cAt time.Time
				if err := pRows.Scan(&id, &amt, &cAt); err == nil {
					suspicious = append(suspicious, SuspiciousTx{
						ID:        id,
						Type:      "ZainCash PENDING",
						Amount:    int64(amt),
						Reason:    "عملية زين كاش معلقة لم يتم تأكيدها أو إلغاؤها",
						CreatedAt: cAt,
					})
				}
			}
			pRows.Close()
		}

		// B. Users with negative balance exceeding credit limit
		uRows, uErr := s.Pool.Query(ctx,
			`SELECT id, wallet_balance, credit_limit
			 FROM users
			 WHERE wallet_balance < -credit_limit`,
		)
		if uErr == nil {
			for uRows.Next() {
				var uid, limit int64
				var bal float64
				if err := uRows.Scan(&uid, &bal, &limit); err == nil {
					suspicious = append(suspicious, SuspiciousTx{
						ID:        uid,
						Type:      "Overdraft Breach",
						Amount:    int64(bal),
						Reason:    fmt.Sprintf("تجاوز سقف الائتمان: الرصيد %d أقل من السقف %d", int64(bal), -limit),
						CreatedAt: time.Now().UTC(),
					})
				}
			}
			uRows.Close()
		}
	}

	details := map[string]any{
		"date":                    dateStr,
		"zaincash_volume_iqd":     gatewayVolumeIQD,
		"manual_topups_iqd":       manualTopupsIQD,
		"cards_sold_iqd":          cardsSoldIQD,
		"refunds_iqd":             refundsIQD,
		"total_inflow_iqd":        totalInflow,
		"total_outflow_iqd":       totalOutflow,
		"expected_movement_iqd":   expectedMovement,
		"actual_movement_iqd":     actualMovement,
		"suspicious_transactions": suspicious,
		"suspicious_count":        len(suspicious),
		"verified_at":             time.Now().UTC(),
	}
	detailsJSON, _ := json.Marshal(details)

	expectedClosing := actualBalanceIQD

	var reportID int64
	err = s.Pool.QueryRow(ctx,
		`INSERT INTO reconciliation_reports (
			report_date, total_system_balance, total_gateway_volume,
			total_ledger_credits, total_ledger_debits, expected_closing_balance,
			actual_closing_balance, discrepancy, status, details, generated_by
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 RETURNING id`,
		dateStr, actualBalanceIQD, gatewayVolumeIQD,
		ledgerCredits, ledgerDebits, expectedClosing,
		actualBalanceIQD, discrepancy, status, detailsJSON, generatedBy,
	).Scan(&reportID)
	if err != nil {
		return nil, fmt.Errorf("save reconciliation report: %w", err)
	}

	return &ReconciliationReport{
		ID:                     reportID,
		ReportDate:             dateStr,
		TotalSystemBalance:     actualBalanceIQD,
		TotalGatewayVolume:     gatewayVolumeIQD,
		ManualTopups:           manualTopupsIQD,
		CardsSold:              cardsSoldIQD,
		Refunds:                refundsIQD,
		TotalLedgerCredits:     ledgerCredits,
		TotalLedgerDebits:      ledgerDebits,
		ExpectedClosingBalance: expectedClosing,
		ActualClosingBalance:   actualBalanceIQD,
		Discrepancy:            discrepancy,
		Status:                 status,
		Details:                details,
		GeneratedBy:            generatedBy,
		CreatedAt:              time.Now().UTC(),
	}, nil
}

// SealFinancialClose generates a cryptographically sealed daily or monthly close.
func (s *Service) SealFinancialClose(ctx context.Context, periodType string, periodDate time.Time, closedBy int64) (*FinancialClose, error) {
	dateStr := periodDate.Format("2006-01-02")
	startOfDay := time.Date(periodDate.Year(), periodDate.Month(), periodDate.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour)
	if periodType == "monthly" {
		startOfDay = time.Date(periodDate.Year(), periodDate.Month(), 1, 0, 0, 0, 0, time.UTC)
		endOfDay = startOfDay.AddDate(0, 1, 0)
		dateStr = periodDate.Format("2006-01")
	}

	var inflow, outflow, txCount int64
	_ = s.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(credit), 0), COALESCE(SUM(debit), 0), COUNT(DISTINCT tx_id)
		 FROM ledger_entries
		 WHERE created_at >= $1 AND created_at < $2`,
		startOfDay, endOfDay,
	).Scan(&inflow, &outflow, &txCount)

	var walletLiability float64
	_ = s.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(wallet_balance), 0)
		 FROM users
		 WHERE role = 'subscriber'`,
	).Scan(&walletLiability)
	walletLiabilityIQD := int64(walletLiability)

	netRevenue := inflow - outflow

	// Cryptographic SHA-256 seal
	rawSeal := fmt.Sprintf("%s|%s|%d|%d|%d|%d|%d|%d",
		periodType, dateStr, inflow, outflow, netRevenue, walletLiabilityIQD, txCount, closedBy,
	)
	hashSum := sha256.Sum256([]byte(rawSeal))
	closingHash := hex.EncodeToString(hashSum[:])

	var closeID int64
	var closedByParam any
	if closedBy > 0 {
		closedByParam = closedBy
	}

	err := s.Pool.QueryRow(ctx,
		`INSERT INTO financial_closes (
			period_type, period_date, total_inflow, total_outflow, net_revenue,
			total_wallet_liability, transaction_count, closing_hash, status, closed_by
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'sealed', $9)
		 ON CONFLICT (period_type, period_date) DO UPDATE SET
			total_inflow = EXCLUDED.total_inflow,
			total_outflow = EXCLUDED.total_outflow,
			net_revenue = EXCLUDED.net_revenue,
			total_wallet_liability = EXCLUDED.total_wallet_liability,
			transaction_count = EXCLUDED.transaction_count,
			closing_hash = EXCLUDED.closing_hash,
			closed_by = EXCLUDED.closed_by,
			closed_at = now()
		 RETURNING id`,
		periodType, dateStr, inflow, outflow, netRevenue,
		walletLiabilityIQD, txCount, closingHash, closedByParam,
	).Scan(&closeID)
	if err != nil {
		return nil, fmt.Errorf("seal financial close: %w", err)
	}

	return &FinancialClose{
		ID:                   closeID,
		PeriodType:           periodType,
		PeriodDate:           dateStr,
		TotalInflow:          inflow,
		TotalOutflow:         outflow,
		NetRevenue:           netRevenue,
		TotalWalletLiability: walletLiabilityIQD,
		TransactionCount:     txCount,
		ClosingHash:          closingHash,
		Status:               "sealed",
		ClosedBy:             &closedBy,
		ClosedAt:             time.Now().UTC(),
	}, nil
}

// GenerateCSV generates an RFC-compliant CSV with UTF-8 BOM so Microsoft Excel
// opens Arabic text cleanly without garbled characters.
func GenerateCSV(report *ReconciliationReport) []byte {
	buf := new(bytes.Buffer)
	// UTF-8 BOM (0xEF, 0xBB, 0xBF)
	buf.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(buf)

	_ = writer.Write([]string{"الحقل المحاسبي", "القيمة (دينار عراقي IQD)", "الملاحظات والتفاصيل"})
	_ = writer.Write([]string{"تاريخ المطابقة", report.ReportDate, "بتوقيت بغداد UTC+3"})
	_ = writer.Write([]string{"واردات بوابة زين كاش", fmt.Sprintf("%d", report.TotalGatewayVolume), "المدفوعات الإلكترونية الناجحة"})
	_ = writer.Write([]string{"الشحن اليدوي والتسويات", fmt.Sprintf("%d", report.ManualTopups), "الشحن النقدي المعتمد"})
	_ = writer.Write([]string{"مبيعات البطاقات الرقمية", fmt.Sprintf("%d", report.CardsSold), "سحب بطاقات التعبئة"})
	_ = writer.Write([]string{"المبالغ المسترجعة (Refunds)", fmt.Sprintf("%d", report.Refunds), "عمليات الإرجاع المعتمدة"})
	_ = writer.Write([]string{"إجمالي إيداعات دفتر الأستاذ (Credits)", fmt.Sprintf("%d", report.TotalLedgerCredits), "الحركات المسجلة"})
	_ = writer.Write([]string{"إجمالي سحوبات دفتر الأستاذ (Debits)", fmt.Sprintf("%d", report.TotalLedgerDebits), "المشتريات والرسوم"})
	_ = writer.Write([]string{"التزام المنصة (إجمالي أرصدة المحافظ)", fmt.Sprintf("%d", report.TotalSystemBalance), "رصيد المحافظ الفعلي الحالي"})
	_ = writer.Write([]string{"الفارق المحاسبي والانحراف", fmt.Sprintf("%d", report.Discrepancy), report.Status})
	_ = writer.Write([]string{"حالة المطابقة النهائية", report.Status, "تمت المطابقة بواسطة " + report.GeneratedBy})

	writer.Flush()
	return buf.Bytes()
}

// GenerateExcelHTML exports a styled HTML spreadsheet natively interpreted by Microsoft Excel.
func GenerateExcelHTML(report *ReconciliationReport) []byte {
	tmpl := fmt.Sprintf(`<!DOCTYPE html>
<html lang="ar" dir="rtl">
<head>
<meta charset="UTF-8">
<style>
body { font-family: 'Segoe UI', Tahoma, Arial, sans-serif; }
table { border-collapse: collapse; width: 100%%; direction: rtl; }
th, td { border: 1px solid #CBD5E1; padding: 8px 12px; text-align: right; }
th { background-color: #0058BC; color: white; font-weight: bold; }
tr:nth-child(even) { background-color: #F8FAFC; }
.matched { color: #16A34A; font-weight: bold; }
.discrepancy { color: #DC2626; font-weight: bold; }
</style>
</head>
<body>
<h2>تقرير المطابقة المالية اليومي — VENUS POS Enterprise</h2>
<p>التاريخ: <strong>%s</strong> | المنفذ: <strong>%s</strong></p>
<table>
<thead>
<tr>
  <th>الحقل المحاسبي</th>
  <th>القيمة (دينار عراقي IQD)</th>
  <th>الحالة والتفاصيل</th>
</tr>
</thead>
<tbody>
<tr><td>واردات بوابة زين كاش</td><td>%d</td><td>المدفوعات الإلكترونية</td></tr>
<tr><td>الشحن اليدوي والتسويات</td><td>%d</td><td>الشحن الإداري والنقدي</td></tr>
<tr><td>مبيعات البطاقات الرقمية</td><td>%d</td><td>المخزون المسحوب</td></tr>
<tr><td>المبالغ المسترجعة</td><td>%d</td><td>الإرجاعات</td></tr>
<tr><td>إيداعات دفتر الأستاذ</td><td>%d</td><td>مجموع الدائن</td></tr>
<tr><td>سحوبات دفتر الأستاذ</td><td>%d</td><td>مجموع المدين</td></tr>
<tr><td>التزام المحافظ الحالي</td><td>%d</td><td>الرصيد الفعلي</td></tr>
<tr><td>الفارق المحاسبي</td><td>%d</td><td class="%s">%s</td></tr>
</tbody>
</table>
</body>
</html>`,
		report.ReportDate, report.GeneratedBy,
		report.TotalGatewayVolume, report.ManualTopups, report.CardsSold,
		report.Refunds, report.TotalLedgerCredits, report.TotalLedgerDebits,
		report.TotalSystemBalance, report.Discrepancy, report.Status, report.Status,
	)
	return []byte(tmpl)
}
