package appconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Broadcaster interface {
	Broadcast(userID int64, event string, payload any)
}

type Service struct {
	Pool *pgxpool.Pool
	Hub  Broadcaster

	mu        sync.RWMutex
	cache     map[string]map[string]any
	cacheTime time.Time
	cacheTTL  time.Duration
}

func NewService(pool *pgxpool.Pool, hub Broadcaster) *Service {
	s := &Service{
		Pool:     pool,
		Hub:      hub,
		cache:    make(map[string]map[string]any),
		cacheTTL: 30 * time.Second,
	}
	_ = s.RefreshCache(context.Background())
	return s
}

func (s *Service) RefreshCache(ctx context.Context) error {
	if s.Pool == nil {
		return nil
	}
	rows, err := s.Pool.Query(ctx, `SELECT config_key, config_value FROM app_config`)
	if err != nil {
		return err
	}
	defer rows.Close()

	newCache := make(map[string]map[string]any)
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err == nil {
			var val map[string]any
			if json.Unmarshal(raw, &val) == nil {
				newCache[key] = val
			}
		}
	}

	s.mu.Lock()
	s.cache = newCache
	s.cacheTime = time.Now()
	s.mu.Unlock()
	return nil
}

func (s *Service) Get(key string) map[string]any {
	s.mu.RLock()
	if time.Since(s.cacheTime) > s.cacheTTL {
		s.mu.RUnlock()
		_ = s.RefreshCache(context.Background())
		s.mu.RLock()
	}
	val, ok := s.cache[key]
	s.mu.RUnlock()
	if ok {
		return val
	}
	return make(map[string]any)
}

func (s *Service) Set(ctx context.Context, key string, val map[string]any, updatedBy int64) error {
	if s.Pool != nil {
		raw, err := json.Marshal(val)
		if err != nil {
			return err
		}

		_, err = s.Pool.Exec(ctx,
			`INSERT INTO app_config (config_key, config_value, version, updated_by, updated_at)
			 VALUES ($1, $2, 1, $3, now())
			 ON CONFLICT (config_key) DO UPDATE
			 SET config_value = $2, version = app_config.version + 1, updated_by = $3, updated_at = now()`,
			key, raw, updatedBy,
		)
		if err != nil {
			return fmt.Errorf("set app config %s: %w", key, err)
		}
	}

	// Instant in-memory cache update
	s.mu.Lock()
	s.cache[key] = val
	s.mu.Unlock()

	// Push change live via WebSocket
	if s.Hub != nil {
		s.Hub.Broadcast(0, "app_config_updated", map[string]any{
			"key":   key,
			"value": val,
		})
	}
	return nil
}

func (s *Service) GetAllConfigs() map[string]map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copied := make(map[string]map[string]any)
	for k, v := range s.cache {
		copied[k] = v
	}
	return copied
}

// Helpers for fast runtime checks

func (s *Service) IsMaintenanceActive(userID int64) (bool, string) {
	cfg := s.Get("maintenance_mode")
	enabled, _ := cfg["enabled"].(bool)
	if !enabled {
		return false, ""
	}
	msg, _ := cfg["message"].(string)
	if msg == "" {
		msg = "النظام يخضع للصيانة المجدولة حالياً، يرجى المحاولة بعد قليل"
	}
	if wList, ok := cfg["whitelisted_uids"].([]any); ok {
		for _, w := range wList {
			if num, ok := w.(float64); ok && int64(num) == userID {
				return false, ""
			}
		}
	}
	return true, msg
}

func (s *Service) CheckForceUpdate(clientVersion string) (bool, string, string) {
	cfg := s.Get("force_update")
	minVer, _ := cfg["min_version"].(string)
	url, _ := cfg["update_url"].(string)
	forced, _ := cfg["is_forced"].(bool)

	if !forced || minVer == "" {
		return false, "", ""
	}
	if strings.TrimSpace(clientVersion) < strings.TrimSpace(minVer) {
		return true, minVer, url
	}
	return false, "", ""
}

func (s *Service) IsFeatureEnabled(featureName string) bool {
	cfg := s.Get("feature_flags")
	if enabled, ok := cfg[featureName].(bool); ok {
		return enabled
	}
	return true // default allow if unspecified
}

type Banner struct {
	ID           int64      `json:"id"`
	Title        string     `json:"title"`
	ImageURL     string     `json:"image_url"`
	TargetURL    string     `json:"target_url"`
	DisplayOrder int        `json:"display_order"`
	IsActive     bool       `json:"is_active"`
	StartsAt     *time.Time `json:"starts_at,omitempty"`
	EndsAt       *time.Time `json:"ends_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (s *Service) ListBanners(ctx context.Context, activeOnly bool) ([]Banner, error) {
	query := `
		SELECT id, title, image_url, target_url, display_order, is_active, starts_at, ends_at, created_at
		FROM app_banners
		WHERE ($1 = false OR (is_active = true AND (starts_at IS NULL OR starts_at <= now()) AND (ends_at IS NULL OR ends_at >= now())))
		ORDER BY display_order ASC, id DESC
	`
	rows, err := s.Pool.Query(ctx, query, activeOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Banner
	for rows.Next() {
		var b Banner
		if err := rows.Scan(&b.ID, &b.Title, &b.ImageURL, &b.TargetURL, &b.DisplayOrder, &b.IsActive, &b.StartsAt, &b.EndsAt, &b.CreatedAt); err == nil {
			list = append(list, b)
		}
	}
	return list, nil
}

func (s *Service) SaveBanner(ctx context.Context, b Banner) (*Banner, error) {
	if b.ImageURL == "" {
		return nil, fmt.Errorf("image_url is required")
	}
	if b.ID > 0 {
		_, err := s.Pool.Exec(ctx,
			`UPDATE app_banners
			 SET title = $1, image_url = $2, target_url = $3, display_order = $4, is_active = $5, starts_at = $6, ends_at = $7
			 WHERE id = $8`,
			b.Title, b.ImageURL, b.TargetURL, b.DisplayOrder, b.IsActive, b.StartsAt, b.EndsAt, b.ID,
		)
		return &b, err
	}

	err := s.Pool.QueryRow(ctx,
		`INSERT INTO app_banners (title, image_url, target_url, display_order, is_active, starts_at, ends_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, created_at`,
		b.Title, b.ImageURL, b.TargetURL, b.DisplayOrder, b.IsActive, b.StartsAt, b.EndsAt,
	).Scan(&b.ID, &b.CreatedAt)
	return &b, err
}

func (s *Service) DeleteBanner(ctx context.Context, id int64) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM app_banners WHERE id = $1`, id)
	return err
}
