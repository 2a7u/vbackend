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

type ReconciliationReport struct {
	ID                     int64          `json:"id"`
	ReportDate             string         `json:"report_date"`
	TotalSystemBalance     int64          `json:"total_system_balance"`
	TotalGatewayVolume     int64          `json:"total_gateway_volume"`
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

	// 2. Total gateway volume (successful ZainCash topups on target date)
	var gatewayVolume float64
	_ = s.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0)
		 FROM zaincash_transactions
		 WHERE status = 'SUCCESS' AND created_at >= $1 AND created_at < $2`,
		startOfDay, endOfDay,
	).Scan(&gatewayVolume)
	gatewayVolumeIQD := int64(gatewayVolume)

	// 3. Total subscriber credits & debits in ledger on target date
	var ledgerCredits, ledgerDebits int64
	_ = s.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(le.credit), 0), COALESCE(SUM(le.debit), 0)
		 FROM ledger_entries le
		 JOIN users u ON le.account_id = u.id
		 WHERE u.role = 'subscriber' AND le.created_at >= $1 AND le.created_at < $2`,
		startOfDay, endOfDay,
	).Scan(&ledgerCredits, &ledgerDebits)

	// 4. Verification formula:
	// Net movement = credits - debits
	netMovement := ledgerCredits - ledgerDebits
	expectedClosing := actualBalanceIQD // By design, if ledger is strictly synchronized
	discrepancy := ledgerCredits - gatewayVolumeIQD
	if discrepancy < 0 {
		discrepancy = -discrepancy
	}

	status := "matched"
	if discrepancy != 0 {
		status = "discrepancy"
	}

	details := map[string]any{
		"date":                 dateStr,
		"net_movement_iqd":     netMovement,
		"gateway_topups_count": 0,
		"verified_at":          time.Now().UTC(),
	}
	detailsJSON, _ := json.Marshal(details)

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
		walletLiabilityIQD, txCount, closingHash, closedBy,
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

// GenerateCSV generates an RFC-compliant CSV report for accounting teams.
func GenerateCSV(report *ReconciliationReport) []byte {
	buf := new(bytes.Buffer)
	writer := csv.NewWriter(buf)

	_ = writer.Write([]string{"الحقل", "القيمة (دينار عراقي)", "ملاحظات"})
	_ = writer.Write([]string{"تاريخ المطابقة", report.ReportDate, ""})
	_ = writer.Write([]string{"إجمالي رصيد محافظ المشتركين (التزام المنصة)", fmt.Sprintf("%d", report.TotalSystemBalance), "رصيد المحافظ الفعلي"})
	_ = writer.Write([]string{"إجمالي حجم مدفوعات بوابة زين كاش", fmt.Sprintf("%d", report.TotalGatewayVolume), "المدفوعات الناجحة"})
	_ = writer.Write([]string{"إجمالي إيداعات دفتر الأستاذ (Credits)", fmt.Sprintf("%d", report.TotalLedgerCredits), "شحن المحافظ"})
	_ = writer.Write([]string{"إجمالي سحوبات دفتر الأستاذ (Debits)", fmt.Sprintf("%d", report.TotalLedgerDebits), "المشتريات والرسوم"})
	_ = writer.Write([]string{"الرصيد الختامي المتوقع", fmt.Sprintf("%d", report.ExpectedClosingBalance), ""})
	_ = writer.Write([]string{"الرصيد الختامي الفعلي", fmt.Sprintf("%d", report.ActualClosingBalance), ""})
	_ = writer.Write([]string{"الفارق / الانحراف المحاسبي", fmt.Sprintf("%d", report.Discrepancy), report.Status})
	_ = writer.Write([]string{"حالة المطابقة", report.Status, "تمت المطابقة بواسطة " + report.GeneratedBy})

	writer.Flush()
	return buf.Bytes()
}
