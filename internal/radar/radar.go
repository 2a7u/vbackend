package radar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type CategoryStockStatus struct {
	CategoryID     string  `json:"category_id"`
	CategoryTitle  string  `json:"category_title"`
	AvailableStock int     `json:"available_stock"`
	MinThreshold   int     `json:"min_threshold"`
	IsLowStock     bool    `json:"is_low_stock"`
	BurnRate1h     float64 `json:"burn_rate_1h"`  // items/hr
	BurnRate6h     float64 `json:"burn_rate_6h"`  // items/hr
	BurnRate24h    float64 `json:"burn_rate_24h"` // items/hr
	EstimatedHours float64 `json:"estimated_hours"`
	EstimatedText  string  `json:"estimated_text"`
}

type RadarSummary struct {
	TotalCategories int                   `json:"total_categories"`
	LowStockCount   int                   `json:"low_stock_count"`
	HasLowStock     bool                  `json:"has_low_stock"`
	AlertMessage    string                `json:"alert_message,omitempty"`
	Categories      []CategoryStockStatus `json:"categories"`
	CheckedAt       time.Time             `json:"checked_at"`
}

type Service struct {
	Pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{Pool: pool}
}

// ComputeExhaustionEstimate safely calculates burn rate and exhaustion text without division by zero.
func ComputeExhaustionEstimate(availableStock int, sales1h, sales6h, sales24h int) (float64, float64, float64, float64, string) {
	rate1h := float64(sales1h)
	rate6h := float64(sales6h) / 6.0
	rate24h := float64(sales24h) / 24.0

	// Weighted effective burn rate: 50% last hour, 30% last 6 hours, 20% last 24 hours
	effectiveRate := (rate1h * 0.5) + (rate6h * 0.3) + (rate24h * 0.2)
	if effectiveRate <= 0.001 {
		if rate1h > 0 {
			effectiveRate = rate1h
		} else if rate6h > 0 {
			effectiveRate = rate6h
		} else if rate24h > 0 {
			effectiveRate = rate24h
		}
	}

	if effectiveRate <= 0.001 {
		return rate1h, rate6h, rate24h, -1, "المخزون مستقر / لا توجد مبيعات حديثة"
	}

	if availableStock <= 0 {
		return rate1h, rate6h, rate24h, 0, "نفد المخزون بالكامل!"
	}

	totalHours := float64(availableStock) / effectiveRate
	h := int(math.Floor(totalHours))
	m := int(math.Round((totalHours - float64(h)) * 60))
	if m == 60 {
		h++
		m = 0
	}

	var text string
	if h > 48 {
		days := h / 24
		text = fmt.Sprintf("يكفي لأكثر من %d أيام", days)
	} else if h > 0 {
		text = fmt.Sprintf("ينفد خلال %d ساعة و %d دقيقة", h, m)
	} else {
		text = fmt.Sprintf("ينفد خلال %d دقيقة", m)
	}

	return rate1h, rate6h, rate24h, totalHours, text
}

// GetRadarSummary evaluates current inventory, category thresholds, burn rates, and alerts.
func (s *Service) GetRadarSummary(ctx context.Context) (*RadarSummary, error) {
	summary := &RadarSummary{
		CheckedAt:  time.Now().UTC(),
		Categories: []CategoryStockStatus{},
	}

	if s.Pool == nil {
		return summary, nil
	}

	// 1. Fetch available stock grouped by brand/category
	stockRows, err := s.Pool.Query(ctx,
		`SELECT brand_id, COUNT(*) 
		 FROM vouchers_vault 
		 WHERE is_sold = FALSE 
		 GROUP BY brand_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("query available stock: %w", err)
	}
	defer stockRows.Close()

	stockMap := make(map[string]int)
	for stockRows.Next() {
		var brand string
		var count int
		if err := stockRows.Scan(&brand, &count); err == nil {
			stockMap[brand] = count
		}
	}

	// 2. Fetch thresholds
	threshRows, err := s.Pool.Query(ctx,
		`SELECT category_id, min_threshold FROM category_thresholds`,
	)
	threshMap := make(map[string]int)
	if err == nil {
		defer threshRows.Close()
		for threshRows.Next() {
			var cat string
			var th int
			if err := threshRows.Scan(&cat, &th); err == nil {
				threshMap[cat] = th
			}
		}
	}

	// 3. Fetch sales in last 1h, 6h, 24h
	now := time.Now().UTC()
	sales1hMap := s.getSalesCountSince(ctx, now.Add(-1*time.Hour))
	sales6hMap := s.getSalesCountSince(ctx, now.Add(-6*time.Hour))
	sales24hMap := s.getSalesCountSince(ctx, now.Add(-24*time.Hour))

	// 4. Distinct categories from catalog & vault
	catRows, err := s.Pool.Query(ctx,
		`SELECT DISTINCT brand_id FROM vouchers_vault WHERE brand_id <> ''
		 UNION
		 SELECT DISTINCT title FROM card_categories WHERE active = TRUE`,
	)
	if err != nil {
		return nil, err
	}
	defer catRows.Close()

	var lowStockCategories []string

	for catRows.Next() {
		var cat string
		if err := catRows.Scan(&cat); err == nil {
			avail := stockMap[cat]
			minTh := 5
			if customTh, ok := threshMap[cat]; ok && customTh > 0 {
				minTh = customTh
			}

			s1 := sales1hMap[cat]
			s6 := sales6hMap[cat]
			s24 := sales24hMap[cat]

			r1, r6, r24, estH, estText := ComputeExhaustionEstimate(avail, s1, s6, s24)
			isLow := avail < minTh

			if isLow {
				lowStockCategories = append(lowStockCategories, fmt.Sprintf("%s (%d متبقي)", cat, avail))
			}

			summary.Categories = append(summary.Categories, CategoryStockStatus{
				CategoryID:     cat,
				CategoryTitle:  cat,
				AvailableStock: avail,
				MinThreshold:   minTh,
				IsLowStock:     isLow,
				BurnRate1h:     r1,
				BurnRate6h:     r6,
				BurnRate24h:    r24,
				EstimatedHours: estH,
				EstimatedText:  estText,
			})
		}
	}

	summary.TotalCategories = len(summary.Categories)
	summary.LowStockCount = len(lowStockCategories)
	summary.HasLowStock = summary.LowStockCount > 0

	if summary.HasLowStock {
		summary.AlertMessage = fmt.Sprintf("تنبيه مخزون حرج: %d فئات وصلت للحد الأدنى [%s]",
			summary.LowStockCount, strings.Join(lowStockCategories, "، "))
		// Dispatch webhook alert asynchronously
		go s.dispatchLowStockWebhook(summary.AlertMessage)
	}

	return summary, nil
}

func (s *Service) getSalesCountSince(ctx context.Context, since time.Time) map[string]int {
	m := make(map[string]int)
	rows, err := s.Pool.Query(ctx,
		`SELECT brand_id, COUNT(*) 
		 FROM vouchers_vault 
		 WHERE is_sold = TRUE AND sold_at >= $1 
		 GROUP BY brand_id`,
		since,
	)
	if err != nil {
		return m
	}
	defer rows.Close()

	for rows.Next() {
		var brand string
		var count int
		if err := rows.Scan(&brand, &count); err == nil {
			m[brand] = count
		}
	}
	return m
}

func (s *Service) dispatchLowStockWebhook(message string) {
	webhookURL := strings.TrimSpace(os.Getenv("INVENTORY_WEBHOOK_URL"))
	if webhookURL == "" {
		webhookURL = strings.TrimSpace(os.Getenv("FRAUD_WEBHOOK_URL"))
	}
	if webhookURL == "" {
		return
	}

	payload := map[string]any{
		"event":     "low_stock_alert",
		"message":   message,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", webhookURL, bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("inventory radar webhook error: %v", err)
		return
	}
	defer resp.Body.Close()
}
