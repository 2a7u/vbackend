package forecast

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DailyPoint struct {
	Date       string `json:"date"`
	DayName    string `json:"day_name"`
	SalesIQD   int64  `json:"sales_iqd"`
	ProfitIQD  int64  `json:"profit_iqd"`
	IsForecast bool   `json:"is_forecast"`
}

type LiquidityForecast struct {
	MerchantID       int64        `json:"merchant_id"`
	GeneratedAt      time.Time    `json:"generated_at"`
	CurrentBalance   int64        `json:"current_balance"`
	SevenDayAvgSales int64        `json:"seven_day_avg_sales"`
	ProjectedWeekly  int64        `json:"projected_weekly_sales"`
	BurnRatePerDay   int64        `json:"burn_rate_per_day"`
	RunwayDays       int          `json:"runway_days"`
	History          []DailyPoint `json:"history"`
	ForecastNextWeek []DailyPoint `json:"forecast_next_week"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// ComputeMovingAverage returns simple moving average for data series.
func ComputeMovingAverage(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	var sum int64
	for _, v := range values {
		sum += v
	}
	return sum / int64(len(values))
}

// GetForecast computes 7-day moving average and projects liquidity for next week.
func (s *Service) GetForecast(ctx context.Context, merchantID int64) (*LiquidityForecast, error) {
	loc := time.FixedZone("Asia/Baghdad", 3*60*60)
	now := time.Now().In(loc)

	res := &LiquidityForecast{
		MerchantID:       merchantID,
		GeneratedAt:      now,
		History:          make([]DailyPoint, 0),
		ForecastNextWeek: make([]DailyPoint, 0),
	}

	var balance float64
	if s.pool != nil {
		_ = s.pool.QueryRow(ctx, "SELECT COALESCE(wallet_balance, 0) FROM users WHERE id = $1;", merchantID).Scan(&balance)
	}
	res.CurrentBalance = int64(balance)

	// Fetch past 7 days sales
	var salesHistory []int64
	for i := 6; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		dateStr := day.Format("2006-01-02")
		dayName := day.Format("Monday")

		var daySales, dayProfit float64
		if s.pool != nil {
			query := `
				SELECT COALESCE(SUM(sale_price), 0), COALESCE(SUM(profit), 0)
				FROM sales
				WHERE user_id = $1 AND created_at >= $2::date AND created_at < ($2::date + interval '1 day');
			`
			_ = s.pool.QueryRow(ctx, query, merchantID, dateStr).Scan(&daySales, &dayProfit)
		}

		salesIQD := int64(daySales)
		profitIQD := int64(dayProfit)
		salesHistory = append(salesHistory, salesIQD)

		res.History = append(res.History, DailyPoint{
			Date:       dateStr,
			DayName:    dayName,
			SalesIQD:   salesIQD,
			ProfitIQD:  profitIQD,
			IsForecast: false,
		})
	}

	avgDaily := ComputeMovingAverage(salesHistory)
	if avgDaily == 0 && res.CurrentBalance > 0 {
		avgDaily = res.CurrentBalance / 10
	}
	res.SevenDayAvgSales = avgDaily
	res.ProjectedWeekly = avgDaily * 7

	// Estimate burn rate per day (cost of cards sold)
	res.BurnRatePerDay = (avgDaily * 85) / 100 // ~85% inventory cost
	if res.BurnRatePerDay > 0 {
		res.RunwayDays = int(res.CurrentBalance / res.BurnRatePerDay)
	} else {
		res.RunwayDays = 30
	}

	// Project next 7 days
	for i := 1; i <= 7; i++ {
		nextDay := now.AddDate(0, 0, i)
		// Small weekend variation simulation based on day of week
		weekday := nextDay.Weekday()
		multiplier := 1.0
		if weekday == time.Thursday || weekday == time.Friday {
			multiplier = 1.15 // +15% on weekends
		}

		projectedSales := int64(float64(avgDaily) * multiplier)
		projectedProfit := (projectedSales * 15) / 100 // ~15% margin

		res.ForecastNextWeek = append(res.ForecastNextWeek, DailyPoint{
			Date:       nextDay.Format("2006-01-02"),
			DayName:    nextDay.Format("Monday"),
			SalesIQD:   projectedSales,
			ProfitIQD:  projectedProfit,
			IsForecast: true,
		})
	}

	return res, nil
}
