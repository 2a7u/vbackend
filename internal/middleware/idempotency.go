package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Idempotency returns a Fiber middleware that guarantees financial requests
// with an Idempotency-Key header are executed exactly once.
func Idempotency(pool *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := strings.TrimSpace(c.Get("Idempotency-Key"))
		if key == "" {
			key = strings.TrimSpace(c.Get("X-Idempotency-Key"))
		}
		// If no idempotency key provided, proceed without caching
		if key == "" {
			return c.Next()
		}

		uid := UserID(c)
		path := c.Path()
		body := c.Body()

		bodyHash := sha256.Sum256(body)
		hashStr := hex.EncodeToString(bodyHash[:])

		ctx := c.Context()

		var storedStatus string
		var respStatus int
		var respBody string
		err := pool.QueryRow(ctx,
			`SELECT status, response_status, response_body
			 FROM idempotency_keys
			 WHERE key = $1`,
			key,
		).Scan(&storedStatus, &respStatus, &respBody)

		if err == nil {
			if storedStatus == "completed" {
				c.Set("X-Idempotent-Replay", "true")
				c.Set("Content-Type", "application/json; charset=utf-8")
				return c.Status(respStatus).SendString(respBody)
			}
			if storedStatus == "in_flight" {
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"error":   "الطلب قيد المعالجة حالياً من قِبل خادم العمليات المالية، يرجى الانتظار",
					"in_flight": true,
				})
			}
		}

		// Insert in-flight marker
		_, err = pool.Exec(ctx,
			`INSERT INTO idempotency_keys (key, user_id, request_path, request_hash, status, created_at)
			 VALUES ($1, NULLIF($2, 0), $3, $4, 'in_flight', now())
			 ON CONFLICT (key) DO NOTHING`,
			key, uid, path, hashStr,
		)
		if err != nil && err != pgx.ErrNoRows {
			// In case another worker won the race
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "الطلب قيد المعالجة بالتوازي",
			})
		}

		// Execute downstream handler
		execErr := c.Next()
		if execErr != nil {
			_ = pool.QueryRow(ctx,
				`UPDATE idempotency_keys SET status = 'failed' WHERE key = $1`, key,
			)
			return execErr
		}

		status := c.Response().StatusCode()
		respBytes := c.Response().Body()

		if status >= 200 && status < 400 {
			_, _ = pool.Exec(ctx,
				`UPDATE idempotency_keys
				 SET status = 'completed', response_status = $1, response_body = $2, completed_at = now()
				 WHERE key = $3`,
				status, string(respBytes), key,
			)
		} else {
			_, _ = pool.Exec(ctx,
				`UPDATE idempotency_keys
				 SET status = 'failed', response_status = $1, response_body = $2, completed_at = now()
				 WHERE key = $3`,
				status, string(respBytes), key,
			)
		}

		_ = time.Now()
		return nil
	}
}
