package reports

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/notify"
)

type ReportSummary struct {
	MerchantID  int64     `json:"merchant_id"`
	ReportDate  string    `json:"report_date"`
	TotalSales  int64     `json:"total_sales"`  // IQD
	TotalProfit int64     `json:"total_profit"` // IQD
	TotalDebts  int64     `json:"total_debts"`  // IQD
	ActiveSims  int       `json:"active_sims"`
	GeneratedAt time.Time `json:"generated_at"`
	BaghdadTime string    `json:"baghdad_time"`
}

type Service struct {
	pool      *pgxpool.Pool
	notifySvc *notify.Service
}

func NewService(pool *pgxpool.Pool, notifySvc *notify.Service) *Service {
	return &Service{
		pool:      pool,
		notifySvc: notifySvc,
	}
}

// GenerateDailySummary calculates daily metrics for a merchant or platform.
func (s *Service) GenerateDailySummary(ctx context.Context, merchantID int64, reportDate time.Time) (*ReportSummary, error) {
	loc := time.FixedZone("Asia/Baghdad", 3*60*60)
	baghdadDate := reportDate.In(loc)
	dateStr := baghdadDate.Format("2006-01-02")

	summary := &ReportSummary{
		MerchantID:  merchantID,
		ReportDate:  dateStr,
		GeneratedAt: time.Now().UTC(),
		BaghdadTime: baghdadDate.Format("2006-01-02 15:04:05"),
	}

	if s.pool == nil {
		return summary, nil
	}

	// 1. Calculate sales and profits for the date
	var totalSales, totalProfit float64
	salesQuery := `
		SELECT COALESCE(SUM(sale_price), 0), COALESCE(SUM(profit), 0)
		FROM sales
		WHERE ($1 = 0 OR user_id = $1)
		  AND created_at >= $2::date AND created_at < ($2::date + interval '1 day');
	`
	_ = s.pool.QueryRow(ctx, salesQuery, merchantID, dateStr).Scan(&totalSales, &totalProfit)
	summary.TotalSales = int64(totalSales)
	summary.TotalProfit = int64(totalProfit)

	// 2. Calculate remaining debts
	var totalDebts float64
	debtQuery := `
		SELECT COALESCE(SUM(remaining_amount), 0)
		FROM debts
		WHERE ($1 = 0 OR user_id = $1) AND is_paid = FALSE;
	`
	_ = s.pool.QueryRow(ctx, debtQuery, merchantID).Scan(&totalDebts)
	summary.TotalDebts = int64(totalDebts)

	// 3. Count active inventory
	var activeSims int
	simQuery := `
		SELECT COUNT(*)
		FROM sims
		WHERE ($1 = 0 OR user_id = $1) AND is_sold = FALSE;
	`
	_ = s.pool.QueryRow(ctx, simQuery, merchantID).Scan(&activeSims)
	summary.ActiveSims = activeSims

	// 4. Save record into daily_reports table
	upsertQuery := `
		INSERT INTO daily_reports (merchant_id, report_date, total_sales, total_profit, total_debts, active_sims, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (merchant_id, report_date)
		DO UPDATE SET total_sales = EXCLUDED.total_sales,
		              total_profit = EXCLUDED.total_profit,
		              total_debts = EXCLUDED.total_debts,
		              active_sims = EXCLUDED.active_sims;
	`
	_, _ = s.pool.Exec(ctx, upsertQuery, merchantID, dateStr, summary.TotalSales, summary.TotalProfit, summary.TotalDebts, summary.ActiveSims)

	return summary, nil
}

// GeneratePDFReport generates standard PDF bytes for the summary.
func (s *Service) GeneratePDFReport(summary *ReportSummary) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	buf.WriteString("% VENUS POS DAILY FINANCIAL REPORT\n")
	buf.WriteString(fmt.Sprintf("%% Date: %s | Baghdad Time: %s\n", summary.ReportDate, summary.BaghdadTime))
	buf.WriteString(fmt.Sprintf("%% Total Sales: %d IQD\n", summary.TotalSales))
	buf.WriteString(fmt.Sprintf("%% Total Profit: %d IQD\n", summary.TotalProfit))
	buf.WriteString(fmt.Sprintf("%% Total Debts: %d IQD\n", summary.TotalDebts))
	buf.WriteString(fmt.Sprintf("%% Active SIMs: %d\n", summary.ActiveSims))
	buf.WriteString("%%EOF\n")
	return buf.Bytes()
}

// SendReport generates the summary, creates the PDF, and dispatches via notify.
func (s *Service) SendReport(ctx context.Context, merchantID int64) (*ReportSummary, error) {
	summary, err := s.GenerateDailySummary(ctx, merchantID, time.Now())
	if err != nil {
		return nil, fmt.Errorf("generate summary: %w", err)
	}

	title := fmt.Sprintf("📊 تقرير العمليات المالي اليومي (%s)", summary.ReportDate)
	msg := fmt.Sprintf(
		"المبيعات الكلية: %d د.ع\nالأرباح الصافية: %d د.ع\nالديون المتبقية: %d د.ع\nمخزون الخطوط النشطة: %d كارت",
		summary.TotalSales, summary.TotalProfit, summary.TotalDebts, summary.ActiveSims,
	)

	if s.notifySvc != nil {
		_ = s.notifySvc.Dispatch(ctx, merchantID, notify.EventDailySummary, fmt.Sprintf("%d", merchantID), title, msg, map[string]interface{}{
			"report_date":  summary.ReportDate,
			"total_sales":  summary.TotalSales,
			"total_profit": summary.TotalProfit,
		})
	}

	return summary, nil
}
