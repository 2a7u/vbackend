package rewards

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/ledger"
)

var (
	ErrRewardAlreadyClaimed = errors.New("reward already credited for this period")
	ErrTargetNotReached     = errors.New("sales target has not been reached")
)

type RewardRule struct {
	ID              int64     `json:"id"`
	MerchantID      int64     `json:"merchant_id"`
	Title           string    `json:"title"`
	TargetSalesIQD  int64     `json:"target_sales_iqd"`
	RewardAmountIQD int64     `json:"reward_amount_iqd"`
	PeriodType      string    `json:"period_type"` // daily, weekly, monthly
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type RewardClaimResult struct {
	RuleID         int64     `json:"rule_id"`
	MerchantID     int64     `json:"merchant_id"`
	PeriodKey      string    `json:"period_key"`
	AchievedSales  int64     `json:"achieved_sales"`
	RewardAmount   int64     `json:"reward_amount"`
	LedgerTxID     string    `json:"ledger_tx_id"`
	CreditedAt     time.Time `json:"credited_at"`
}

type Service struct {
	pool      *pgxpool.Pool
	ledgerSvc *ledger.Service
}

func NewService(pool *pgxpool.Pool, ledgerSvc *ledger.Service) *Service {
	return &Service{
		pool:      pool,
		ledgerSvc: ledgerSvc,
	}
}

// CurrentPeriodKey returns standard period key e.g. "2026-W46" or "2026-11".
func CurrentPeriodKey(periodType string, t time.Time) string {
	loc := time.FixedZone("Asia/Baghdad", 3*60*60)
	bt := t.In(loc)
	year, week := bt.ISOWeek()
	switch periodType {
	case "monthly":
		return fmt.Sprintf("%d-%02d", bt.Year(), bt.Month())
	case "daily":
		return bt.Format("2006-01-02")
	default: // weekly
		return fmt.Sprintf("%d-W%02d", year, week)
	}
}

// EvaluateAndReward checks merchant sales and deposits reward if rule target is met.
func (s *Service) EvaluateAndReward(ctx context.Context, merchantID, ruleID int64) (*RewardClaimResult, error) {
	if s.pool == nil {
		return nil, nil
	}

	// 1. Fetch rule
	var rule RewardRule
	err := s.pool.QueryRow(ctx, `
		SELECT id, merchant_id, title, target_sales_iqd, reward_amount_iqd, period_type, is_active, created_at, updated_at
		FROM rewards_rules
		WHERE id = $1 AND is_active = TRUE;
	`, ruleID).Scan(&rule.ID, &rule.MerchantID, &rule.Title, &rule.TargetSalesIQD, &rule.RewardAmountIQD, &rule.PeriodType, &rule.IsActive, &rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("rule not found or inactive: %w", err)
	}

	periodKey := CurrentPeriodKey(rule.PeriodType, time.Now())

	// 2. Check if already claimed for this period
	var existingCount int
	_ = s.pool.QueryRow(ctx, `
		SELECT count(*) FROM rewards_ledger
		WHERE merchant_id = $1 AND rule_id = $2 AND period_key = $3;
	`, merchantID, ruleID, periodKey).Scan(&existingCount)
	if existingCount > 0 {
		return nil, ErrRewardAlreadyClaimed
	}

	// 3. Compute achieved sales
	var achievedSales float64
	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(sale_price), 0)
		FROM sales
		WHERE user_id = $1 AND created_at >= date_trunc('week', now());
	`, merchantID).Scan(&achievedSales)
	if err != nil {
		return nil, fmt.Errorf("calculate sales: %w", err)
	}

	achievedIQD := int64(achievedSales)
	if achievedIQD < rule.TargetSalesIQD {
		return nil, fmt.Errorf("%w: achieved %d IQD / required %d IQD", ErrTargetNotReached, achievedIQD, rule.TargetSalesIQD)
	}

	// 4. Double-Entry Ledger credit in a single transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin reward tx: %w", err)
	}
	defer tx.Rollback(ctx)

	revenueAccountID, err := s.ledgerSvc.GetSystemAccountID(ctx, ledger.SystemEmailRevenue)
	if err != nil {
		return nil, fmt.Errorf("lookup system revenue account: %w", err)
	}

	txUUID := uuid.New()
	idemKey := fmt.Sprintf("reward_%d_%d_%s", merchantID, ruleID, periodKey)

	doubleEntryReq := ledger.DoubleEntryRequest{
		TxID:           txUUID,
		RefType:        "reward_bonus",
		RefID:          fmt.Sprintf("%d", ruleID),
		IdempotencyKey: idemKey,
		CreatedAt:      time.Now().UTC(),
		Legs: []ledger.Leg{
			{AccountID: revenueAccountID, Debit: rule.RewardAmountIQD, Credit: 0},
			{AccountID: merchantID, Debit: 0, Credit: rule.RewardAmountIQD},
		},
	}

	_, err = s.ledgerSvc.ExecuteDoubleEntry(ctx, tx, doubleEntryReq)
	if err != nil {
		return nil, fmt.Errorf("execute reward double entry: %w", err)
	}

	// 5. Record claim in rewards_ledger
	insertLedgerQuery := `
		INSERT INTO rewards_ledger (merchant_id, rule_id, period_key, achieved_sales_iqd, reward_amount_iqd, ledger_tx_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, now());
	`
	_, err = tx.Exec(ctx, insertLedgerQuery, merchantID, ruleID, periodKey, achievedIQD, rule.RewardAmountIQD, txUUID.String())
	if err != nil {
		return nil, fmt.Errorf("record reward ledger: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit reward tx: %w", err)
	}

	return &RewardClaimResult{
		RuleID:        ruleID,
		MerchantID:    merchantID,
		PeriodKey:     periodKey,
		AchievedSales: achievedIQD,
		RewardAmount:  rule.RewardAmountIQD,
		LedgerTxID:    txUUID.String(),
		CreditedAt:    doubleEntryReq.CreatedAt,
	}, nil
}

// ListRules retrieves rewards rules.
func (s *Service) ListRules(ctx context.Context, merchantID int64) ([]RewardRule, error) {
	if s.pool == nil {
		return []RewardRule{}, nil
	}
	query := `
		SELECT id, merchant_id, title, target_sales_iqd, reward_amount_iqd, period_type, is_active, created_at, updated_at
		FROM rewards_rules
		WHERE merchant_id = $1 OR merchant_id = 0
		ORDER BY id ASC;
	`
	rows, err := s.pool.Query(ctx, query, merchantID)
	if err != nil {
		return nil, fmt.Errorf("list reward rules: %w", err)
	}
	defer rows.Close()

	var list []RewardRule
	for rows.Next() {
		var r RewardRule
		if err := rows.Scan(&r.ID, &r.MerchantID, &r.Title, &r.TargetSalesIQD, &r.RewardAmountIQD, &r.PeriodType, &r.IsActive, &r.CreatedAt, &r.UpdatedAt); err == nil {
			list = append(list, r)
		}
	}
	return list, nil
}

// SaveRule inserts or updates a reward rule.
func (s *Service) SaveRule(ctx context.Context, r RewardRule) error {
	if s.pool == nil {
		return nil
	}
	query := `
		INSERT INTO rewards_rules (merchant_id, title, target_sales_iqd, reward_amount_iqd, period_type, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, now(), now());
	`
	_, err := s.pool.Exec(ctx, query, r.MerchantID, r.Title, r.TargetSalesIQD, r.RewardAmountIQD, r.PeriodType, r.IsActive)
	return err
}

// DeleteRule removes a reward rule.
func (s *Service) DeleteRule(ctx context.Context, id int64) error {
	if s.pool == nil {
		return nil
	}
	_, err := s.pool.Exec(ctx, "DELETE FROM rewards_rules WHERE id = $1;", id)
	return err
}
