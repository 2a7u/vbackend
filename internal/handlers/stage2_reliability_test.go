package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"venus_backend/internal/jobs"
	"venus_backend/internal/middleware"
	"venus_backend/internal/refund"
	syncPkg "venus_backend/internal/sync"
)

func TestStage2SyncDelta(t *testing.T) {
	app := fiber.New()
	syncSvc := syncPkg.NewService(nil)
	syncH := &SyncHandler{SyncSvc: syncSvc}

	app.Get("/api/sync", func(c *fiber.Ctx) error {
		c.Locals("claims", &middleware.Claims{UserID: 1})
		return syncH.GetDelta(c)
	})

	req := httptest.NewRequest("GET", "/api/sync?since=2026-11-16T00:00:00Z", nil)
	req.Header.Set("X-User-Id", "1")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected test error: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var delta syncPkg.SyncDelta
	if err := json.NewDecoder(resp.Body).Decode(&delta); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if delta.Cursor == "" {
		t.Fatal("expected non-empty new cursor in sync delta")
	}
}

func TestStage2JobsDeadQueue(t *testing.T) {
	app := fiber.New()
	jobsSvc := jobs.NewService(nil)
	refundSvc := refund.NewService(nil, nil)
	handler := NewJobsAndRefundHandler(jobsSvc, refundSvc)

	app.Get("/api/admin/jobs/dead", handler.ListDeadJobs)

	req := httptest.NewRequest("GET", "/api/admin/jobs/dead", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected test error: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if _, ok := body["items"]; !ok {
		t.Fatal("expected items field in response")
	}
}

func TestStage2RefundValidation(t *testing.T) {
	app := fiber.New()
	jobsSvc := jobs.NewService(nil)
	refundSvc := refund.NewService(nil, nil)
	handler := NewJobsAndRefundHandler(jobsSvc, refundSvc)

	app.Post("/api/refund", func(c *fiber.Ctx) error {
		c.Locals("claims", &middleware.Claims{UserID: 1})
		return handler.PostRefund(c)
	})

	// Invalid refund amount (<= 0)
	req := httptest.NewRequest("POST", "/api/refund", strings.NewReader(`{"amount": 0, "ref_type": "voucher", "ref_id": "123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "1")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected test error: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid amount, got %d", resp.StatusCode)
	}
}
