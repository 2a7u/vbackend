package handlers_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"venus_backend/internal/handlers"
	"venus_backend/internal/middleware"
)

func TestPhase2AdminEndpointsRBAC(t *testing.T) {
	app := fiber.New()
	jwtSecret := "test-rbac-secret-32-bytes-long-key!!"

	fintechH := &handlers.FintechHandler{}

	api := app.Group("/api")
	admin := api.Group("/admin", middleware.JWT(jwtSecret), middleware.AdminOnly())

	// Phase 2 Routes
	admin.Get("/pricing/tiers", fintechH.ListPriceTiers)
	admin.Post("/pricing/tiers/:id/prices", fintechH.SetTierPrice)
	admin.Get("/pricing/overrides", fintechH.ListSubscriberPriceOverrides)
	admin.Post("/pricing/subscribers/:id/overrides", fintechH.SetSubscriberPriceOverride)
	admin.Delete("/pricing/subscribers/:id/overrides/:productId", fintechH.RemoveSubscriberPriceOverride)
	admin.Get("/pricing/resolve/:userId/:productId", fintechH.ResolveSubscriberPrice)
	admin.Post("/hierarchy/users/:id/parent", fintechH.SetParentAgent)
	admin.Post("/hierarchy/agents/:id/commissions", fintechH.SetAgentCommission)
	admin.Get("/hierarchy/commissions", fintechH.ListAgentCommissions)
	admin.Get("/hierarchy/agents/:id/report", fintechH.GetAgentCommissionReport)
	admin.Get("/radar/summary", fintechH.GetInventoryRadar)
	admin.Post("/radar/thresholds", fintechH.SetCategoryThreshold)
	admin.Post("/vault/rotate-keys", fintechH.RotateVaultKeys)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/admin/pricing/tiers"},
		{"POST", "/api/admin/pricing/tiers/1/prices"},
		{"GET", "/api/admin/pricing/overrides"},
		{"POST", "/api/admin/pricing/subscribers/10/overrides"},
		{"DELETE", "/api/admin/pricing/subscribers/10/overrides/asia_5000"},
		{"GET", "/api/admin/pricing/resolve/10/asia_5000"},
		{"POST", "/api/admin/hierarchy/users/10/parent"},
		{"POST", "/api/admin/hierarchy/agents/10/commissions"},
		{"GET", "/api/admin/hierarchy/commissions"},
		{"GET", "/api/admin/hierarchy/agents/10/report"},
		{"GET", "/api/admin/radar/summary"},
		{"POST", "/api/admin/radar/thresholds"},
		{"POST", "/api/admin/vault/rotate-keys"},
	}

	subscriberToken, err := middleware.SignToken(jwtSecret, 999, "pos@store.iq", "subscriber", time.Hour)
	if err != nil {
		t.Fatalf("failed to sign subscriber token: %v", err)
	}

	for _, ep := range endpoints {
		t.Run("NoAuth_"+ep.method+"_"+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, bytes.NewBufferString("{}"))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req, 2000)
			if err != nil {
				t.Fatalf("request error: %v", err)
			}
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected 401 Unauthorized without token, got %d for %s %s", resp.StatusCode, ep.method, ep.path)
			}
		})

		t.Run("SubscriberRoleForbidden_"+ep.method+"_"+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, bytes.NewBufferString("{}"))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+subscriberToken)
			resp, err := app.Test(req, 2000)
			if err != nil {
				t.Fatalf("request error: %v", err)
			}
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("expected 403 Forbidden for subscriber role, got %d for %s %s", resp.StatusCode, ep.method, ep.path)
			}
		})
	}
}
