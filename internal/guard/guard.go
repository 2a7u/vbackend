package guard

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrPriceBelowCost = errors.New("PRICE_BELOW_COST")
)

type GuardSetting struct {
	ID               int64     `json:"id"`
	MerchantID       int64     `json:"merchant_id"`
	ItemID           string    `json:"item_id"`
	MinMarginIQD     int64     `json:"min_margin_iqd"`
	MinMarginPercent int       `json:"min_margin_percent"`
	IsActive         bool      `json:"is_active"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// CheckSalePriceTx verifies inside a database transaction that sellingPrice >= costPrice + min_margin.
// It checks specific item margin first, then merchant global margin ('*').
func (s *Service) CheckSalePriceTx(ctx context.Context, tx pgx.Tx, merchantID int64, itemID string, costPrice, sellingPrice int64) error {
	if costPrice < 0 || sellingPrice < 0 {
		return ErrPriceBelowCost
	}

	var minIQD int64
	var minPercent int
	var found bool

	// 1. Check specific item margin
	err := tx.QueryRow(ctx, `
		SELECT min_margin_iqd, min_margin_percent
		FROM guard_settings
		WHERE merchant_id = $1 AND item_id = $2 AND is_active = true
	`, merchantID, itemID).Scan(&minIQD, &minPercent)
	if err == nil {
		found = true
	} else if errors.Is(err, pgx.ErrNoRows) {
		// 2. Fallback to merchant global margin
		err = tx.QueryRow(ctx, `
			SELECT min_margin_iqd, min_margin_percent
			FROM guard_settings
			WHERE merchant_id = $1 AND item_id = '*' AND is_active = true
		`, merchantID).Scan(&minIQD, &minPercent)
		if err == nil {
			found = true
		}
	} else {
		return fmt.Errorf("guard query: %w", err)
	}

	minAllowed := costPrice
	if found {
		marginFromPercent := (costPrice * int64(minPercent)) / 100
		marginFromIQD := minIQD
		effectiveMargin := marginFromPercent
		if marginFromIQD > effectiveMargin {
			effectiveMargin = marginFromIQD
		}
		minAllowed = costPrice + effectiveMargin
	}

	if sellingPrice < minAllowed {
		return fmt.Errorf("%w: selling price (%d IQD) is below minimum allowed (%d IQD)", ErrPriceBelowCost, sellingPrice, minAllowed)
	}

	return nil
}

// CheckSalePrice provides standalone check without existing transaction
func (s *Service) CheckSalePrice(ctx context.Context, merchantID int64, itemID string, costPrice, sellingPrice int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	return s.CheckSalePriceTx(ctx, tx, merchantID, itemID, costPrice, sellingPrice)
}

func (s *Service) ListSettings(ctx context.Context, merchantID int64) ([]GuardSetting, error) {
	query := `
		SELECT id, merchant_id, item_id, min_margin_iqd, min_margin_percent, is_active, created_at, updated_at
		FROM guard_settings
		WHERE merchant_id = $1
		ORDER BY item_id ASC
	`
	rows, err := s.pool.Query(ctx, query, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []GuardSetting
	for rows.Next() {
		var g GuardSetting
		if err := rows.Scan(&g.ID, &g.MerchantID, &g.ItemID, &g.MinMarginIQD, &g.MinMarginPercent, &g.IsActive, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		settings = append(settings, g)
	}
	return settings, rows.Err()
}

func (s *Service) SaveSetting(ctx context.Context, g *GuardSetting) error {
	query := `
		INSERT INTO guard_settings (merchant_id, item_id, min_margin_iqd, min_margin_percent, is_active, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (merchant_id, item_id) DO UPDATE SET
			min_margin_iqd = EXCLUDED.min_margin_iqd,
			min_margin_percent = EXCLUDED.min_margin_percent,
			is_active = EXCLUDED.is_active,
			updated_at = now()
		RETURNING id, created_at, updated_at
	`
	return s.pool.QueryRow(ctx, query, g.MerchantID, g.ItemID, g.MinMarginIQD, g.MinMarginPercent, g.IsActive).Scan(&g.ID, &g.CreatedAt, &g.UpdatedAt)
}

func (s *Service) DeleteSetting(ctx context.Context, merchantID int64, itemID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM guard_settings WHERE merchant_id = $1 AND item_id = $2`, merchantID, itemID)
	return err
}
