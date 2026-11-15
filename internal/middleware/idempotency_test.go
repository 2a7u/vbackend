package middleware_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gofiber/fiber/v2"
	"venus_backend/internal/middleware"
	"venus_backend/internal/testhelper"
)

func TestIdempotencyMiddleware(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		return
	}

	app := fiber.New()
	var executionCount atomic.Int64

	app.Post("/api/charge", middleware.Idempotency(pool), func(c *fiber.Ctx) error {
		executionCount.Add(1)
		var body map[string]any
		_ = c.BodyParser(&body)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status": "success",
			"count":  executionCount.Load(),
			"echo":   body,
		})
	})

	t.Run("Replay returns stored response without re-executing handler", func(t *testing.T) {
		key := "test-key-replay-1001"
		body := `{"amount": 50000, "subscriber": "user_42"}`

		// First request
		req1 := httptest.NewRequest("POST", "/api/charge", bytes.NewReader([]byte(body)))
		req1.Header.Set("Content-Type", "application/json")
		req1.Header.Set("Idempotency-Key", key)

		resp1, err := app.Test(req1)
		if err != nil {
			t.Fatalf("first request failed: %v", err)
		}
		if resp1.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp1.StatusCode)
		}
		bodyBytes1, _ := io.ReadAll(resp1.Body)

		// Second request with exact same key & body
		req2 := httptest.NewRequest("POST", "/api/charge", bytes.NewReader([]byte(body)))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Idempotency-Key", key)

		resp2, err := app.Test(req2)
		if err != nil {
			t.Fatalf("second request failed: %v", err)
		}
		if resp2.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp2.StatusCode)
		}
		if resp2.Header.Get("X-Idempotent-Replay") != "true" {
			t.Errorf("expected X-Idempotent-Replay header to be 'true'")
		}

		bodyBytes2, _ := io.ReadAll(resp2.Body)
		if string(bodyBytes1) != string(bodyBytes2) {
			t.Errorf("expected identical response body on replay. got %s vs %s", string(bodyBytes2), string(bodyBytes1))
		}

		// Ensure handler only ran once
		if executionCount.Load() != 1 {
			t.Errorf("handler executed %d times, expected exactly 1", executionCount.Load())
		}
	})

	t.Run("Payload mismatch rejection returns 422 Unprocessable Entity", func(t *testing.T) {
		key := "test-key-mismatch-2002"
		bodyOriginal := `{"amount": 10000}`
		bodyDifferent := `{"amount": 99999}`

		// Initial request
		req1 := httptest.NewRequest("POST", "/api/charge", bytes.NewReader([]byte(bodyOriginal)))
		req1.Header.Set("Content-Type", "application/json")
		req1.Header.Set("Idempotency-Key", key)
		resp1, err := app.Test(req1)
		if err != nil || resp1.StatusCode != http.StatusOK {
			t.Fatalf("initial request failed: %v, status %d", err, resp1.StatusCode)
		}

		// Second request with SAME key but DIFFERENT payload
		req2 := httptest.NewRequest("POST", "/api/charge", bytes.NewReader([]byte(bodyDifferent)))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Idempotency-Key", key)

		resp2, err := app.Test(req2)
		if err != nil {
			t.Fatalf("mismatch request failed: %v", err)
		}
		if resp2.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 Unprocessable Entity on payload mismatch, got %d", resp2.StatusCode)
		}

		var errResp map[string]any
		_ = json.NewDecoder(resp2.Body).Decode(&errResp)
		if errResp["code"] != "IDEMPOTENCY_PAYLOAD_MISMATCH" {
			t.Errorf("expected code IDEMPOTENCY_PAYLOAD_MISMATCH, got %v", errResp["code"])
		}
	})

	t.Run("Duplicate concurrent in-flight request returns 409 Conflict", func(t *testing.T) {
		key := "test-key-inflight-3003"
		reqBody := []byte(`{"amount": 5000}`)
		bodyHash := sha256.Sum256(reqBody)
		hashStr := hex.EncodeToString(bodyHash[:])

		// Pre-insert an in-flight key to simulate a concurrent long-running operation
		ctx := t.Context()
		_, err := pool.Exec(ctx,
			`INSERT INTO idempotency_keys (key, request_path, request_hash, status, created_at)
			 VALUES ($1, '/api/charge', $2, 'in_flight', now())
			 ON CONFLICT (key) DO UPDATE SET status = 'in_flight', request_hash = $2`,
			key, hashStr,
		)
		if err != nil {
			t.Fatalf("pre-insert in_flight key failed: %v", err)
		}

		req := httptest.NewRequest("POST", "/api/charge", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409 Conflict for in-flight request, got %d", resp.StatusCode)
		}
	})
}
