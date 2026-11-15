package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrKeyNotFound = errors.New("api key not found or inactive")
	ErrScopeDenied = errors.New("api key lacks required permission scope")
)

type APIKey struct {
	ID                 int64      `json:"id"`
	MerchantID         int64      `json:"merchant_id"`
	Label              string     `json:"label"`
	Prefix             string     `json:"prefix"`
	Scopes             []string   `json:"scopes"`
	RateLimitPerMinute int        `json:"rate_limit_per_minute"`
	UsageCount         int64      `json:"usage_count"`
	LastUsedAt         *time.Time `json:"last_used_at,omitempty"`
	IsActive           bool       `json:"is_active"`
	CreatedAt          time.Time  `json:"created_at"`
}

type GeneratedKeyResult struct {
	Key       APIKey `json:"key"`
	SecretKey string `json:"secret_key"` // Plaintext shown only once!
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func HashKey(plainSecret string) string {
	sum := sha256.Sum256([]byte(plainSecret))
	return hex.EncodeToString(sum[:])
}

// GenerateKey produces a new hashed partner key with scopes.
func (s *Service) GenerateKey(ctx context.Context, merchantID int64, label string, scopes []string, rateLimit int) (*GeneratedKeyResult, error) {
	if len(scopes) == 0 {
		scopes = []string{"read", "sell"}
	}
	if rateLimit <= 0 {
		rateLimit = 60
	}

	randomBytes := make([]byte, 24)
	_, _ = rand.Read(randomBytes)
	secretHex := hex.EncodeToString(randomBytes)
	fullKey := fmt.Sprintf("vsk_live_%s", secretHex)
	prefix := fullKey[:12]
	hashed := HashKey(fullKey)

	if s.pool == nil {
		return &GeneratedKeyResult{
			Key: APIKey{
				MerchantID:         merchantID,
				Label:              label,
				Prefix:             prefix,
				Scopes:             scopes,
				RateLimitPerMinute: rateLimit,
				IsActive:           true,
				CreatedAt:          time.Now().UTC(),
			},
			SecretKey: fullKey,
		}, nil
	}

	query := `
		INSERT INTO api_keys_v2 (merchant_id, label, key_hash, prefix, scopes, rate_limit_per_minute, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, true, now())
		RETURNING id, created_at;
	`
	var id int64
	var createdAt time.Time
	err := s.pool.QueryRow(ctx, query, merchantID, label, hashed, prefix, scopes, rateLimit).Scan(&id, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert api key: %w", err)
	}

	return &GeneratedKeyResult{
		Key: APIKey{
			ID:                 id,
			MerchantID:         merchantID,
			Label:              label,
			Prefix:             prefix,
			Scopes:             scopes,
			RateLimitPerMinute: rateLimit,
			IsActive:           true,
			CreatedAt:          createdAt,
		},
		SecretKey: fullKey,
	}, nil
}

// AuthenticateKey verifies the secret key, increments usage counter, and validates required scope.
func (s *Service) AuthenticateKey(ctx context.Context, plainKey string, requiredScope string) (*APIKey, error) {
	if s.pool == nil {
		return &APIKey{MerchantID: 1, Scopes: []string{"read", "sell"}, IsActive: true}, nil
	}

	hashed := HashKey(plainKey)
	query := `
		SELECT id, merchant_id, label, prefix, scopes, rate_limit_per_minute, usage_count, last_used_at, is_active, created_at
		FROM api_keys_v2
		WHERE key_hash = $1 AND is_active = TRUE;
	`
	var k APIKey
	err := s.pool.QueryRow(ctx, query, hashed).Scan(
		&k.ID, &k.MerchantID, &k.Label, &k.Prefix, &k.Scopes,
		&k.RateLimitPerMinute, &k.UsageCount, &k.LastUsedAt, &k.IsActive, &k.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrKeyNotFound
	} else if err != nil {
		return nil, err
	}

	// Scope verification
	if requiredScope != "" {
		hasScope := false
		for _, sc := range k.Scopes {
			if sc == requiredScope || sc == "admin" || sc == "*" {
				hasScope = true
				break
			}
		}
		if !hasScope {
			return nil, ErrScopeDenied
		}
	}

	// Update usage counter asynchronously
	go func(keyID int64) {
		_, _ = s.pool.Exec(context.Background(), `
			UPDATE api_keys_v2
			SET usage_count = usage_count + 1, last_used_at = now()
			WHERE id = $1;
		`, keyID)
	}(k.ID)

	return &k, nil
}

// ListKeys returns all partner keys for merchant.
func (s *Service) ListKeys(ctx context.Context, merchantID int64) ([]APIKey, error) {
	if s.pool == nil {
		return []APIKey{}, nil
	}
	query := `
		SELECT id, merchant_id, label, prefix, scopes, rate_limit_per_minute, usage_count, last_used_at, is_active, created_at
		FROM api_keys_v2
		WHERE merchant_id = $1 OR merchant_id = 0
		ORDER BY id DESC;
	`
	rows, err := s.pool.Query(ctx, query, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(
			&k.ID, &k.MerchantID, &k.Label, &k.Prefix, &k.Scopes,
			&k.RateLimitPerMinute, &k.UsageCount, &k.LastUsedAt, &k.IsActive, &k.CreatedAt,
		); err == nil {
			list = append(list, k)
		}
	}
	return list, nil
}

// RevokeKey deactivates an API key.
func (s *Service) RevokeKey(ctx context.Context, id int64) error {
	if s.pool == nil {
		return nil
	}
	_, err := s.pool.Exec(ctx, "UPDATE api_keys_v2 SET is_active = FALSE WHERE id = $1;", id)
	return err
}

// GetOpenAPISpec returns the complete OpenAPI 3.0.3 specification map.
func (s *Service) GetOpenAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"openapi": "3.0.3",
		"info": map[string]interface{}{
			"title":       "VENUS / URUK POS Enterprise Partner API",
			"description": "High-concurrency B2B & Fintech API for digital vouchers, wallets, POS operations, and sync.",
			"version":     "2.0.0",
		},
		"servers": []map[string]interface{}{
			{"url": "https://vapi.urukiq.com", "description": "Production Server"},
		},
		"paths": map[string]interface{}{
			"/api/vouchers/dispense": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Atomically dispense encrypted voucher PINs",
					"description": "Claims voucher PINs from vault, evaluates guard cost margin, posts double-entry ledger debit, and decrypts.",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "PINs revealed successfully"},
						"422": map[string]interface{}{"description": "PRICE_BELOW_COST rejection"},
					},
				},
			},
			"/api/sync": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Offline-first delta synchronization",
					"description": "Returns changes since cursor with gzip compression.",
					"parameters": []map[string]interface{}{
						{"name": "since", "in": "query", "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Delta entities and new cursor"},
					},
				},
			},
			"/verify/{token}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Cryptographic seal invoice verification",
					"parameters": []map[string]interface{}{
						{"name": "token", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Verified invoice metadata or rendered HTML"},
						"404": map[string]interface{}{"description": "Invalid seal token or signature"},
					},
				},
			},
		},
	}
}
