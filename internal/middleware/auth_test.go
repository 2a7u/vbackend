package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"venus_backend/internal/middleware"
)

func TestSignTokenAndSignTokenWithMerchant_ClaimsOrder(t *testing.T) {
	const (
		secret     = "test-jwt-secret-32-bytes-minimum!"
		uid        = int64(1)
		email      = "mahmoudraheem76@gmail.com"
		role       = "admin"
		merchantID = "511c2e09-83c8-45eb-b0da-52ed4b43da75"
	)

	tok, err := middleware.SignTokenWithMerchant(secret, uid, email, role, merchantID, time.Hour)
	if err != nil {
		t.Fatalf("SignTokenWithMerchant failed: %v", err)
	}

	claims, err := middleware.ParseToken(secret, tok)
	if err != nil {
		t.Fatalf("ParseToken failed: %v", err)
	}

	if claims.UserID != uid {
		t.Fatalf("expected uid=%d, got %d", uid, claims.UserID)
	}
	if claims.Email != email {
		t.Fatalf("expected email=%q, got %q (claims swapped!)", email, claims.Email)
	}
	if claims.Role != role {
		t.Fatalf("expected role=%q, got %q (claims swapped!)", role, claims.Role)
	}
	if claims.MerchantID != merchantID {
		t.Fatalf("expected mid=%q, got %q (claims swapped!)", merchantID, claims.MerchantID)
	}

	// Also verify SignToken preserves email and role when merchantID is empty.
	tokNoMid, err := middleware.SignToken(secret, uid, email, role, time.Hour)
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}
	claimsNoMid, err := middleware.ParseToken(secret, tokNoMid)
	if err != nil {
		t.Fatalf("ParseToken (no mid) failed: %v", err)
	}
	if claimsNoMid.Email != email || claimsNoMid.Role != role || claimsNoMid.MerchantID != "" {
		t.Fatalf("unexpected SignToken claims: %+v", claimsNoMid)
	}
}

func TestJWTQueryTokenAndCookieAndAdminOnly(t *testing.T) {
	const (
		secret     = "test-jwt-secret-32-bytes-minimum!"
		merchantID = "511c2e09-83c8-45eb-b0da-52ed4b43da75"
	)

	adminJWT, err := middleware.SignTokenWithMerchant(secret, 1, "mahmoudraheem76@gmail.com", "admin", merchantID, time.Hour)
	if err != nil {
		t.Fatalf("sign admin jwt: %v", err)
	}
	subJWT, err := middleware.SignTokenWithMerchant(secret, 42, "shop@venus.iq", "subscriber", merchantID, time.Hour)
	if err != nil {
		t.Fatalf("sign subscriber jwt: %v", err)
	}

	app := fiber.New()
	admin := app.Group("/api/admin", middleware.JWT(secret), middleware.AdminOnly())
	admin.Get("/export/subscribers", func(c *fiber.Ctx) error {
		cl := middleware.ClaimsFrom(c)
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"ok":    true,
			"email": cl.Email,
			"role":  cl.Role,
			"mid":   cl.MerchantID,
		})
	})

	// 1. Valid admin JWT via ?token=... query param -> 200 OK
	reqQuery := httptest.NewRequest(http.MethodGet, "/api/admin/export/subscribers?token="+adminJWT, nil)
	respQuery, err := app.Test(reqQuery, 2000)
	if err != nil {
		t.Fatalf("query token request err: %v", err)
	}
	if respQuery.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for ?token= query param, got %d", respQuery.StatusCode)
	}

	// 2. Valid admin JWT via Authorization Bearer header -> 200 OK
	reqHeader := httptest.NewRequest(http.MethodGet, "/api/admin/export/subscribers", nil)
	reqHeader.Header.Set("Authorization", "Bearer "+adminJWT)
	respHeader, err := app.Test(reqHeader, 2000)
	if err != nil {
		t.Fatalf("bearer header request err: %v", err)
	}
	if respHeader.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for Authorization header, got %d", respHeader.StatusCode)
	}

	// 3. Valid admin JWT via venus_admin_token cookie -> 200 OK
	reqCookie := httptest.NewRequest(http.MethodGet, "/api/admin/export/subscribers", nil)
	reqCookie.AddCookie(&http.Cookie{Name: "venus_admin_token", Value: adminJWT})
	respCookie, err := app.Test(reqCookie, 2000)
	if err != nil {
		t.Fatalf("cookie token request err: %v", err)
	}
	if respCookie.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for venus_admin_token cookie, got %d", respCookie.StatusCode)
	}

	// 4. Subscriber JWT via ?token=... -> 403 Forbidden
	reqSub := httptest.NewRequest(http.MethodGet, "/api/admin/export/subscribers?token="+subJWT, nil)
	respSub, err := app.Test(reqSub, 2000)
	if err != nil {
		t.Fatalf("subscriber token request err: %v", err)
	}
	if respSub.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for subscriber role, got %d", respSub.StatusCode)
	}

	// 5. Missing token -> 401 Unauthorized
	reqMissing := httptest.NewRequest(http.MethodGet, "/api/admin/export/subscribers", nil)
	respMissing, err := app.Test(reqMissing, 2000)
	if err != nil {
		t.Fatalf("missing auth request err: %v", err)
	}
	if respMissing.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized when token missing, got %d", respMissing.StatusCode)
	}

	// 6. Malformed ?token=... -> 401 Unauthorized (even with X-User-Id header)
	reqBad := httptest.NewRequest(http.MethodGet, "/api/admin/export/subscribers?token=invalid.jwt.value", nil)
	reqBad.Header.Set("X-User-Id", "1")
	respBad, err := app.Test(reqBad, 2000)
	if err != nil {
		t.Fatalf("bad token request err: %v", err)
	}
	if respBad.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for invalid ?token=, got %d", respBad.StatusCode)
	}
}
