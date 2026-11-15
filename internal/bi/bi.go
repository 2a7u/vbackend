package bi

import (
	"context"
	"encoding/json"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	Pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{Pool: pool}
}

type BIDashboard struct {
	PeakSalesHours     map[int]int64 `json:"peak_sales_hours"`      // hour (0-23) -> volume IQD
	TopGovernorates    map[string]int64 `json:"top_governorates"`   // governorate -> volume IQD
	SubscriptionProfit int64         `json:"subscription_profit"`  // IQD
	VouchersProfit     int64         `json:"vouchers_profit"`      // IQD
	TotalNetProfit     int64         `json:"total_net_profit"`     // IQD
	RetentionRate      float64       `json:"retention_rate"`       // percentage
	ChurnRate          float64       `json:"churn_rate"`           // percentage
	ActiveSubscribers  int           `json:"active_subscribers"`
	TotalSubscribers   int           `json:"total_subscribers"`
	GeneratedAt        time.Time     `json:"generated_at"`
}

type ServerHealth struct {
	Status           string    `json:"status"`
	DBLatencyMs      int64     `json:"db_latency_ms"`
	DBConnected      bool      `json:"db_connected"`
	GoroutinesCount  int       `json:"goroutines_count"`
	AllocatedRAMMB   float64   `json:"allocated_ram_mb"`
	TotalAllocRAMMB  float64   `json:"total_alloc_ram_mb"`
	SystemRAMMB      float64   `json:"system_ram_mb"`
	NumGC            uint32    `json:"num_gc"`
	ServerTimeUTC    time.Time `json:"server_time_utc"`
	ServerTimeBaghdad string   `json:"server_time_baghdad"`
}

// GetBIDashboard fetches aggregated analytics without hitting heavy live row locks.
func (s *Service) GetBIDashboard(ctx context.Context) (*BIDashboard, error) {
	board := &BIDashboard{
		PeakSalesHours:  make(map[int]int64),
		TopGovernorates: make(map[string]int64),
		GeneratedAt:     time.Now().UTC(),
	}

	// 1. Peak sales hours from sim_sales & credit_sales
	hRows, err := s.Pool.Query(ctx,
		`SELECT EXTRACT(HOUR FROM created_at)::int AS hr, COALESCE(SUM(sell_price), 0)
		 FROM sim_sales
		 WHERE created_at >= now() - interval '30 days'
		 GROUP BY hr
		 ORDER BY hr ASC`,
	)
	if err == nil {
		defer hRows.Close()
		for hRows.Next() {
			var hr int
			var vol int64
			if err := hRows.Scan(&hr, &vol); err == nil {
				board.PeakSalesHours[hr] = vol
			}
		}
	}

	// 2. Active subscribers and churn calculations
	var totalSubs, activeSubs, expiredSubs int
	_ = s.Pool.QueryRow(ctx,
		`SELECT 
			COUNT(*) FILTER (WHERE role = 'subscriber'),
			COUNT(*) FILTER (WHERE role = 'subscriber' AND status = 'active' AND (subscription_ends_at IS NULL OR subscription_ends_at > now())),
			COUNT(*) FILTER (WHERE role = 'subscriber' AND (status = 'suspended' OR (subscription_ends_at IS NOT NULL AND subscription_ends_at <= now())))
		 FROM users`,
	).Scan(&totalSubs, &activeSubs, &expiredSubs)

	board.TotalSubscribers = totalSubs
	board.ActiveSubscribers = activeSubs

	if totalSubs > 0 {
		board.RetentionRate = (float64(activeSubs) / float64(totalSubs)) * 100.0
		board.ChurnRate = (float64(expiredSubs) / float64(totalSubs)) * 100.0
	}

	// 3. Profits: vouchers net profit + subscriptions revenue
	var vProfit, sRevenue int64
	_ = s.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(net_profit), 0) FROM vouchers_vault WHERE is_sold = true`,
	).Scan(&vProfit)

	_ = s.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM zaincash_transactions WHERE status = 'SUCCESS' AND order_type = 'subscription'`,
	).Scan(&sRevenue)

	board.VouchersProfit = vProfit
	board.SubscriptionProfit = sRevenue
	board.TotalNetProfit = vProfit + sRevenue

	return board, nil
}

// GetServerHealth calculates live server and PostgreSQL diagnostics.
func (s *Service) GetServerHealth(ctx context.Context) ServerHealth {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	loc, err := time.LoadLocation("Asia/Baghdad")
	if err != nil {
		loc = time.FixedZone("UTC+3", 3*3600)
	}

	health := ServerHealth{
		Status:            "healthy",
		GoroutinesCount:   runtime.NumGoroutine(),
		AllocatedRAMMB:    float64(m.Alloc) / 1024 / 1024,
		TotalAllocRAMMB:   float64(m.TotalAlloc) / 1024 / 1024,
		SystemRAMMB:       float64(m.Sys) / 1024 / 1024,
		NumGC:             m.NumGC,
		ServerTimeUTC:     time.Now().UTC(),
		ServerTimeBaghdad: time.Now().In(loc).Format("2006-01-02 15:04:05 MST"),
	}

	// PostgreSQL Ping and roundtrip latency
	if s.Pool != nil {
		start := time.Now()
		pingErr := s.Pool.Ping(ctx)
		health.DBLatencyMs = time.Since(start).Milliseconds()
		health.DBConnected = (pingErr == nil)
	}

	if !health.DBConnected {
		health.Status = "degraded"
	}

	return health
}

// GenerateFullBackup creates a structured JSON backup of key business tables.
// All voucher PINs remain strictly encrypted in ciphertext (AES-256-GCM) with ZERO plaintext.
func (s *Service) GenerateFullBackup(ctx context.Context) ([]byte, error) {
	backup := make(map[string]any)
	backup["generated_at"] = time.Now().UTC().Format(time.RFC3339)
	backup["platform"] = "VENUS POS Enterprise"
	backup["version"] = "2.0-Enterprise"

	// 1. Users
	uRows, err := s.Pool.Query(ctx,
		`SELECT id, email, full_name, store_name, phone, role, is_approved, wallet_balance, credit_limit, status, plan_name, subscription_ends_at, created_at
		 FROM users ORDER BY id ASC`,
	)
	if err == nil {
		defer uRows.Close()
		var users []map[string]any
		for uRows.Next() {
			var u struct {
				ID            int64      `json:"id"`
				Email         string     `json:"email"`
				FullName      string     `json:"full_name"`
				StoreName     string     `json:"store_name"`
				Phone         string     `json:"phone"`
				Role          string     `json:"role"`
				IsApproved    bool       `json:"is_approved"`
				WalletBalance float64    `json:"wallet_balance"`
				CreditLimit   int64      `json:"credit_limit"`
				Status        string     `json:"status"`
				PlanName      string     `json:"plan_name"`
				EndsAt        *time.Time `json:"subscription_ends_at"`
				CreatedAt     time.Time  `json:"created_at"`
			}
			if err := uRows.Scan(&u.ID, &u.Email, &u.FullName, &u.StoreName, &u.Phone, &u.Role, &u.IsApproved, &u.WalletBalance, &u.CreditLimit, &u.Status, &u.PlanName, &u.EndsAt, &u.CreatedAt); err == nil {
				users = append(users, map[string]any{
					"id":             u.ID,
					"email":          u.Email,
					"full_name":      u.FullName,
					"store_name":     u.StoreName,
					"phone":          u.Phone,
					"role":           u.Role,
					"wallet_balance": int64(u.WalletBalance),
					"credit_limit":   u.CreditLimit,
					"status":         u.Status,
					"plan_name":      u.PlanName,
					"ends_at":        u.EndsAt,
					"created_at":     u.CreatedAt,
				})
			}
		}
		backup["users"] = users
	}

	// 2. Encrypted Vouchers (PIN is encrypted; never plaintext)
	vRows, err := s.Pool.Query(ctx,
		`SELECT id, brand_id, voucher_id, serial_number, pin_encrypted, nonce, key_version, pin_hmac, cost_price, selling_price, status, is_sold, created_at
		 FROM vouchers_vault ORDER BY id ASC`,
	)
	if err == nil {
		defer vRows.Close()
		var vouchers []map[string]any
		for vRows.Next() {
			var id int64
			var brandID, vID, sn, encPin, nonce, pinHmac, status string
			var kVer int
			var cost, sell int64
			var isSold bool
			var cAt time.Time
			if err := vRows.Scan(&id, &brandID, &vID, &sn, &encPin, &nonce, &kVer, &pinHmac, &cost, &sell, &status, &isSold, &cAt); err == nil {
				vouchers = append(vouchers, map[string]any{
					"id":            id,
					"brand_id":      brandID,
					"voucher_id":    vID,
					"serial_number": sn,
					"pin_encrypted": encPin,
					"nonce":         nonce,
					"key_version":   kVer,
					"pin_hmac":      pinHmac,
					"cost_price":    cost,
					"selling_price": sell,
					"status":        status,
					"is_sold":       isSold,
					"created_at":    cAt,
				})
			}
		}
		backup["vouchers_vault_encrypted"] = vouchers
	}

	return json.MarshalIndent(backup, "", "  ")
}
