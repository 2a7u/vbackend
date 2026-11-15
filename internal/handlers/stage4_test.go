package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	apiPkg "venus_backend/internal/api"
	"venus_backend/internal/forecast"
	"venus_backend/internal/middleware"
	"venus_backend/internal/restock"
	"venus_backend/internal/rewards"
	"venus_backend/internal/risk"
	"venus_backend/internal/surge"
	"venus_backend/internal/tills"
)

func TestStage4Endpoints(t *testing.T) {
	app := fiber.New()

	rewardsSvc := rewards.NewService(nil, nil)
	restockSvc := restock.NewService(nil, nil)
	riskSvc := risk.NewService(nil)
	forecastSvc := forecast.NewService(nil)
	tillsSvc := tills.NewService(nil)
	apiSvc := apiPkg.NewService(nil)
	surgeSvc := surge.NewService(nil)

	h := NewStage4Handler(rewardsSvc, restockSvc, riskSvc, forecastSvc, tillsSvc, apiSvc, surgeSvc)

	app.Get("/forecast", h.GetForecast)
	app.Get("/api/surge/quote", h.GetSurgeQuote)
	app.Get("/api/docs/openapi.json", h.GetOpenAPISpec)
	app.Get("/api/tills/active", func(c *fiber.Ctx) error {
		c.Locals("claims", &middleware.Claims{UserID: 10, Role: "cashier"})
		return h.GetActiveShift(c)
	})

	// 1. Test /forecast
	req1 := httptest.NewRequest("GET", "/forecast", nil)
	resp1, err := app.Test(req1)
	if err != nil {
		t.Fatalf("forecast error: %v", err)
	}
	if resp1.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200 for forecast, got %d", resp1.StatusCode)
	}
	var f forecast.LiquidityForecast
	if err := json.NewDecoder(resp1.Body).Decode(&f); err != nil {
		t.Fatalf("decode forecast err: %v", err)
	}
	if len(f.ForecastNextWeek) != 7 {
		t.Fatalf("expected 7 projected days, got %d", len(f.ForecastNextWeek))
	}

	// 2. Test /api/surge/quote
	req2 := httptest.NewRequest("GET", "/api/surge/quote?base_price=10000", nil)
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("surge quote error: %v", err)
	}
	if resp2.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200 for surge quote, got %d", resp2.StatusCode)
	}

	// 3. Test OpenAPI Doc
	req3 := httptest.NewRequest("GET", "/api/docs/openapi.json", nil)
	resp3, err := app.Test(req3)
	if err != nil {
		t.Fatalf("openapi doc error: %v", err)
	}
	if resp3.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200 for openapi doc, got %d", resp3.StatusCode)
	}

	// 4. Test Tills Active Shift & Cashier Permission Masking
	req4 := httptest.NewRequest("GET", "/api/tills/active", nil)
	req4.Header.Set("X-User-Id", "10")
	resp4, err := app.Test(req4)
	if err != nil {
		t.Fatalf("tills active error: %v", err)
	}
	if resp4.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200 for tills active, got %d", resp4.StatusCode)
	}
	var tillsBody map[string]interface{}
	if err := json.NewDecoder(resp4.Body).Decode(&tillsBody); err != nil {
		t.Fatalf("decode tills body err: %v", err)
	}
	perms, ok := tillsBody["permissions"].(map[string]interface{})
	if !ok {
		t.Fatal("expected permissions map")
	}
	if perms["can_view_costs"] != false || perms["can_view_profits"] != false {
		t.Fatal("cashier permissions MUST mask costs and profits (can_view_costs/profits = false)")
	}
}
