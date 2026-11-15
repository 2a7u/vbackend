package promotions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/ledger"
)

type Service struct {
	Pool      *pgxpool.Pool
	LedgerSvc *ledger.Service
}

func NewService(pool *pgxpool.Pool, ledgerSvc *ledger.Service) *Service {
	return &Service{
		Pool:      pool,
		LedgerSvc: ledgerSvc,
	}
}

type Coupon struct {
	ID                         int64      `json:"id"`
	Code                       string     `json:"code"`
	DiscountType               string     `json:"discount_type"` // 'percentage', 'fixed'
	DiscountValue              int64      `json:"discount_value"`
	MaxUses                    int        `json:"max_uses"`
	UsedCount                  int        `json:"used_count"`
	MaxUsesPerUser             int        `json:"max_uses_per_user"`
	RepresentativeID           *int64     `json:"representative_id,omitempty"`
	RepresentativeName         string     `json:"representative_name,omitempty"`
	RepresentativeCommissionIQD int64     `json:"representative_commission_iqd"`
	ExpiresAt                  *time.Time `json:"expires_at,omitempty"`
	IsActive                   bool       `json:"is_active"`
	CreatedAt                  time.Time  `json:"created_at"`
}

type RedemptionResult struct {
	CouponID            int64 `json:"coupon_id"`
	OriginalAmountIQD   int64 `json:"original_amount_iqd"`
	DiscountAppliedIQD  int64 `json:"discount_applied_iqd"`
	FinalAmountIQD      int64 `json:"final_amount_iqd"`
	RepresentativeID    *int64 `json:"representative_id,omitempty"`
	RepCommissionIQD    int64 `json:"representative_commission_iqd,omitempty"`
}

// CreateCoupon provisions a new discount or representative coupon code.
func (s *Service) CreateCoupon(ctx context.Context, c Coupon) (*Coupon, error) {
	c.Code = strings.ToUpper(strings.TrimSpace(c.Code))
	if c.Code == "" || c.DiscountValue <= 0 {
		return nil, errors.New("كود الخصم وقيمة الخصم إلزاميان")
	}
	if c.DiscountType != "percentage" && c.DiscountType != "fixed" {
		c.DiscountType = "percentage"
	}
	if c.DiscountType == "percentage" && c.DiscountValue > 100 {
		return nil, errors.New("نسبة الخصم المئوية لا يمكن أن تتجاوز 100%")
	}
	if c.MaxUses <= 0 {
		c.MaxUses = 100
	}
	if c.MaxUsesPerUser <= 0 {
		c.MaxUsesPerUser = 1
	}

	var id int64
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO coupons (code, discount_type, discount_value, max_uses, max_uses_per_user,
		                     representative_id, representative_commission_iqd, expires_at, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, created_at`,
		c.Code, c.DiscountType, c.DiscountValue, c.MaxUses, c.MaxUsesPerUser,
		c.RepresentativeID, c.RepresentativeCommissionIQD, c.ExpiresAt, c.IsActive,
	).Scan(&id, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create coupon: %w", err)
	}
	c.ID = id
	return &c, nil
}

// ValidateAndApplyCoupon evaluates and consumes a coupon inside an atomic transaction.
func (s *Service) ValidateAndApplyCoupon(ctx context.Context, tx pgx.Tx, code string, userID int64, originalAmountIQD int64) (*RedemptionResult, error) {
	cleanCode := strings.ToUpper(strings.TrimSpace(code))
	if cleanCode == "" {
		return &RedemptionResult{
			OriginalAmountIQD:  originalAmountIQD,
			DiscountAppliedIQD: 0,
			FinalAmountIQD:     originalAmountIQD,
		}, nil
	}

	var (
		cID         int64
		discType    string
		discVal     int64
		maxUses     int
		usedCount   int
		maxPerUser  int
		repID       *int64
		repComm     int64
		expiresAt   *time.Time
		isActive    bool
	)
	err := tx.QueryRow(ctx,
		`SELECT id, discount_type, discount_value, max_uses, used_count, max_uses_per_user,
		        representative_id, representative_commission_iqd, expires_at, is_active
		 FROM coupons
		 WHERE code = $1
		 FOR UPDATE`,
		cleanCode,
	).Scan(&cID, &discType, &discVal, &maxUses, &usedCount, &maxPerUser, &repID, &repComm, &expiresAt, &isActive)
	if err == pgx.ErrNoRows {
		return nil, errors.New("كود الخصم غير موجود أو غير صالح")
	}
	if err != nil {
		return nil, fmt.Errorf("query coupon: %w", err)
	}

	if !isActive {
		return nil, errors.New("كود الخصم معطل حالياً")
	}
	if expiresAt != nil && expiresAt.Before(time.Now().UTC()) {
		return nil, errors.New("كود الخصم منتهي الصلاحية")
	}
	if maxUses > 0 && usedCount >= maxUses {
		return nil, errors.New("تم استنفاد الحد الأقصى لاستخدام كود الخصم")
	}

	// Check per-user redemption limit
	var userUsageCount int
	_ = tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM coupon_redemptions WHERE coupon_id = $1 AND user_id = $2`,
		cID, userID,
	).Scan(&userUsageCount)
	if maxPerUser > 0 && userUsageCount >= maxPerUser {
		return nil, errors.New("لقد قمت باستخدام كود الخصم هذا بالحد الأقصى المسموح به")
	}

	var discountApplied int64
	if discType == "percentage" {
		discountApplied = (originalAmountIQD * discVal) / 100
	} else {
		discountApplied = discVal
	}
	if discountApplied > originalAmountIQD {
		discountApplied = originalAmountIQD
	}
	finalAmount := originalAmountIQD - discountApplied

	// Record redemption
	_, err = tx.Exec(ctx,
		`INSERT INTO coupon_redemptions (coupon_id, user_id, discount_applied_iqd)
		 VALUES ($1, $2, $3)`,
		cID, userID, discountApplied,
	)
	if err != nil {
		return nil, fmt.Errorf("record coupon redemption: %w", err)
	}

	// Increment coupon counter
	_, _ = tx.Exec(ctx, `UPDATE coupons SET used_count = used_count + 1 WHERE id = $1`, cID)

	// If representative has commission, credit representative wallet via double-entry
	if repID != nil && *repID > 0 && repComm > 0 && s.LedgerSvc != nil {
		revenueAccID, rErr := s.LedgerSvc.GetSystemAccountIDTx(ctx, tx, ledger.SystemEmailRevenue)
		if rErr == nil {
			_, _ = s.LedgerSvc.ExecuteDoubleEntry(ctx, tx, ledger.DoubleEntryRequest{
				RefType: "coupon_rep_commission",
				RefID:   fmt.Sprintf("coupon_%d_user_%d", cID, userID),
				Legs: []ledger.Leg{
					{AccountID: revenueAccID, Debit: repComm, Credit: 0},
					{AccountID: *repID, Debit: 0, Credit: repComm},
				},
				CreatedAt: time.Now().UTC(),
			})
		}
	}

	return &RedemptionResult{
		CouponID:            cID,
		OriginalAmountIQD:   originalAmountIQD,
		DiscountAppliedIQD:  discountApplied,
		FinalAmountIQD:      finalAmount,
		RepresentativeID:    repID,
		RepCommissionIQD:    repComm,
	}, nil
}

// ListCoupons returns all coupons with representative info.
func (s *Service) ListCoupons(ctx context.Context) ([]Coupon, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT c.id, c.code, c.discount_type, c.discount_value, c.max_uses, c.used_count,
		        c.max_uses_per_user, c.representative_id, COALESCE(u.full_name, ''),
		        c.representative_commission_iqd, c.expires_at, c.is_active, c.created_at
		 FROM coupons c
		 LEFT JOIN users u ON c.representative_id = u.id
		 ORDER BY c.id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Coupon
	for rows.Next() {
		var c Coupon
		if err := rows.Scan(
			&c.ID, &c.Code, &c.DiscountType, &c.DiscountValue, &c.MaxUses, &c.UsedCount,
			&c.MaxUsesPerUser, &c.RepresentativeID, &c.RepresentativeName,
			&c.RepresentativeCommissionIQD, &c.ExpiresAt, &c.IsActive, &c.CreatedAt,
		); err == nil {
			list = append(list, c)
		}
	}
	return list, nil
}
