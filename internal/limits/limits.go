package limits

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BlockedEntity struct {
	ID             int64     `json:"id"`
	MerchantID     int64     `json:"merchant_id"`
	IP             string    `json:"ip"`
	Identifier     string    `json:"identifier"`
	ActionType     string    `json:"action_type"`
	FailedAttempts int       `json:"failed_attempts"`
	BlockedUntil   time.Time `json:"blocked_until"`
	Reason         string    `json:"reason"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type tokenBucket struct {
	tokens     float64
	capacity   float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

type Service struct {
	pool    *pgxpool.Pool
	mu      sync.Mutex
	buckets map[string]*tokenBucket
}

func NewService(pool *pgxpool.Pool) *Service {
	s := &Service{
		pool:    pool,
		buckets: make(map[string]*tokenBucket),
	}
	// Background cleanup of stale buckets every 10 minutes
	go s.cleanupLoop()
	return s
}

func (s *Service) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for k, b := range s.buckets {
			if now.Sub(b.lastRefill) > 30*time.Minute {
				delete(s.buckets, k)
			}
		}
		s.mu.Unlock()
	}
}

// Allow checks in-memory token bucket. Capacity default 60, refill 2 tokens/sec (120 req/min).
func (s *Service) Allow(key string, capacity, refillRate float64) (bool, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	b, exists := s.buckets[key]
	if !exists {
		b = &tokenBucket{
			tokens:     capacity - 1.0,
			capacity:   capacity,
			refillRate: refillRate,
			lastRefill: now,
		}
		s.buckets[key] = b
		return true, 0
	}

	// Refill
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	b.lastRefill = now

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true, 0
	}

	needed := 1.0 - b.tokens
	retryAfter := int(needed / b.refillRate)
	if retryAfter < 1 {
		retryAfter = 1
	}
	return false, retryAfter
}

// IsBlocked checks if IP or merchant is currently blocked in DB
func (s *Service) IsBlocked(ctx context.Context, ip string, merchantID int64) (bool, int, string, error) {
	if s.pool == nil {
		return false, 0, "", nil
	}
	var blockedUntil time.Time
	var reason string

	err := s.pool.QueryRow(ctx, `
		SELECT blocked_until, reason
		FROM limits_blocks
		WHERE blocked_until > now() AND (
			(ip <> '' AND ip = $1) OR
			($2 > 0 AND merchant_id = $2)
		)
		ORDER BY blocked_until DESC
		LIMIT 1
	`, ip, merchantID).Scan(&blockedUntil, &reason)
	if err == nil {
		diff := int(time.Until(blockedUntil).Seconds())
		if diff < 1 {
			diff = 1
		}
		return true, diff, reason, nil
	}

	return false, 0, "", nil
}

// RecordFailedAttempt increments fail counter and blocks if attempts >= maxAttempts
func (s *Service) RecordFailedAttempt(ctx context.Context, merchantID int64, ip, identifier, actionType string, maxAttempts int, blockDuration time.Duration) (bool, time.Time, error) {
	if s.pool == nil {
		return false, time.Now(), nil
	}
	var currentAttempts int
	var id int64
	var blockedUntil time.Time

	err := s.pool.QueryRow(ctx, `
		SELECT id, failed_attempts, blocked_until
		FROM limits_blocks
		WHERE action_type = $1 AND (
			(identifier <> '' AND identifier = $2) OR
			(ip <> '' AND ip = $3)
		)
		ORDER BY id DESC LIMIT 1
	`, actionType, identifier, ip).Scan(&id, &currentAttempts, &blockedUntil)

	now := time.Now()
	if err == nil {
		currentAttempts++
		if currentAttempts >= maxAttempts {
			blockedUntil = now.Add(blockDuration)
		}
		_, updErr := s.pool.Exec(ctx, `
			UPDATE limits_blocks
			SET failed_attempts = $1, blocked_until = $2, merchant_id = $3, updated_at = now()
			WHERE id = $4
		`, currentAttempts, blockedUntil, merchantID, id)
		return currentAttempts >= maxAttempts, blockedUntil, updErr
	}

	// Insert fresh attempt record
	currentAttempts = 1
	blockedUntil = now
	if maxAttempts <= 1 {
		blockedUntil = now.Add(blockDuration)
	}

	reason := fmt.Sprintf("تكرار المحاولات الفاشلة لـ %s", actionType)
	_, insErr := s.pool.Exec(ctx, `
		INSERT INTO limits_blocks (merchant_id, ip, identifier, action_type, failed_attempts, blocked_until, reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, merchantID, ip, identifier, actionType, currentAttempts, blockedUntil, reason)

	return currentAttempts >= maxAttempts, blockedUntil, insErr
}

// ResetAttempts clears failed attempts on successful action
func (s *Service) ResetAttempts(ctx context.Context, ip, identifier, actionType string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM limits_blocks
		WHERE action_type = $1 AND (
			(identifier <> '' AND identifier = $2) OR
			(ip <> '' AND ip = $3)
		)
	`, actionType, identifier, ip)
	return err
}

func (s *Service) ListBlocks(ctx context.Context) ([]BlockedEntity, error) {
	query := `
		SELECT id, merchant_id, ip, identifier, action_type, failed_attempts, blocked_until, reason, created_at, updated_at
		FROM limits_blocks
		WHERE blocked_until > now()
		ORDER BY blocked_until DESC
		LIMIT 100
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blocks []BlockedEntity
	for rows.Next() {
		var b BlockedEntity
		if err := rows.Scan(&b.ID, &b.MerchantID, &b.IP, &b.Identifier, &b.ActionType, &b.FailedAttempts, &b.BlockedUntil, &b.Reason, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		blocks = append(blocks, b)
	}
	return blocks, rows.Err()
}

func (s *Service) Unblock(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM limits_blocks WHERE id = $1`, id)
	return err
}

// Fiber Middleware to enforce rate limits and ban checks
func (s *Service) Middleware(capacity, refillRate float64) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ip := c.IP()
		var merchantID int64
		if u, ok := c.Locals("user_id").(int64); ok && u > 0 {
			merchantID = u
		}

		// 1. Check permanent/temporary ban
		blocked, retrySec, reason, err := s.IsBlocked(c.Context(), ip, merchantID)
		if err == nil && blocked {
			c.Set("Retry-After", fmt.Sprintf("%d", retrySec))
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"success":     false,
				"error":       "TEMPORARILY_BLOCKED",
				"code":        "TEMPORARILY_BLOCKED",
				"retry_after": retrySec,
				"message":     fmt.Sprintf("تم حظر هذا الحساب/الشبكة مؤقتاً: %s. يرجى الانتظار %d ثانية.", reason, retrySec),
			})
		}

		// 2. Token bucket check
		key := fmt.Sprintf("%s:%d", ip, merchantID)
		allowed, retryAfter := s.Allow(key, capacity, refillRate)
		if !allowed {
			c.Set("Retry-After", fmt.Sprintf("%d", retryAfter))
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"success":     false,
				"error":       "RATE_LIMIT_EXCEEDED",
				"code":        "RATE_LIMIT_EXCEEDED",
				"retry_after": retryAfter,
				"message":     fmt.Sprintf("تم تجاوز حد الطلبات المسموح. يرجى الانتظار %d ثوانٍ.", retryAfter),
			})
		}

		return c.Next()
	}
}
