package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"venus_backend/internal/middleware"
)

func TestZainCashPublicRoutesNoAuth(t *testing.T) {
	app := fiber.New()
	jwtSecret := "test-secret-12345678901234567890"

	zainH := &ZainCashHandler{}

	api := app.Group("/api")

	// Public routes registered before auth
	api.Get("/zaincash/checkout", zainH.CheckoutZainCash)
	api.Post("/zaincash/checkout/confirm", zainH.ConfirmCheckoutZainCash)
	api.Get("/zaincash/redirect", zainH.RedirectZainCash)
	api.Post("/zaincash/webhook", zainH.WebhookZainCash)

	// Authenticated routes
	auth := api.Group("", middleware.JWT(jwtSecret))
	auth.Post("/payments/zaincash/init", zainH.InitZainCash)
	auth.Get("/payments/zaincash/inquiry/:transactionId", zainH.InquiryZainCash)

	// 1. Test GET /api/zaincash/checkout without Authorization header -> MUST be 200 OK
	req := httptest.NewRequest(http.MethodGet, "/api/zaincash/checkout?tx=test-uuid-123&order=venus1001&amount=25000", nil)
	resp, err := app.Test(req, 2000)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected HTTP 200, got %d: %s", resp.StatusCode, string(body))
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "9647802999569") {
		t.Errorf("checkout HTML missing test MSISDN 9647802999569")
	}
	if !strings.Contains(bodyStr, "1111") {
		t.Errorf("checkout HTML missing test PIN 1111")
	}
	if !strings.Contains(bodyStr, "111111") {
		t.Errorf("checkout HTML missing test OTP 111111")
	}
	if strings.Contains(bodyStr, "missing Authorization header") {
		t.Errorf("checkout HTML improperly returned missing Authorization header error!")
	}

	// 2. Test GET /api/zaincash/redirect?status=failed without Authorization header -> MUST be 200 OK HTML
	reqRedirect := httptest.NewRequest(http.MethodGet, "/api/zaincash/redirect?status=failed", nil)
	respRedirect, err := app.Test(reqRedirect, 2000)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if respRedirect.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respRedirect.Body)
		t.Fatalf("expected HTTP 200, got %d: %s", respRedirect.StatusCode, string(body))
	}

	// 3. Test POST /api/payments/zaincash/init without Authorization header -> MUST be 401 Unauthorized
	reqInit := httptest.NewRequest(http.MethodPost, "/api/payments/zaincash/init", nil)
	respInit, err := app.Test(reqInit, 2000)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if respInit.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(respInit.Body)
		t.Fatalf("expected HTTP 401 for authenticated route, got %d: %s", respInit.StatusCode, string(body))
	}
}
