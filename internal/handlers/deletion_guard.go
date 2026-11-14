// R4: bounded deletion guard + shared audit writer.
//
// Rules enforced for non-admin subscribers:
//   - at most 5 deletions per calendar day (Asia/Baghdad),
//   - only records created within the last 24 hours may be deleted,
//   - every deletion is archived into audit_logs with full details.
//
// Admins bypass both limits and may delete any record at any time.
package handlers

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// maxDeletionsPerDay is the hard daily cap for non-admin subscribers.
	maxDeletionsPerDay = 5
	// deletionWindow is how old a record may be for a non-admin to delete it.
	deletionWindow = 24 * time.Hour

	// arabicDailyLimitError is the exact Arabic message the app surfaces.
	arabicDailyLimitError = "لقد وصلت للحد الأقصى المسموح به للحذف اليومي (5 سجلات)"
	// arabicWindowError blocks deleting records older than 24h (non-admin).
	arabicWindowError = "لا يمكن حذف سجلات أقدم من 24 ساعة إلا من قبل Director"
)

// checkDeletionGuard enforces the R4 bounded-deletion rules.
// Returns (allowed, httpStatus, errorMessage). Admins always pass.
func checkDeletionGuard(ctx context.Context, pool *pgxpool.Pool, uid int64, isAdmin bool, createdAt time.Time) (bool, int, string) {
	if isAdmin {
		return true, 0, ""
	}
	if createdAt.IsZero() || time.Since(createdAt) > deletionWindow {
		return false, fiber.StatusForbidden, arabicWindowError
	}
	n, err := countDeletionsToday(ctx, pool, uid)
	if err != nil {
		return false, fiber.StatusInternalServerError, "database error"
	}
	if n >= maxDeletionsPerDay {
		return false, fiber.StatusTooManyRequests, arabicDailyLimitError
	}
	return true, 0, ""
}

// countDeletionsToday counts DELETE_* audit rows for uid since local
// (Asia/Baghdad) calendar midnight.
func countDeletionsToday(ctx context.Context, pool *pgxpool.Pool, uid int64) (int, error) {
	var n int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_logs
		 WHERE user_id = $1
		   AND action LIKE 'DELETE\_%'
		   AND created_at >= (date_trunc('day', now() AT TIME ZONE 'Asia/Baghdad') AT TIME ZONE 'Asia/Baghdad')`,
		uid,
	).Scan(&n)
	return n, err
}

// writeAudit inserts one audit_logs row using the pool (non-transactional).
// It never fails the caller's request.
func writeAudit(ctx context.Context, pool *pgxpool.Pool, uid int64, email, action, entity, entityID string, details map[string]any, ip string) {
	if pool == nil {
		return
	}
	name, store := "", ""
	if uid > 0 {
		_ = pool.QueryRow(ctx, `SELECT full_name, store_name FROM users WHERE id = $1`, uid).Scan(&name, &store)
	}
	_, _ = pool.Exec(ctx,
		`INSERT INTO audit_logs (user_id, user_email, user_name, store_name, action, entity, entity_id, details, ip_address)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9)`,
		nullableInt64(uid), email, name, store, action, entity, entityID, toJSON(details), ip)
}

// recordLoginHistory stores a login attempt (success or failure).
func recordLoginHistory(ctx context.Context, pool *pgxpool.Pool, uid int64, email string, success bool, ip string) {
	if pool == nil {
		return
	}
	_, _ = pool.Exec(ctx,
		`INSERT INTO login_history (user_id, email, success, ip_address) VALUES ($1, $2, $3, $4)`,
		nullableInt64(uid), email, success, ip)
}
