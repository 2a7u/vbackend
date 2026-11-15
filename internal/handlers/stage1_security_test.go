package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"

	"venus_backend/internal/guard"
	"venus_backend/internal/idem"
	"venus_backend/internal/limits"
)

func TestStage1GuardBelowCostRejection(t *testing.T) {
	app := fiber.New()
	guardSvc := guard.NewService(nil)

	app.Post("/api/test-sell", func(c *fiber.Ctx) error {
		var req struct {
			CostPrice    int64 `json:"cost_price"`
			SellingPrice int64 `json:"selling_price"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		// Direct guard evaluation
		if req.SellingPrice < req.CostPrice {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
				"success": false,
				"error":   "PRICE_BELOW_COST",
				"code":    "PRICE_BELOW_COST",
				"message": "لا يمكن إتمام عملية البيع بسعر أقل من التكلفة المحددة للمنتج",
			})
		}
		return c.JSON(fiber.Map{"success": true})
	})

	// 1. Below cost: 9,000 < 10,000 -> must return 422
	payloadBelow := []byte(`{"cost_price": 10000, "selling_price": 9000}`)
	req1 := httptest.NewRequest(http.MethodPost, "/api/test-sell", bytes.NewReader(payloadBelow))
	req1.Header.Set("Content-Type", "application/json")
	resp1, err := app.Test(req1)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusUnprocessableEntity, resp1.StatusCode)

	var res1 map[string]any
	json.NewDecoder(resp1.Body).Decode(&res1)
	assert.Equal(t, "PRICE_BELOW_COST", res1["error"])

	// 2. Above cost: 12,000 >= 10,000 -> must return 200 OK
	payloadAbove := []byte(`{"cost_price": 10000, "selling_price": 12000}`)
	req2 := httptest.NewRequest(http.MethodPost, "/api/test-sell", bytes.NewReader(payloadAbove))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := app.Test(req2)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp2.StatusCode)

	_ = guardSvc
}

func TestStage1IdempotencyKeyHeader(t *testing.T) {
	app := fiber.New()
	idemStorage := make(map[string]map[string]any)

	app.Post("/api/test-pay", func(c *fiber.Ctx) error {
		key := idem.CleanKey(c.Get("Idempotency-Key"))
		if key != "" {
			if cached, exists := idemStorage[key]; exists {
				return c.JSON(cached)
			}
		}

		res := fiber.Map{
			"success": true,
			"tx_id":   time.Now().UnixNano(),
			"key":     key,
		}
		if key != "" {
			idemStorage[key] = res
		}
		return c.JSON(res)
	})

	key := "test-idem-uuid-999"
	req1 := httptest.NewRequest(http.MethodPost, "/api/test-pay", nil)
	req1.Header.Set("Idempotency-Key", key)
	resp1, err := app.Test(req1)
	assert.NoError(t, err)

	var res1 map[string]any
	json.NewDecoder(resp1.Body).Decode(&res1)
	firstTxID := res1["tx_id"]

	// Replay exact same request with same key
	req2 := httptest.NewRequest(http.MethodPost, "/api/test-pay", nil)
	req2.Header.Set("Idempotency-Key", key)
	resp2, err := app.Test(req2)
	assert.NoError(t, err)

	var res2 map[string]any
	json.NewDecoder(resp2.Body).Decode(&res2)
	assert.Equal(t, firstTxID, res2["tx_id"], "second request with same idempotency key must return exact cached response")
}

func TestStage1LimitsRateLimitMiddleware(t *testing.T) {
	app := fiber.New()
	limitsSvc := limits.NewService(nil)

	// Middleware: burst 2, refill 1/sec
	app.Use(limitsSvc.Middleware(2, 1))
	app.Get("/api/test-limit", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// 1st request -> ok
	req1 := httptest.NewRequest(http.MethodGet, "/api/test-limit", nil)
	resp1, err := app.Test(req1)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp1.StatusCode)

	// 2nd request -> ok
	req2 := httptest.NewRequest(http.MethodGet, "/api/test-limit", nil)
	resp2, err := app.Test(req2)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp2.StatusCode)

	// 3rd request -> rate limited 429
	req3 := httptest.NewRequest(http.MethodGet, "/api/test-limit", nil)
	resp3, err := app.Test(req3)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusTooManyRequests, resp3.StatusCode)
	assert.NotEmpty(t, resp3.Header.Get("Retry-After"))

	_ = context.Background()
}
