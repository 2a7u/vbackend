package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"venus_backend/internal/db/migrations"
	"venus_backend/internal/handlers"
	"venus_backend/internal/limits"
	"venus_backend/internal/middleware"
	"venus_backend/internal/testhelper"
)

func TestLoginJWTClaimsAndQueryTokenExportSubscribers(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		return
	}

	const jwtSecret = "test-auth-export-secret-32-bytes!!"
	const adminEmail = "mahmoudraheem76@gmail.com"
	const adminPassword = "AdminPassword123!"

	ctx := context.Background()
	_ = migrations.EnsureMigrationsTable(ctx, pool)
	if _, err := migrations.ApplyPending(ctx, pool); err != nil {
		t.Fatalf("apply pending migrations: %v", err)
	}
	pwHash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password failed: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO users (email, password_hash, full_name, store_name, role, is_approved, status, plan_name, subscription_ends_at)
		 VALUES ($1, $2, 'Super Admin', 'VENUS POS', 'admin', TRUE, 'active', 'vip', now() + interval '3650 days')
		 ON CONFLICT (email) DO UPDATE
		 SET password_hash = EXCLUDED.password_hash,
		     role = 'admin',
		     is_approved = TRUE,
		     status = 'active'`,
		adminEmail, string(pwHash),
	)
	if err != nil {
		t.Fatalf("seed admin user failed: %v", err)
	}

	authH := &handlers.AuthHandler{
		Pool:            pool,
		JWTSecret:       jwtSecret,
		TokenTTL:        24 * time.Hour,
		SuperAdminEmail: adminEmail,
		LimitsSvc:       limits.NewService(pool),
	}
	enterpriseH := handlers.NewEnterpriseHandler(pool, nil, nil, nil, nil, nil, nil, nil, nil)

	app := fiber.New()
	api := app.Group("/api")
	api.Post("/auth/login", authH.Login)

	admin := api.Group("/admin", middleware.JWT(jwtSecret), middleware.AdminOnly())
	admin.Get("/export/subscribers", enterpriseH.ExportSubscribersCSV)

	// 1. Login as SuperAdmin (re-seeds with role='admin' and merchant_id UUID)
	loginBody, _ := json.Marshal(map[string]string{
		"email":    adminEmail,
		"password": adminPassword,
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := app.Test(loginReq, 5000)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	if loginResp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(loginResp.Body)
		t.Fatalf("expected 200 OK from /api/auth/login, got %d: %s", loginResp.StatusCode, string(raw))
	}

	var loginData struct {
		Token string                 `json:"token"`
		User  map[string]interface{} `json:"user"`
	}
	if err := json.NewDecoder(loginResp.Body).Decode(&loginData); err != nil {
		t.Fatalf("decode login response failed: %v", err)
	}
	if loginData.Token == "" {
		t.Fatal("expected non-empty JWT token from /api/auth/login")
	}

	// 2. Verify claims inside the issued JWT are NOT swapped
	claims, err := middleware.ParseToken(jwtSecret, loginData.Token)
	if err != nil {
		t.Fatalf("ParseToken failed on issued token: %v", err)
	}
	if claims.UserID <= 0 {
		t.Fatalf("expected positive uid, got %d", claims.UserID)
	}
	if claims.Email != adminEmail {
		t.Fatalf("expected JWT claim email=%q, got %q", adminEmail, claims.Email)
	}
	if claims.Role != "admin" {
		t.Fatalf("expected JWT claim role=%q, got %q", "admin", claims.Role)
	}
	if _, err := uuid.Parse(claims.MerchantID); err != nil {
		t.Fatalf("expected JWT claim mid to be a valid UUID, got %q: %v", claims.MerchantID, err)
	}

	// 3. Call GET /api/admin/export/subscribers?token=<valid_admin_jwt> WITHOUT Authorization header -> MUST be 200 OK
	exportReq := httptest.NewRequest(http.MethodGet, "/api/admin/export/subscribers?token="+loginData.Token, nil)
	exportResp, err := app.Test(exportReq, 5000)
	if err != nil {
		t.Fatalf("export request failed: %v", err)
	}
	if exportResp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(exportResp.Body)
		t.Fatalf("expected 200 OK for GET /api/admin/export/subscribers?token=..., got %d: %s", exportResp.StatusCode, string(raw))
	}
	ct := exportResp.Header.Get(fiber.HeaderContentType)
	if !strings.Contains(ct, "text/csv") {
		t.Fatalf("expected text/csv content type, got %q", ct)
	}
}
