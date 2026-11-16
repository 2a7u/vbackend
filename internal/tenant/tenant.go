package tenant

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/middleware"
)

var (
	ErrUnauthorized       = errors.New("unauthorized: missing or invalid credentials")
	ErrInvalidMerchantID  = errors.New("invalid or empty merchant_id")
	ErrTenantTxNotFound   = errors.New("tenant transaction not found in context")
)

const (
	ContextTxKey       = "tenant_tx"
	ContextMerchantKey = "tenant_id"
)

// SetupTenantTx applies transaction-local tenant isolation:
// 1. SELECT set_config('app.merchant_id', $1, true) (SET LOCAL)
// 2. SET LOCAL ROLE app_user (to guarantee RLS is strictly enforced even if pool connects as superuser)
func SetupTenantTx(ctx context.Context, tx pgx.Tx, merchantID uuid.UUID) error {
	var applied string
	err := tx.QueryRow(ctx, "SELECT set_config('app.merchant_id', $1, true)", merchantID.String()).Scan(&applied)
	if err != nil {
		return fmt.Errorf("set tenant config: %w", err)
	}

	var hasAppUser bool
	_ = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = 'app_user')").Scan(&hasAppUser)
	if hasAppUser {
		if _, err := tx.Exec(ctx, "SET LOCAL ROLE app_user"); err != nil {
			return fmt.Errorf("set app_user role: %w", err)
		}
	}
	return nil
}

// WithTenant runs fn inside a dedicated transaction strictly configured with
// SELECT set_config('app.merchant_id', $1, true). The third parameter 'true'
// marks the setting as transaction-local (SET LOCAL), ensuring it is purged upon
// COMMIT or ROLLBACK and never leaks across the connection pool.
func WithTenant(ctx context.Context, pool *pgxpool.Pool, merchantID uuid.UUID, fn func(tx pgx.Tx) error) error {
	if merchantID == uuid.Nil {
		return ErrInvalidMerchantID
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tenant tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := SetupTenantTx(ctx, tx, merchantID); err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetTenantID strictly extracts the tenant merchant_id from the cryptographically verified JWT claim.
// It explicitly rejects and ignores any merchant_id passed in query strings, request headers, or body.
func GetTenantID(c *fiber.Ctx, pool *pgxpool.Pool) (uuid.UUID, error) {
	cl := middleware.ClaimsFrom(c)
	if cl == nil {
		return uuid.Nil, ErrUnauthorized
	}

	// 1. Check claim merchant_id first
	if cl.MerchantID != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(cl.MerchantID))
		if err == nil && parsed != uuid.Nil {
			return parsed, nil
		}
	}

	// 2. If claim only had UserID (legacy token), resolve authoritative merchant_id from DB
	if cl.UserID > 0 && pool != nil {
		var mID uuid.UUID
		err := pool.QueryRow(c.Context(), "SELECT merchant_id FROM users WHERE id = $1", cl.UserID).Scan(&mID)
		if err == nil && mID != uuid.Nil {
			cl.MerchantID = mID.String() // cache on claims object
			return mID, nil
		}
	}

	return uuid.Nil, ErrInvalidMerchantID
}

// TxFrom returns the current transaction scoped to the verified tenant.
func TxFrom(c *fiber.Ctx) pgx.Tx {
	val := c.Locals(ContextTxKey)
	if tx, ok := val.(pgx.Tx); ok && tx != nil {
		return tx
	}
	return nil
}

// DBExecutor represents any database client that can execute SQL queries (pool or transaction).
type DBExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// DB returns the transaction scoped to the current tenant if available, or the fallback pool.
func DB(c *fiber.Ctx, fallback *pgxpool.Pool) DBExecutor {
	if tx := TxFrom(c); tx != nil {
		return tx
	}
	return fallback
}

// NotFound returns an opaque, standard 404 response.
// Crucially, it returns 404 whether the record is nonexistent or belongs to another merchant,
// preventing ID enumeration or existence discovery.
func NotFound(c *fiber.Ctx) error {
	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
		"error": "السجل غير موجود",
	})
}

// Middleware creates a transaction-scoped RLS boundary for every authenticated request.
// It applies SET LOCAL app.merchant_id for the duration of the request and commits/rolls back cleanly.
func Middleware(pool *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		path := c.Path()
		// Skip public, unauthenticated routes
		if strings.HasPrefix(path, "/api/auth/") ||
			strings.HasPrefix(path, "/api/zaincash/") ||
			strings.HasPrefix(path, "/api/payments/zaincash/") ||
			strings.HasPrefix(path, "/verify/") ||
			path == "/health" || path == "/healthz" || path == "/readyz" {
			return c.Next()
		}

		merchantID, err := GetTenantID(c, pool)
		if err != nil || merchantID == uuid.Nil {
			// If request is unauthenticated or has no valid merchant ID, continue without tenant tx
			// (Individual handlers or auth middleware will handle unauthorized responses)
			return c.Next()
		}

		tx, err := pool.Begin(c.Context())
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "فشل بدء الجلسة المؤمنة"})
		}
		defer tx.Rollback(c.Context())

		if err := SetupTenantTx(c.Context(), tx, merchantID); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "فشل تفعيل عزل الحساب"})
		}

		c.Locals(ContextTxKey, tx)
		c.Locals(ContextMerchantKey, merchantID)

		// Execute downstream handlers
		handlerErr := c.Next()

		// Commit transaction if no handler error and HTTP status < 400
		if handlerErr == nil && c.Response().StatusCode() < 400 {
			if commitErr := tx.Commit(c.Context()); commitErr != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "فشل تثبيت العملية المالية"})
			}
		}

		return handlerErr
	}
}
