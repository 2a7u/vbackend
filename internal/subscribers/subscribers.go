package subscribers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/ledger"
	"venus_backend/internal/middleware"
)

type Broadcaster interface {
	Broadcast(userID int64, event string, payload any)
}

type Service struct {
	Pool      *pgxpool.Pool
	Hub       Broadcaster
	JWTSecret string
	LedgerSvc *ledger.Service
}

func NewService(pool *pgxpool.Pool, hub Broadcaster, jwtSecret string, ledgerSvc *ledger.Service) *Service {
	return &Service{
		Pool:      pool,
		Hub:       hub,
		JWTSecret: jwtSecret,
		LedgerSvc: ledgerSvc,
	}
}

// Subscriber360 aggregates all data across profile, subscription, inventory, sales, debts, margins, ledger and sessions.
type Subscriber360 struct {
	User struct {
		ID            int64      `json:"id"`
		Email         string     `json:"email"`
		FullName      string     `json:"full_name"`
		StoreName     string     `json:"store_name"`
		Phone         string     `json:"phone"`
		Role          string     `json:"role"`
		IsApproved    bool       `json:"is_approved"`
		WalletBalance int64      `json:"wallet_balance"`
		CreditLimit   int64      `json:"credit_limit"`
		WalletStatus  string     `json:"wallet_status"`
		Status        string     `json:"status"`
		PlanName      string     `json:"plan_name"`
		EndsAt        *time.Time `json:"subscription_ends_at"`
		DeviceID      string     `json:"device_id"`
		DeviceModel   string     `json:"device_model"`
		AppVersion    string     `json:"app_version"`
		IsDeviceLocked bool      `json:"is_device_locked"`
		LastHeartbeat *time.Time `json:"last_heartbeat"`
		CreatedAt     time.Time  `json:"created_at"`
	} `json:"user"`

	SubscriptionStatus struct {
		PlanName   string `json:"plan_name"`
		DaysLeft   int    `json:"days_left"`
		IsExpired  bool   `json:"is_expired"`
		ActiveDays int    `json:"active_days"`
	} `json:"subscription_status"`

	InventorySummary struct {
		SimCount    int   `json:"sim_count"`
		ActiveSims  int   `json:"active_sims"`
		TotalCostIQD int64 `json:"total_cost_iqd"`
	} `json:"inventory_summary"`

	SalesSummary struct {
		SimSalesCount       int   `json:"sim_sales_count"`
		TotalSimSalesIQD    int64 `json:"total_sim_sales_iqd"`
		CreditSalesCount    int   `json:"credit_sales_count"`
		TotalCreditSalesIQD int64 `json:"total_credit_sales_iqd"`
		VoucherPurchases    int   `json:"voucher_purchases"`
		TotalVouchersIQD    int64 `json:"total_vouchers_iqd"`
	} `json:"sales_summary"`

	DebtsSummary struct {
		ActiveContractsCount int   `json:"active_contracts_count"`
		RemainingDebtIQD     int64 `json:"remaining_debt_iqd"`
		TotalPaymentsIQD     int64 `json:"total_payments_iqd"`
	} `json:"debts_summary"`

	ProfitsSummary struct {
		TotalProfitIQD int64 `json:"total_profit_iqd"`
	} `json:"profits_summary"`

	RecentLedgerEntries []ledger.LedgerEntry `json:"recent_ledger_entries"`
	LastActiveAt        *time.Time           `json:"last_active_at"`
}

// GetSubscriber360 compiles a complete 360-degree overview for a single subscriber.
func (s *Service) GetSubscriber360(ctx context.Context, userID int64) (*Subscriber360, error) {
	overview := &Subscriber360{}

	// 1. User profile & device metadata
	var (
		bal   float64
		ends  *time.Time
		hb    *time.Time
		cAt   time.Time
	)
	err := s.Pool.QueryRow(ctx,
		`SELECT id, email, COALESCE(full_name, ''), COALESCE(store_name, ''), COALESCE(phone, ''),
		        role, is_approved, COALESCE(wallet_balance, 0), COALESCE(credit_limit, 0),
		        COALESCE(wallet_status, 'active'), COALESCE(status, 'active'),
		        COALESCE(plan_name, 'مشترك تجريبي'), subscription_ends_at,
		        COALESCE(device_id, ''), COALESCE(device_model, ''), COALESCE(app_version, ''),
		        COALESCE(is_device_locked, false), last_heartbeat, created_at
		 FROM users WHERE id = $1`,
		userID,
	).Scan(
		&overview.User.ID, &overview.User.Email, &overview.User.FullName, &overview.User.StoreName,
		&overview.User.Phone, &overview.User.Role, &overview.User.IsApproved, &bal,
		&overview.User.CreditLimit, &overview.User.WalletStatus, &overview.User.Status,
		&overview.User.PlanName, &ends, &overview.User.DeviceID, &overview.User.DeviceModel,
		&overview.User.AppVersion, &overview.User.IsDeviceLocked, &hb, &cAt,
	)
	if err == pgx.ErrNoRows {
		return nil, errors.New("المشترك غير موجود")
	}
	if err != nil {
		return nil, fmt.Errorf("fetch user 360: %w", err)
	}

	overview.User.WalletBalance = int64(bal)
	overview.User.EndsAt = ends
	overview.User.LastHeartbeat = hb
	overview.User.CreatedAt = cAt

	// Subscription days calculation
	overview.SubscriptionStatus.PlanName = overview.User.PlanName
	if ends != nil {
		daysLeft := int(time.Until(*ends).Hours() / 24)
		if daysLeft < 0 {
			overview.SubscriptionStatus.DaysLeft = 0
			overview.SubscriptionStatus.IsExpired = true
		} else {
			overview.SubscriptionStatus.DaysLeft = daysLeft
			overview.SubscriptionStatus.IsExpired = false
		}
	} else {
		overview.SubscriptionStatus.IsExpired = true
	}

	// 2. SIM inventory
	_ = s.Pool.QueryRow(ctx,
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'available'), COALESCE(SUM(cost_price), 0)
		 FROM sim_inventory WHERE user_id = $1`,
		userID,
	).Scan(&overview.InventorySummary.SimCount, &overview.InventorySummary.ActiveSims, &overview.InventorySummary.TotalCostIQD)

	// 3. Sales statistics
	_ = s.Pool.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(SUM(sell_price), 0) FROM sim_sales WHERE user_id = $1`,
		userID,
	).Scan(&overview.SalesSummary.SimSalesCount, &overview.SalesSummary.TotalSimSalesIQD)

	_ = s.Pool.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(SUM(amount), 0) FROM credit_sales WHERE user_id = $1`,
		userID,
	).Scan(&overview.SalesSummary.CreditSalesCount, &overview.SalesSummary.TotalCreditSalesIQD)

	// 4. Voucher purchases
	_ = s.Pool.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(SUM(sold_price), 0), COALESCE(SUM(net_profit), 0)
		 FROM vouchers_vault WHERE sold_to_user_id = $1`,
		userID,
	).Scan(&overview.SalesSummary.VoucherPurchases, &overview.SalesSummary.TotalVouchersIQD, &overview.ProfitsSummary.TotalProfitIQD)

	// 5. Debts summary
	_ = s.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FILTER (WHERE status = 'open'), COALESCE(SUM(remaining_amount), 0)
		 FROM debt_contracts WHERE user_id = $1`,
		userID,
	).Scan(&overview.DebtsSummary.ActiveContractsCount, &overview.DebtsSummary.RemainingDebtIQD)

	_ = s.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM debt_payments WHERE user_id = $1`,
		userID,
	).Scan(&overview.DebtsSummary.TotalPaymentsIQD)

	// 6. Recent ledger entries (up to 20)
	lRows, lErr := s.Pool.Query(ctx,
		`SELECT id, tx_id, account_id, debit, credit, balance_after, currency, ref_type, ref_id, idempotency_key, created_at, prev_hash, hash
		 FROM ledger_entries
		 WHERE account_id = $1
		 ORDER BY id DESC LIMIT 20`,
		userID,
	)
	if lErr == nil {
		defer lRows.Close()
		for lRows.Next() {
			var e ledger.LedgerEntry
			if err := lRows.Scan(&e.ID, &e.TxID, &e.AccountID, &e.Debit, &e.Credit, &e.BalanceAfter, &e.Currency, &e.RefType, &e.RefID, &e.IdempotencyKey, &e.CreatedAt, &e.PrevHash, &e.Hash); err == nil {
				overview.RecentLedgerEntries = append(overview.RecentLedgerEntries, e)
			}
		}
	}

	overview.LastActiveAt = hb
	if overview.LastActiveAt == nil && len(overview.RecentLedgerEntries) > 0 {
		overview.LastActiveAt = &overview.RecentLedgerEntries[0].CreatedAt
	}

	return overview, nil
}

// BulkFilter criteria for mass subscriber operations.
type BulkFilter struct {
	PlanName            string `json:"plan_name"`
	ExpiringWithinHours int    `json:"expiring_within_hours"`
	Status              string `json:"status"`
}

type BulkActionRequest struct {
	BatchID    uuid.UUID  `json:"batch_id"`
	ActionType string     `json:"action_type"` // "add_gift_days", "send_notification"
	Filter     BulkFilter `json:"filter"`
	GiftDays   int        `json:"gift_days"`
	Message    string     `json:"message"`
	DryRun     bool       `json:"dry_run"`
}

type BulkActionResult struct {
	BatchID          string   `json:"batch_id"`
	DryRun           bool     `json:"dry_run"`
	SubscribersCount int      `json:"subscribers_count"`
	TargetUserIDs    []int64  `json:"target_user_ids"`
	Summary          string   `json:"summary"`
}

// ProcessBulkAction performs a preview (dry-run) or transactional batch action.
func (s *Service) ProcessBulkAction(ctx context.Context, adminID int64, req BulkActionRequest) (*BulkActionResult, error) {
	if req.BatchID == uuid.Nil {
		req.BatchID = uuid.New()
	}

	query := `
		SELECT id, email, full_name, subscription_ends_at
		FROM users
		WHERE role = 'subscriber'
		  AND ($1 = '' OR plan_name = $1)
		  AND ($2 = '' OR status = $2)
		  AND ($3 = 0 OR (subscription_ends_at IS NOT NULL AND subscription_ends_at <= now() + ($3 || ' hours')::interval))
	`
	rows, err := s.Pool.Query(ctx, query, req.Filter.PlanName, req.Filter.Status, req.Filter.ExpiringWithinHours)
	if err != nil {
		return nil, fmt.Errorf("filter subscribers for bulk action: %w", err)
	}
	defer rows.Close()

	var targetIDs []int64
	for rows.Next() {
		var uid int64
		var em, fn string
		var ends *time.Time
		if err := rows.Scan(&uid, &em, &fn, &ends); err == nil {
			targetIDs = append(targetIDs, uid)
		}
	}
	rows.Close()

	res := &BulkActionResult{
		BatchID:          req.BatchID.String(),
		DryRun:           req.DryRun,
		SubscribersCount: len(targetIDs),
		TargetUserIDs:    targetIDs,
	}

	if req.DryRun {
		res.Summary = fmt.Sprintf("معاينة تجريبية: سيتم تطبيق الإجراء على %d مشترك", len(targetIDs))
		return res, nil
	}

	// Idempotent execution: record batch in bulk_actions_log
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	for _, uid := range targetIDs {
		if req.ActionType == "add_gift_days" && req.GiftDays > 0 {
			_, _ = tx.Exec(ctx,
				`UPDATE users
				 SET subscription_ends_at = GREATEST(COALESCE(subscription_ends_at, now()), now()) + ($1 || ' days')::interval,
				     updated_at = now()
				 WHERE id = $2`,
				req.GiftDays, uid,
			)
		}
	}

	_, _ = tx.Exec(ctx,
		`INSERT INTO bulk_actions_log (batch_id, action_type, filters, subscribers_count, status, executed_by)
		 VALUES ($1, $2, $3, $4, 'completed', $5)`,
		req.BatchID, req.ActionType, fmt.Sprintf(`{"plan": "%s", "hours": %d}`, req.Filter.PlanName, req.Filter.ExpiringWithinHours),
		len(targetIDs), adminID,
	)

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	if req.ActionType == "send_notification" && req.Message != "" && s.Hub != nil {
		for _, uid := range targetIDs {
			s.Hub.Broadcast(uid, "announcement", map[string]any{
				"title":   "تنبيه خاص للمشتركين",
				"message": req.Message,
			})
		}
	}

	res.Summary = fmt.Sprintf("تم تنفيذ الإجراء بنجاح على %d مشترك وتوثيقه بالرقم التعريفي %s", len(targetIDs), req.BatchID)
	return res, nil
}

// Session management & device controls
func (s *Service) ResetDeviceFingerprint(ctx context.Context, userID int64) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE users
		 SET device_id = '', device_model = '', is_device_locked = false, updated_at = now()
		 WHERE id = $1`,
		userID,
	)
	return err
}

func (s *Service) SetDeviceLock(ctx context.Context, userID int64, locked bool) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE users
		 SET is_device_locked = $1, updated_at = now()
		 WHERE id = $2`,
		locked, userID,
	)
	return err
}

func (s *Service) EvictAllSessions(ctx context.Context, userID int64) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE users
		 SET token_version = token_version + 1, updated_at = now()
		 WHERE id = $1`,
		userID,
	)
	if err == nil && s.Hub != nil {
		s.Hub.Broadcast(userID, "session_revoked", map[string]string{
			"reason": "تم إنهاء الجلسة بواسطة إدارة المنظمة",
		})
	}
	return err
}

// CreateImpersonationToken signs a read-only token to securely view the subscriber view without write access.
func (s *Service) CreateImpersonationToken(ctx context.Context, adminID, targetUserID int64, reason string) (string, error) {
	var email, role string
	err := s.Pool.QueryRow(ctx, `SELECT email, role FROM users WHERE id = $1`, targetUserID).Scan(&email, &role)
	if err != nil {
		return "", errors.New("المشترك المطلوب غير موجود")
	}

	// Issue short-lived read-only token (valid 1 hour)
	token, err := middleware.SignImpersonationToken(s.JWTSecret, targetUserID, email, role, adminID, 1*time.Hour)
	if err != nil {
		return "", err
	}

	// Record in audit logs
	_, _ = s.Pool.Exec(ctx,
		`INSERT INTO audit_logs (user_id, actor_id, action, entity, entity_id, details)
		 VALUES ($1, $2, 'impersonation_session_started', 'user', $3, $4)`,
		targetUserID, adminID, fmt.Sprintf("%d", targetUserID),
		fmt.Sprintf(`{"reason": "%s", "expires_in": "1h"}`, reason),
	)

	return token, nil
}
