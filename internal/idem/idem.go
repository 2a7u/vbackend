package idem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrKeyInProgress = errors.New("OPERATION_IN_PROGRESS")
)

type StoredResult struct {
	ID         int64           `json:"id"`
	Key        string          `json:"key"`
	MerchantID int64           `json:"merchant_id"`
	Resource   string          `json:"resource"`
	Status     string          `json:"status"`
	StatusCode int             `json:"status_code"`
	Response   json.RawMessage `json:"response"`
	ExpiresAt  time.Time       `json:"expires_at"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// LockKey acquires a transaction-level advisory lock on (merchantID, resource, key)
func LockKey(ctx context.Context, tx pgx.Tx, merchantID int64, resource, key string) error {
	lockStr := fmt.Sprintf("idem:%d:%s:%s", merchantID, resource, key)
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, lockStr)
	return err
}

// CheckOrStart checks if an idempotency key exists.
// If completed, returns (true, StoredResult, nil).
// If pending, returns (false, StoredResult, ErrKeyInProgress).
// If not exists or expired, creates a pending record and returns (false, StoredResult, nil).
func (s *Service) CheckOrStart(ctx context.Context, tx pgx.Tx, merchantID int64, resource, key string, ttl time.Duration) (bool, *StoredResult, error) {
	if ttl < 10*time.Second {
		ttl = 10 * time.Second
	} else if ttl > 24*time.Hour {
		ttl = 24 * time.Hour
	}

	// 1. Acquire transaction advisory lock
	if err := LockKey(ctx, tx, merchantID, resource, key); err != nil {
		return false, nil, fmt.Errorf("advisory lock: %w", err)
	}

	// 2. Query existing
	var res StoredResult
	var respBytes []byte
	err := tx.QueryRow(ctx, `
		SELECT id, key, merchant_id, resource, status, status_code, response, expires_at
		FROM idem_keys
		WHERE merchant_id = $1 AND key = $2
		FOR UPDATE
	`, merchantID, key).Scan(&res.ID, &res.Key, &res.MerchantID, &res.Resource, &res.Status, &res.StatusCode, &respBytes, &res.ExpiresAt)

	now := time.Now()
	if err == nil {
		res.Response = respBytes
		// Key exists
		if res.ExpiresAt.After(now) {
			if res.Status == "completed" {
				return true, &res, nil
			}
			if res.Status == "pending" {
				return false, &res, ErrKeyInProgress
			}
		}
		// Expired or failed -> reuse and set to pending
		expiresAt := now.Add(ttl)
		_, updErr := tx.Exec(ctx, `
			UPDATE idem_keys
			SET status = 'pending', resource = $1, status_code = 200, response = '{}'::jsonb, expires_at = $2, updated_at = now()
			WHERE id = $3
		`, resource, expiresAt, res.ID)
		if updErr != nil {
			return false, nil, fmt.Errorf("renew expired idem key: %w", updErr)
		}
		res.Status = "pending"
		res.ExpiresAt = expiresAt
		return false, &res, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, nil, fmt.Errorf("query idem key: %w", err)
	}

	// Not exists -> insert pending
	expiresAt := now.Add(ttl)
	err = tx.QueryRow(ctx, `
		INSERT INTO idem_keys (key, merchant_id, resource, status, status_code, response, expires_at)
		VALUES ($1, $2, $3, 'pending', 200, '{}'::jsonb, $4)
		RETURNING id
	`, key, merchantID, resource, expiresAt).Scan(&res.ID)
	if err != nil {
		return false, nil, fmt.Errorf("insert pending idem key: %w", err)
	}

	res.Key = key
	res.MerchantID = merchantID
	res.Resource = resource
	res.Status = "pending"
	res.ExpiresAt = expiresAt
	return false, &res, nil
}

// CompleteTx updates the idempotency key record to completed with the final status code and JSON response inside a transaction.
func (s *Service) CompleteTx(ctx context.Context, tx pgx.Tx, merchantID int64, key string, statusCode int, responseData any) error {
	raw, err := json.Marshal(responseData)
	if err != nil {
		raw = []byte("{}")
	}

	_, err = tx.Exec(ctx, `
		UPDATE idem_keys
		SET status = 'completed', status_code = $1, response = $2, updated_at = now()
		WHERE merchant_id = $3 AND key = $4
	`, statusCode, raw, merchantID, key)
	return err
}

// Complete updates outside of caller tx if needed.
func (s *Service) Complete(ctx context.Context, merchantID int64, key string, statusCode int, responseData any) error {
	raw, err := json.Marshal(responseData)
	if err != nil {
		raw = []byte("{}")
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE idem_keys
		SET status = 'completed', status_code = $1, response = $2, updated_at = now()
		WHERE merchant_id = $3 AND key = $4
	`, statusCode, raw, merchantID, key)
	return err
}

// Fail marks the key as failed or deletes it so subsequent requests can re-attempt.
func (s *Service) Fail(ctx context.Context, merchantID int64, key string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM idem_keys WHERE merchant_id = $1 AND key = $2`, merchantID, key)
	return err
}

// CleanKey helper to parse and sanitize header
func CleanKey(raw string) string {
	k := strings.TrimSpace(raw)
	if len(k) > 128 {
		return k[:128]
	}
	return k
}
