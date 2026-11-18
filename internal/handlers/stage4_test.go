package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"venus_backend/internal/middleware"
	"venus_backend/internal/tills"
)

func TestStage4Endpoints(t *testing.T) {
	app := fiber.New()

	tillsSvc := tills.NewService(nil)

	h := NewStage4Handler(tillsSvc)

	app.Get("/api/tills/active", func(c *fiber.Ctx) error {
		c.Locals("claims", &middleware.Claims{UserID: 10, Role: "cashier"})
		return h.GetActiveShift(c)
	})

	// Test Tills Active Shift & Cashier Permission Masking
	req := httptest.NewRequest("GET", "/api/tills/active", nil)
	req.Header.Set("X-User-Id", "10")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("tills active error: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200 for tills active, got %d", resp.StatusCode)
	}
	var tillsBody map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&tillsBody); err != nil {
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
