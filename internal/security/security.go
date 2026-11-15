package security

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/ledger"
)

type Service struct {
	Pool      *pgxpool.Pool
	LedgerSvc *ledger.Service

	mu          sync.RWMutex
	rateLimits  map[string]*tokenBucket
	failedLogin map[string]int
}

type tokenBucket struct {
	tokens     float64
	capacity   float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

func NewService(pool *pgxpool.Pool, ledgerSvc *ledger.Service) *Service {
	s := &Service{
		Pool:        pool,
		LedgerSvc:   ledgerSvc,
		rateLimits:  make(map[string]*tokenBucket),
		failedLogin: make(map[string]int),
	}
	// Background cleaner for rate limits
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s.mu.Lock()
			s.rateLimits = make(map[string]*tokenBucket)
			s.mu.Unlock()
		}
	}()
	return s
}

// ExtractClientIP retrieves the genuine client IP address.
func ExtractClientIP(c *fiber.Ctx) string {
	xff := c.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}
	xrip := strings.TrimSpace(c.Get("X-Real-IP"))
	if xrip != "" {
		return xrip
	}
	return c.IP()
}

// RateLimiter returns a token-bucket rate limiter middleware.
func (s *Service) RateLimiter(capacity float64, refillRate float64) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ip := ExtractClientIP(c)

		// 1. Check IP Rules (blacklist & auto-ban)
		if s.Pool != nil {
			var isBlocked bool
			var banExpires *time.Time
			_ = s.Pool.QueryRow(c.Context(),
				`SELECT true, ban_expires_at FROM ip_rules
				 WHERE ip_address = $1 AND (rule_type = 'blacklist' OR (rule_type = 'auto_ban' AND (ban_expires_at IS NULL OR ban_expires_at > now())))`,
				ip,
			).Scan(&isBlocked, &banExpires)
			if isBlocked {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"error": "تم حظر عنوان بروتوكول الإنترنت (IP) الخاص بك لدواعي أمنية",
					"code":  "IP_BLOCKED",
				})
			}
		}

		s.mu.Lock()
		tb, exists := s.rateLimits[ip]
		now := time.Now()
		if !exists {
			tb = &tokenBucket{
				tokens:     capacity,
				capacity:   capacity,
				refillRate: refillRate,
				lastRefill: now,
			}
			s.rateLimits[ip] = tb
		}

		// Refill tokens
		elapsed := now.Sub(tb.lastRefill).Seconds()
		tb.tokens = tb.tokens + elapsed*tb.refillRate
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}
		tb.lastRefill = now

		if tb.tokens < 1.0 {
			s.mu.Unlock()
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "تجاوزت الحد المسموح به من الطلبات، يرجى المحاولة بعد قليل",
				"code":  "RATE_LIMIT_EXCEEDED",
			})
		}

		tb.tokens -= 1.0
		s.mu.Unlock()

		return c.Next()
	}
}

// RecordFailedLogin increments failed attempts and triggers escalating auto-ban upon exceeding threshold.
func (s *Service) RecordFailedLogin(ctx context.Context, ip string) {
	s.mu.Lock()
	s.failedLogin[ip]++
	count := s.failedLogin[ip]
	s.mu.Unlock()

	if count >= 5 && s.Pool != nil {
		banDuration := 15 * time.Minute
		if count >= 10 {
			banDuration = 24 * time.Hour
		}
		expiresAt := time.Now().UTC().Add(banDuration)
		_, _ = s.Pool.Exec(ctx,
			`INSERT INTO ip_rules (ip_address, rule_type, reason, ban_expires_at)
			 VALUES ($1, 'auto_ban', 'تكرار محاولات تسجيل الدخول الفاشلة', $2)
			 ON CONFLICT (ip_address) DO UPDATE
			 SET ban_expires_at = $2, reason = 'تكرار محاولات تسجيل الدخول الفاشلة'`,
			ip, expiresAt,
		)
	}
}

// ResetFailedLogin resets the counter upon successful login.
func (s *Service) ResetFailedLogin(ip string) {
	s.mu.Lock()
	delete(s.failedLogin, ip)
	s.mu.Unlock()
}

// Forensic Audit Trail Logger
func (s *Service) LogAudit(ctx context.Context, actorID int64, actorEmail, action, entity, entityID string, before, after map[string]any, ip, ua string) {
	if s.Pool == nil {
		return
	}
	details := map[string]any{
		"before":     before,
		"after":      after,
		"user_agent": ua,
	}
	detailsJSON, _ := json.Marshal(details)

	_, _ = s.Pool.Exec(ctx,
		`INSERT INTO audit_logs (actor_id, actor_email, action, entity, entity_id, details, ip_address, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, now())`,
		actorID, actorEmail, action, entity, entityID, detailsJSON, ip,
	)
}

// RollbackEngine reverses an auditable operation
func (s *Service) RollbackOperation(ctx context.Context, auditID int64, adminID int64) error {
	var (
		action   string
		entity   string
		entityID string
		details  map[string]any
	)
	err := s.Pool.QueryRow(ctx,
		`SELECT action, entity, entity_id, details FROM audit_logs WHERE id = $1`,
		auditID,
	).Scan(&action, &entity, &entityID, &details)
	if err != nil {
		return fmt.Errorf("read audit record: %w", err)
	}

	if strings.HasPrefix(action, "rollback_") {
		return errors.New("لا يمكن إجراء تراجع عن عملية تراجع سابقة")
	}

	// Financial operations must be reversed through the double-entry ledger service
	if entity == "ledger" || strings.Contains(action, "wallet") || strings.Contains(action, "adjustment") {
		if txIDStr, ok := details["tx_id"].(string); ok && txIDStr != "" && s.LedgerSvc != nil {
			txUUID, pErr := uuid.Parse(txIDStr)
			if pErr == nil {
				tx, bErr := s.Pool.Begin(ctx)
				if bErr != nil {
					return bErr
				}
				defer tx.Rollback(ctx)

				_, revErr := s.LedgerSvc.ExecuteReversal(ctx, tx, txUUID, fmt.Sprintf("تراجع إداري بواسطة المشرف #%d", adminID), adminID)
				if revErr != nil {
					return fmt.Errorf("reversal failed: %w", revErr)
				}
				if err := tx.Commit(ctx); err != nil {
					return err
				}

				s.LogAudit(ctx, adminID, "admin", "rollback_"+action, entity, entityID, details, map[string]any{"reversed_tx_id": txIDStr}, "", "")
				return nil
			}
		}
	}

	return errors.New("العملية المحددة غير قابلة للعكس تلقائياً")
}

// Webhook Dispatcher with HMAC Signature and Exponential Backoff
func (s *Service) DispatchWebhook(ctx context.Context, eventName string, payload any) {
	if s.Pool == nil {
		return
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT id, name, url, secret_token FROM webhooks WHERE is_active = true AND $1 = ANY(events)`,
		eventName,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	type whTarget struct {
		id     int64
		name   string
		url    string
		secret string
	}
	var targets []whTarget
	for rows.Next() {
		var t whTarget
		if err := rows.Scan(&t.id, &t.name, &t.url, &t.secret); err == nil {
			targets = append(targets, t)
		}
	}
	rows.Close()

	payloadBytes, _ := json.Marshal(map[string]any{
		"event":     eventName,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      payload,
	})

	for _, t := range targets {
		go s.sendWebhookWithRetry(t.id, t.url, t.secret, eventName, payloadBytes)
	}
}

func (s *Service) sendWebhookWithRetry(webhookID int64, targetURL, secret, eventName string, payload []byte) {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	signature := hex.EncodeToString(mac.Sum(nil))

	client := &http.Client{Timeout: 10 * time.Second}
	maxRetries := 3
	backoff := 1 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("POST", targetURL, bytes.NewReader(payload))
		if err != nil {
			break
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Venus-Signature", signature)
		req.Header.Set("X-Venus-Event", eventName)

		resp, err := client.Do(req)
		var status int
		var bodyStr string
		if err == nil {
			status = resp.StatusCode
			resp.Body.Close()
		}

		if status >= 200 && status < 300 {
			// Delivery success
			if s.Pool != nil {
				_, _ = s.Pool.Exec(context.Background(),
					`INSERT INTO webhook_deliveries (webhook_id, event_name, payload, response_status, attempts, status)
					 VALUES ($1, $2, $3, $4, $5, 'delivered')`,
					webhookID, eventName, payload, status, attempt,
				)
			}
			return
		}

		// Log retry and sleep
		if attempt < maxRetries {
			time.Sleep(backoff)
			backoff *= 2
		} else {
			// Final failure
			if s.Pool != nil {
				_, _ = s.Pool.Exec(context.Background(),
					`INSERT INTO webhook_deliveries (webhook_id, event_name, payload, response_status, response_body, attempts, status)
					 VALUES ($1, $2, $3, $4, $5, $6, 'failed')`,
					webhookID, eventName, payload, status, bodyStr, attempt,
				)
			}
		}
	}
}

// GenerateAPIKey generates a cryptographically secure external API key.
// Returns the plaintext key (displayed ONLY once to the admin) and the stored record.
func (s *Service) GenerateAPIKey(ctx context.Context, keyName, role string, scopes []string) (string, error) {
	rawBytes := make([]byte, 24)
	if _, err := rand.Read(rawBytes); err != nil {
		return "", err
	}
	rawHex := hex.EncodeToString(rawBytes)
	plaintextKey := fmt.Sprintf("vsk_live_%s", rawHex)
	prefix := plaintextKey[:13] // "vsk_live_1234"

	// Hash plaintext key with SHA-256
	h := sha256.Sum256([]byte(plaintextKey))
	keyHash := hex.EncodeToString(h[:])

	_, err := s.Pool.Exec(ctx,
		`INSERT INTO api_keys (key_name, key_prefix, key_hash, role, scopes, is_active, created_at)
		 VALUES ($1, $2, $3, $4, $5, true, now())`,
		keyName, prefix, keyHash, role, scopes,
	)
	if err != nil {
		return "", fmt.Errorf("store api key: %w", err)
	}

	return plaintextKey, nil
}
