package surge

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrCapExceeded = errors.New("multiplier percent exceeds maximum configured surge cap")
)

type SurgeConfig struct {
	ID                int64     `json:"id"`
	MerchantID        int64     `json:"merchant_id"`
	IsEnabled         bool      `json:"is_enabled"`
	MultiplierPercent int       `json:"multiplier_percent"`
	MaxCapPercent     int       `json:"max_cap_percent"`
	Reason            string    `json:"reason"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type SurgeQuote struct {
	BasePrice    int64  `json:"base_price"`
	SurgePercent int    `json:"surge_percent"`
	SurgeAmount  int64  `json:"surge_amount"`
	FinalPrice   int64  `json:"final_price"`
	IsSurgeOn    bool   `json:"is_surge_on"`
	Reason       string `json:"reason"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// ComputeSurge calculates the dynamic surge adjustment on base price.
func ComputeSurge(basePrice int64, isEnabled bool, multiplierPct, maxCapPct int, reason string) SurgeQuote {
	if !isEnabled || multiplierPct <= 0 || basePrice <= 0 {
		return SurgeQuote{
			BasePrice:    basePrice,
			SurgePercent: 0,
			SurgeAmount:  0,
			FinalPrice:   basePrice,
			IsSurgeOn:    false,
			Reason:       "",
		}
	}

	effectivePct := multiplierPct
	if maxCapPct > 0 && effectivePct > maxCapPct {
		effectivePct = maxCapPct
	}

	surgeAmount := (basePrice * int64(effectivePct)) / 100
	finalPrice := basePrice + surgeAmount

	return SurgeQuote{
		BasePrice:    basePrice,
		SurgePercent: effectivePct,
		SurgeAmount:  surgeAmount,
		FinalPrice:   finalPrice,
		IsSurgeOn:    true,
		Reason:       reason,
	}
}

// GetQuote computes live surge quote for a specific merchant and base price.
func (s *Service) GetQuote(ctx context.Context, merchantID int64, basePrice int64) SurgeQuote {
	cfg := s.GetConfig(ctx, merchantID)
	return ComputeSurge(basePrice, cfg.IsEnabled, cfg.MultiplierPercent, cfg.MaxCapPercent, cfg.Reason)
}

// GetConfig returns merchant or global surge settings.
func (s *Service) GetConfig(ctx context.Context, merchantID int64) SurgeConfig {
	defaultCfg := SurgeConfig{
		MerchantID:        merchantID,
		IsEnabled:         false,
		MultiplierPercent: 0,
		MaxCapPercent:     15,
		Reason:            "",
		UpdatedAt:         time.Now().UTC(),
	}

	if s.pool == nil {
		return defaultCfg
	}

	query := `
		SELECT id, merchant_id, is_enabled, multiplier_percent, max_cap_percent, reason, updated_at
		FROM surge_config
		WHERE merchant_id = $1 OR merchant_id = 0
		ORDER BY merchant_id DESC LIMIT 1;
	`
	var cfg SurgeConfig
	err := s.pool.QueryRow(ctx, query, merchantID).Scan(
		&cfg.ID, &cfg.MerchantID, &cfg.IsEnabled, &cfg.MultiplierPercent, &cfg.MaxCapPercent, &cfg.Reason, &cfg.UpdatedAt,
	)
	if err != nil {
		return defaultCfg
	}
	return cfg
}

// UpdateConfig updates surge configuration.
func (s *Service) UpdateConfig(ctx context.Context, cfg SurgeConfig) error {
	if cfg.MaxCapPercent <= 0 {
		cfg.MaxCapPercent = 15
	}
	if cfg.MultiplierPercent > cfg.MaxCapPercent {
		return ErrCapExceeded
	}

	if s.pool == nil {
		return nil
	}

	query := `
		INSERT INTO surge_config (merchant_id, is_enabled, multiplier_percent, max_cap_percent, reason, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (id) DO UPDATE
		SET is_enabled = EXCLUDED.is_enabled,
		    multiplier_percent = EXCLUDED.multiplier_percent,
		    max_cap_percent = EXCLUDED.max_cap_percent,
		    reason = EXCLUDED.reason,
		    updated_at = now();
	`
	_, err := s.pool.Exec(ctx, query, cfg.MerchantID, cfg.IsEnabled, cfg.MultiplierPercent, cfg.MaxCapPercent, cfg.Reason)
	return err
}
