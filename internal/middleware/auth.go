// Package middleware provides JWT authentication and role-based authorization
// for Fiber handlers. The user id is always taken from the verified token,
// never from request input, which is what enforces user_id isolation.
package middleware

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// Claims is the JWT payload carried by every authenticated request.
type Claims struct {
	UserID int64  `json:"uid"`
	Email  string `json:"email"`
	Role   string `json:"role"` // "subscriber" | "admin"
	jwt.RegisteredClaims
}

// SignToken issues a signed JWT valid for the given duration.
func SignToken(secret string, userID int64, email, role string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			Issuer:    "venus_backend",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// JWT verifies the Bearer token and stores the claims in the request context.
func JWT(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		path := c.Path()
		// Public ZainCash callbacks, redirects, payment init, and checkout pages never require JWT
		if strings.HasPrefix(path, "/api/zaincash/") ||
			strings.HasPrefix(path, "/api/auth/") ||
			strings.HasPrefix(path, "/api/payments/zaincash/") {
			return c.Next()
		}

		authHeader := c.Get(fiber.HeaderAuthorization)
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing Authorization header"})
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "expected 'Bearer <token>'"})
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(parts[1], claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fiber.NewError(fiber.StatusUnauthorized, "unexpected signing method")
			}
			return []byte(secret), nil
		})
		if err != nil || !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid or expired token"})
		}

		c.Locals("claims", claims)
		return c.Next()
	}
}

// ParseToken validates a raw JWT string and returns its claims. Used by the
// WebSocket handler where the token arrives as a query parameter.
func ParseToken(secret, tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fiber.NewError(fiber.StatusUnauthorized, "unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "invalid or expired token")
	}
	return claims, nil
}

// ClaimsFrom extracts the verified claims stored by JWT().
func ClaimsFrom(c *fiber.Ctx) *Claims {
	if v := c.Locals("claims"); v != nil {
		if cl, ok := v.(*Claims); ok {
			return cl
		}
	}
	return nil
}

// UserID returns the authenticated user id (0 when unauthenticated).
func UserID(c *fiber.Ctx) int64 {
	if cl := ClaimsFrom(c); cl != nil {
		return cl.UserID
	}
	return 0
}

// AdminOnly rejects any authenticated user whose role is not "admin".
func AdminOnly() fiber.Handler {
	return func(c *fiber.Ctx) error {
		cl := ClaimsFrom(c)
		if cl == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthenticated"})
		}
		if cl.Role != "admin" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "admin role required"})
		}
		return c.Next()
	}
}
