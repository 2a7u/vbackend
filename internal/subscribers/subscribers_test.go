package subscribers_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"venus_backend/internal/middleware"
)

func TestImpersonationTokenReadOnlyScope(t *testing.T) {
	secret := "test_enterprise_secret_key_12345"
	targetUID := int64(42)
	adminUID := int64(1)
	email := "subscriber42@uruk.pos"
	role := "subscriber"

	tokenStr, err := middleware.SignImpersonationToken(secret, targetUID, email, role, adminUID, 1*time.Hour)
	if err != nil {
		t.Fatalf("sign impersonation token: %v", err)
	}

	claims := &middleware.Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("parse impersonation token failed: %v", err)
	}

	if claims.UserID != targetUID {
		t.Errorf("expected UserID %d, got %d", targetUID, claims.UserID)
	}
	if claims.Scope != "read_only" {
		t.Errorf("expected scope 'read_only', got %q", claims.Scope)
	}
	if claims.ImpersonatorID != adminUID {
		t.Errorf("expected ImpersonatorID %d, got %d", adminUID, claims.ImpersonatorID)
	}
}
