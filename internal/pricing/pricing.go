package pricing

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ResolvedPrice contains the resolved monetary price and source determination.
type ResolvedPrice struct {
	ProductID     string `json:"product_id"`
	Price         int64  `json:"price"` // in IQD
	Source        string `json:"source"` // "subscriber_override", "tier_price", "catalog_default"
	TierCode      string `json:"tier_code,omitempty"`
	OriginalPrice int64  `json:"original_price"`
}

type PriceTier struct {
	ID          int64     `json:"id"`
	Code        string    `json:"code"`
	NameAr      string    `json:"name_ar"`
	Description string    `json:"description"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
}

type Service struct {
	Pool  *pgxpool.Pool
	mu    sync.RWMutex
	cache map[string]cachedPrice
}

type cachedPrice struct {
	resolved  ResolvedPrice
	expiresAt time.Time
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		Pool:  pool,
		cache: make(map[string]cachedPrice),
	}
}

// Invalidate clears user/product specific entries or the whole pricing cache.
func (s *Service) Invalidate(userID int64, productID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if userID == 0 && productID == "" {
		s.cache = make(map[string]cachedPrice)
		return
	}

	for k := range s.cache {
		parts := strings.Split(k, ":")
		if len(parts) == 2 {
			if (userID > 0 && parts[0] == fmt.Sprintf("%d", userID)) || (productID != "" && parts[1] == productID) {
				delete(s.cache, k)
			}
		}
	}
}

// ResolvePrice implements strict resolution order:
// 1. Subscriber price override
// 2. Assigned / Default Tier price
// 3. Public catalog default price
func (s *Service) ResolvePrice(ctx context.Context, userID int64, productID string, catalogDefaultPrice int64) (ResolvedPrice, error) {
	cacheKey := fmt.Sprintf("%d:%s", userID, productID)

	s.mu.RLock()
	if item, found := s.cache[cacheKey]; found && time.Now().Before(item.expiresAt) {
		s.mu.RUnlock()
		return item.resolved, nil
	}
	s.mu.RUnlock()

	resolved := ResolvedPrice{
		ProductID:     productID,
		Price:         catalogDefaultPrice,
		Source:        "catalog_default",
		OriginalPrice: catalogDefaultPrice,
	}

	if s.Pool == nil {
		return resolved, nil
	}

	// 1. Check Subscriber Override
	if userID > 0 {
		var customPrice int64
		err := s.Pool.QueryRow(ctx,
			`SELECT custom_price
			 FROM subscriber_price_overrides
			 WHERE user_id = $1 AND product_id = $2`,
			userID, productID,
		).Scan(&customPrice)
		if err == nil {
			resolved.Price = customPrice
			resolved.Source = "subscriber_override"
			s.setCache(cacheKey, resolved)
			return resolved, nil
		}
	}

	// 2. Check Tier Price
	var tierID *int64
	var tierCode string
	if userID > 0 {
		_ = s.Pool.QueryRow(ctx,
			`SELECT u.price_tier_id, COALESCE(pt.code, '')
			 FROM users u
			 LEFT JOIN price_tiers pt ON pt.id = u.price_tier_id
			 WHERE u.id = $1`,
			userID,
		).Scan(&tierID, &tierCode)
	}

	if tierID == nil {
		// Fallback to default tier
		var defaultID int64
		err := s.Pool.QueryRow(ctx,
			`SELECT id, code FROM price_tiers WHERE is_default = TRUE LIMIT 1`,
		).Scan(&defaultID, &tierCode)
		if err == nil {
			tierID = &defaultID
		}
	}

	if tierID != nil {
		var tierPrice int64
		err := s.Pool.QueryRow(ctx,
			`SELECT price FROM tier_prices WHERE tier_id = $1 AND product_id = $2`,
			*tierID, productID,
		).Scan(&tierPrice)
		if err == nil {
			resolved.Price = tierPrice
			resolved.Source = "tier_price"
			resolved.TierCode = tierCode
			s.setCache(cacheKey, resolved)
			return resolved, nil
		}
	}

	// 3. Fallback to Catalog Default
	s.setCache(cacheKey, resolved)
	return resolved, nil
}

func (s *Service) setCache(key string, res ResolvedPrice) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[key] = cachedPrice{
		resolved:  res,
		expiresAt: time.Now().Add(5 * time.Minute),
	}
}

// ComputeProfit calculates platform net profit in Iraqi Dinar.
func ComputeProfit(costPrice, sellPrice int64) int64 {
	return sellPrice - costPrice
}

// ListTiers returns all active pricing tiers.
func (s *Service) ListTiers(ctx context.Context) ([]PriceTier, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, code, name_ar, description, is_default, created_at
		 FROM price_tiers
		 ORDER BY id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tiers []PriceTier
	for rows.Next() {
		var t PriceTier
		if err := rows.Scan(&t.ID, &t.Code, &t.NameAr, &t.Description, &t.IsDefault, &t.CreatedAt); err == nil {
			tiers = append(tiers, t)
		}
	}
	return tiers, nil
}

// SetTierPrice assigns or updates a product price for a specific tier.
func (s *Service) SetTierPrice(ctx context.Context, tierID int64, productID string, price int64) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO tier_prices (tier_id, product_id, price, updated_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (tier_id, product_id)
		 DO UPDATE SET price = EXCLUDED.price, updated_at = now()`,
		tierID, productID, price,
	)
	if err == nil {
		s.Invalidate(0, productID)
	}
	return err
}

// SetSubscriberOverride sets an exclusive price override for a single subscriber.
func (s *Service) SetSubscriberOverride(ctx context.Context, userID int64, productID string, customPrice int64) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO subscriber_price_overrides (user_id, product_id, custom_price, updated_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (user_id, product_id)
		 DO UPDATE SET custom_price = EXCLUDED.custom_price, updated_at = now()`,
		userID, productID, customPrice,
	)
	if err == nil {
		s.Invalidate(userID, productID)
	}
	return err
}

// RemoveSubscriberOverride deletes a custom override, falling back to tier/catalog.
func (s *Service) RemoveSubscriberOverride(ctx context.Context, userID int64, productID string) error {
	_, err := s.Pool.Exec(ctx,
		`DELETE FROM subscriber_price_overrides WHERE user_id = $1 AND product_id = $2`,
		userID, productID,
	)
	if err == nil {
		s.Invalidate(userID, productID)
	}
	return err
}
