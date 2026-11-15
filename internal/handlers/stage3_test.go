package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"venus_backend/internal/notify"
	"venus_backend/internal/reports"
	"venus_backend/internal/seal"
)

func TestStage3SealAndVerify(t *testing.T) {
	app := fiber.New()
	sealSvc := seal.NewService(nil, "test-secret-key-123", "https://vapi.urukiq.com")
	notifySvc := notify.NewService(nil, nil)
	reportsSvc := reports.NewService(nil, notifySvc)

	h := NewStage3Handler(sealSvc, notifySvc, reportsSvc)

	app.Get("/verify/:token", h.VerifyReceipt)

	// Invalid token
	req := httptest.NewRequest("GET", "/verify/v-invalidtoken", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("expected 404 for invalid token, got %d", resp.StatusCode)
	}
}

func TestStage3ReportsSendNow(t *testing.T) {
	app := fiber.New()
	sealSvc := seal.NewService(nil, "test-secret-key-123", "https://vapi.urukiq.com")
	notifySvc := notify.NewService(nil, nil)
	reportsSvc := reports.NewService(nil, notifySvc)

	h := NewStage3Handler(sealSvc, notifySvc, reportsSvc)

	app.Post("/api/admin/reports/send-now", h.SendReportsNow)

	req := httptest.NewRequest("POST", "/api/admin/reports/send-now", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if body["success"] != true {
		t.Fatal("expected success: true")
	}
}
