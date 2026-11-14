// Package handlers: admin superpower endpoints — one-time factory reset,
// full audit log, subscriber management (edit/delete), and digital cards.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/middleware"
)

// AdminHandler groups all admin-only endpoints.
type AdminHandler struct {
	Pool *pgxpool.Pool
	Hub  *WSHub
}

// audit writes one row into audit_logs. Never fails the request.
func (h *AdminHandler) audit(c *fiber.Ctx, action, entity, entityID string, details map[string]any) {
	if h == nil || h.Pool == nil {
		return
	}
	cl := middleware.ClaimsFrom(c)
	uid := int64(0)
	email, name, store := "", "", ""
	if cl != nil {
		uid = cl.UserID
		email = cl.Email
	}
	if uid > 0 {
		_ = h.Pool.QueryRow(c.Context(),
			`SELECT email, full_name, store_name FROM users WHERE id = $1`, uid,
		).Scan(&email, &name, &store)
	}
	ip := c.IP()
	_, _ = h.Pool.Exec(c.Context(),
		`INSERT INTO audit_logs (user_id, user_email, user_name, store_name, action, entity, entity_id, details, ip_address)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9)`,
		nullableInt64(uid), email, name, store, action, entity, entityID, toJSON(details), ip)
}

// auditTx is the transactional variant used inside handlers that already hold a tx.
func auditTx(ctx context.Context, tx pgx.Tx, uid int64, email, action, entity, entityID string, details map[string]any, ip string) {
	name, store := "", ""
	_ = tx.QueryRow(ctx, `SELECT full_name, store_name FROM users WHERE id = $1`, uid).Scan(&name, &store)
	_, _ = tx.Exec(ctx,
		`INSERT INTO audit_logs (user_id, user_email, user_name, store_name, action, entity, entity_id, details, ip_address)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9)`,
		nullableInt64(uid), email, name, store, action, entity, entityID, toJSON(details), ip)
}

func nullableInt64(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}

func toJSON(m map[string]any) string {
	if m == nil {
		return "{}"
	}
	// minimal JSON encoding without pulling encoding/json into hot path twice
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// R1: One-time factory reset (runs ONCE per deploy via system_meta marker)
// ---------------------------------------------------------------------------

// RunOneTimeResets executes any pending one-time migrations recorded in
// system_meta. Currently: reset_v2_clean truncates all transactional tables.
func (h *AdminHandler) RunOneTimeResets(ctx context.Context) error {
	var exists bool
	err := h.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM system_meta WHERE key = 'reset_v2_clean')`).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return nil // already ran once — never wipe again on restart
	}
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	stmts := []string{
		`TRUNCATE sim_sales, credit_sales, sim_inventory, debt_payments, debt_contracts, store_expenses, support_tickets, subscriber_backups, wallet_ledger, subscriptions, audit_logs, login_history RESTART IDENTITY CASCADE`,
		// Keep users but reset every account except the super admin to a clean pending state.
		`UPDATE users SET is_approved = FALSE, role = 'subscriber', wallet_balance = 0 WHERE email <> $1`,
		`INSERT INTO system_meta (key) VALUES ('reset_v2_clean') ON CONFLICT (key) DO NOTHING`,
	}
	for i, s := range stmts {
		if i == 1 {
			if _, err := tx.Exec(ctx, s, superAdminEmail()); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec(ctx, s); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// FactoryReset is the admin endpoint that triggers the one-time clean reset.
// POST /api/admin/factory-reset
func (h *AdminHandler) FactoryReset(c *fiber.Ctx) error {
	if err := h.RunOneTimeResets(c.Context()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "factory reset failed: " + err.Error()})
	}
	h.audit(c, "FACTORY_RESET", "system", "reset_v2_clean", map[string]any{"at": time.Now()})
	return c.JSON(fiber.Map{"success": true, "reset": "reset_v2_clean"})
}

// ---------------------------------------------------------------------------
// R4: Audit log listing (admin) — real-time oversight of all actions/deletions
// ---------------------------------------------------------------------------

// ListAuditLogs (admin) returns the latest audit entries.
// GET /api/admin/audit-logs?limit=200
func (h *AdminHandler) ListAuditLogs(c *fiber.Ctx) error {
	limit := 200
	if v := c.QueryInt("limit", 200); v > 0 && v <= 1000 {
		limit = v
	}
	rows, err := h.Pool.Query(c.Context(),
		`SELECT id, user_id, user_email, user_name, store_name, action, entity, entity_id, details, ip_address, created_at
		 FROM audit_logs ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer rows.Close()
	out := make([]fiber.Map, 0)
	for rows.Next() {
		var (
			id        int64
			uid       *int64
			email     string
			name      string
			store     string
			action    string
			entity    string
			entityID  string
			details   []byte
			ip        string
			createdAt time.Time
		)
		if err := rows.Scan(&id, &uid, &email, &name, &store, &action, &entity, &entityID, &details, &ip, &createdAt); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "scan error"})
		}
		out = append(out, fiber.Map{
			"id": id, "user_id": uid, "user_email": email, "user_name": name,
			"store_name": store, "action": action, "entity": entity, "entity_id": entityID,
			"details": string(details), "ip_address": ip, "created_at": createdAt,
		})
	}
	return c.JSON(out)
}

// ListLoginHistory (admin) returns login history for a user or everyone.
// GET /api/admin/login-history?user_id=123
func (h *AdminHandler) ListLoginHistory(c *fiber.Ctx) error {
	query := `SELECT id, user_id, email, success, ip_address, created_at FROM login_history ORDER BY created_at DESC LIMIT 500`
	args := []any{}
	if uid := c.QueryInt("user_id", 0); uid > 0 {
		query = `SELECT id, user_id, email, success, ip_address, created_at FROM login_history WHERE user_id = $1 ORDER BY created_at DESC LIMIT 500`
		args = append(args, uid)
	}
	rows, err := h.Pool.Query(c.Context(), query, args...)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer rows.Close()
	out := make([]fiber.Map, 0)
	for rows.Next() {
		var (
			id        int64
			uid       int64
			email     string
			success   bool
			ip        string
			createdAt time.Time
		)
		if err := rows.Scan(&id, &uid, &email, &success, &ip, &createdAt); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "scan error"})
		}
		out = append(out, fiber.Map{
			"id": id, "user_id": uid, "email": email, "success": success,
			"ip_address": ip, "created_at": createdAt,
		})
	}
	return c.JSON(out)
}

// ---------------------------------------------------------------------------
// R5A: Subscriber management — full edit (incl. avatar/address) + delete
// ---------------------------------------------------------------------------

type adminUpdateUserRequest struct {
	Email     *string `json:"email"`
	Password  *string `json:"password"`
	FullName  *string `json:"full_name"`
	StoreName *string `json:"store_name"`
	Phone     *string `json:"phone"`
	Address   *string `json:"address"`
	AvatarURL *string `json:"avatar_url"`
	Role      *string `json:"role"`
	Approved  *bool   `json:"is_approved"`
}

// AdminUpdateUser (admin) edits any subscriber field.
// PUT /api/admin/users/:id
func (h *AdminHandler) AdminUpdateUser(c *fiber.Ctx) error {
	id, err := paramID(c)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}
	var req adminUpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}

	tx, err := h.Pool.Begin(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer tx.Rollback(c.Context())

	if req.Email != nil {
		e := strings.ToLower(strings.TrimSpace(*req.Email))
		if !validEmail(e) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid email"})
		}
		var other int64
		err = tx.QueryRow(c.Context(), `SELECT id FROM users WHERE email = $1 AND id <> $2`, e, id).Scan(&other)
		if err == nil {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "email already in use"})
		}
		if err != pgx.ErrNoRows {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
		}
		if _, err := tx.Exec(c.Context(), `UPDATE users SET email = $1, updated_at = now() WHERE id = $2`, e, id); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not update email"})
		}
	}
	if req.Password != nil && *req.Password != "" {
		if !validPassword(*req.Password) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "password must be at least 6 characters"})
		}
		hash, err := hashPassword(*req.Password)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not hash password"})
		}
		if _, err := tx.Exec(c.Context(), `UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`, hash, id); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not update password"})
		}
	}
	if req.FullName != nil {
		if _, err := tx.Exec(c.Context(), `UPDATE users SET full_name = $1, updated_at = now() WHERE id = $2`, strings.TrimSpace(*req.FullName), id); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not update full_name"})
		}
	}
	if req.StoreName != nil {
		if _, err := tx.Exec(c.Context(), `UPDATE users SET store_name = $1, updated_at = now() WHERE id = $2`, strings.TrimSpace(*req.StoreName), id); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not update store_name"})
		}
	}
	if req.Phone != nil {
		if _, err := tx.Exec(c.Context(), `UPDATE users SET phone = $1, updated_at = now() WHERE id = $2`, strings.TrimSpace(*req.Phone), id); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not update phone"})
		}
	}
	if req.Address != nil {
		if _, err := tx.Exec(c.Context(), `UPDATE users SET address = $1, updated_at = now() WHERE id = $2`, strings.TrimSpace(*req.Address), id); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not update address"})
		}
	}
	if req.AvatarURL != nil {
		if _, err := tx.Exec(c.Context(), `UPDATE users SET avatar_url = $1, updated_at = now() WHERE id = $2`, strings.TrimSpace(*req.AvatarURL), id); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not update avatar_url"})
		}
	}
	if req.Role != nil {
		r := strings.TrimSpace(*req.Role)
		if r != "subscriber" && r != "admin" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "role must be subscriber or admin"})
		}
		if _, err := tx.Exec(c.Context(), `UPDATE users SET role = $1, updated_at = now() WHERE id = $2`, r, id); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not update role"})
		}
	}
	if req.Approved != nil {
		if _, err := tx.Exec(c.Context(), `UPDATE users SET is_approved = $1, updated_at = now() WHERE id = $2`, *req.Approved, id); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not update is_approved"})
		}
	}

	if err := tx.Commit(c.Context()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "commit failed"})
	}
	h.audit(c, "UPDATE_USER", "user", itoa(id), map[string]any{"fields": req})
	return c.JSON(fiber.Map{"success": true})
}

// AdminDeleteUser (admin) deletes a subscriber with ON DELETE CASCADE.
// DELETE /api/admin/users/:id
func (h *AdminHandler) AdminDeleteUser(c *fiber.Ctx) error {
	id, err := paramID(c)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}
	cl := middleware.ClaimsFrom(c)
	if cl != nil && cl.UserID == id {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cannot delete your own admin account"})
	}
	var email, name string
	_ = h.Pool.QueryRow(c.Context(), `SELECT email, full_name FROM users WHERE id = $1`, id).Scan(&email, &name)
	res, err := h.Pool.Exec(c.Context(), `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if res.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}
	h.audit(c, "DELETE_USER", "user", itoa(id), map[string]any{"email": email, "full_name": name, "deleted_at": time.Now()})
	return c.JSON(fiber.Map{"success": true})
}

// ---------------------------------------------------------------------------
// R5A: Digital cards management (admin CRUD + public list)
// ---------------------------------------------------------------------------

// ListDigitalCards (public to authenticated users) returns active cards.
// GET /api/digital-cards
func (h *AdminHandler) ListDigitalCards(c *fiber.Ctx) error {
	// Admins see every card (including inactive); subscribers only active ones.
	query := `SELECT id, category_id, title, category, image_url, buy_price, sell_price, active, created_at, updated_at
		  FROM digital_cards`
	if cl := middleware.ClaimsFrom(c); cl == nil || cl.Role != "admin" {
		query += ` WHERE active = TRUE`
	}
	query += ` ORDER BY category, title`
	rows, err := h.Pool.Query(c.Context(), query)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer rows.Close()
	out := make([]fiber.Map, 0)
	for rows.Next() {
		var (
			id         int64
			categoryID *int64
			title      string
			category   string
			imageURL   string
			buyPrice   float64
			sellPrice  float64
			active     bool
			createdAt  time.Time
			updatedAt  time.Time
		)
		if err := rows.Scan(&id, &categoryID, &title, &category, &imageURL, &buyPrice, &sellPrice, &active, &createdAt, &updatedAt); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "scan error"})
		}
		out = append(out, fiber.Map{
			"id": id, "category_id": categoryID, "title": title, "category": category,
			"image_url": imageURL, "buy_price": buyPrice, "sell_price": sellPrice,
			"active": active, "created_at": createdAt, "updated_at": updatedAt,
		})
	}
	return c.JSON(out)
}

type digitalCardRequest struct {
	CategoryID *int64  `json:"category_id"`
	Title      string  `json:"title"`
	Category   string  `json:"category"`
	ImageURL   string  `json:"image_url"`
	BuyPrice   float64 `json:"buy_price"`
	SellPrice  float64 `json:"sell_price"`
	Active     *bool   `json:"active"`
}

// AdminSaveDigitalCard (admin) creates or updates a digital card.
// POST /api/admin/digital-cards  |  PUT /api/admin/digital-cards/:id
func (h *AdminHandler) AdminSaveDigitalCard(c *fiber.Ctx) error {
	var req digitalCardRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" || req.BuyPrice < 0 || req.SellPrice < 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "title, buy_price and sell_price are required"})
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}

	idParam := strings.TrimSpace(c.Params("id"))
	if idParam == "" {
		var id int64
		err := h.Pool.QueryRow(c.Context(),
			`INSERT INTO digital_cards (category_id, title, category, image_url, buy_price, sell_price, active)
			 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			req.CategoryID, req.Title, strings.TrimSpace(req.Category), strings.TrimSpace(req.ImageURL),
			req.BuyPrice, req.SellPrice, active,
		).Scan(&id)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
		}
		h.audit(c, "CREATE_CARD", "digital_card", itoa(id), map[string]any{"title": req.Title})
		if h.Hub != nil {
			h.Hub.Broadcast(0, "digital_cards", fiber.Map{"action": "created", "id": id})
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": id, "success": true})
	}

	id, err := paramID(c)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid card id"})
	}
	res, err := h.Pool.Exec(c.Context(),
		`UPDATE digital_cards SET category_id = $1, title = $2, category = $3, image_url = $4,
		 buy_price = $5, sell_price = $6, active = $7, updated_at = now() WHERE id = $8`,
		req.CategoryID, req.Title, strings.TrimSpace(req.Category), strings.TrimSpace(req.ImageURL),
		req.BuyPrice, req.SellPrice, active, id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if res.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "card not found"})
	}
	h.audit(c, "UPDATE_CARD", "digital_card", itoa(id), map[string]any{"title": req.Title})
	if h.Hub != nil {
		h.Hub.Broadcast(0, "digital_cards", fiber.Map{"action": "updated", "id": id})
	}
	return c.JSON(fiber.Map{"success": true})
}

// AdminDeleteDigitalCard (admin) removes a card.
// DELETE /api/admin/digital-cards/:id
func (h *AdminHandler) AdminDeleteDigitalCard(c *fiber.Ctx) error {
	id, err := paramID(c)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid card id"})
	}
	res, err := h.Pool.Exec(c.Context(), `DELETE FROM digital_cards WHERE id = $1`, id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if res.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "card not found"})
	}
	h.audit(c, "DELETE_CARD", "digital_card", itoa(id), map[string]any{"deleted_at": time.Now()})
	if h.Hub != nil {
		h.Hub.Broadcast(0, "digital_cards", fiber.Map{"action": "deleted", "id": id})
	}
	return c.JSON(fiber.Map{"success": true})
}

// AdminAdjustWallet atomically updates a subscriber's wallet balance.
// POST /api/admin/users/:id/wallet or POST /api/admin/wallet/credit
func (h *AdminHandler) AdminAdjustWallet(c *fiber.Ctx) error {
	id, err := paramID(c)
	if err != nil || id <= 0 {
		// Try reading user_id or target_user_id from body
		var b struct {
			UserID       int64   `json:"user_id"`
			TargetUserID string  `json:"target_user_id"`
			Amount       float64 `json:"amount"`
			Note         string  `json:"note"`
		}
		_ = c.BodyParser(&b)
		if b.UserID > 0 {
			id = b.UserID
		} else if b.TargetUserID != "" {
			if parsed, pErr := strconv.ParseInt(b.TargetUserID, 10, 64); pErr == nil {
				id = parsed
			}
		}
	}
	if id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "valid user id required"})
	}

	var req struct {
		Amount float64 `json:"amount"`
		Note   string  `json:"note"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	if req.Amount == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "amount must not be zero"})
	}

	note := strings.TrimSpace(req.Note)
	if note == "" {
		if req.Amount > 0 {
			note = "شحن رصيد يدوي من الإدارة"
		} else {
			note = "خصم رصيد يدوي من الإدارة"
		}
	}

	tx, err := h.Pool.Begin(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer tx.Rollback(c.Context())

	var newBalance float64
	err = tx.QueryRow(c.Context(),
		`UPDATE users
		 SET wallet_balance = COALESCE(wallet_balance, 0) + $1,
		     updated_at = now()
		 WHERE id = $2
		 RETURNING wallet_balance`, req.Amount, id,
	).Scan(&newBalance)
	if err == pgx.ErrNoRows {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not update wallet balance"})
	}

	kind := "admin_credit"
	if req.Amount < 0 {
		kind = "admin_debit"
	}
	_, _ = tx.Exec(c.Context(),
		`INSERT INTO wallet_ledger (user_id, amount, kind, provider, status, note, completed_at)
		 VALUES ($1, $2, $3, 'admin', 'success', $4, now())`,
		id, req.Amount, kind, note)

	if err := tx.Commit(c.Context()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "commit failed"})
	}

	h.audit(c, "WALLET_ADJUST", "user", itoa(id), map[string]any{
		"amount": req.Amount, "new_balance": newBalance, "note": note,
	})

	if h.Hub != nil {
		h.Hub.Broadcast(id, "wallet_updated", fiber.Map{
			"user_id": id, "wallet_balance": newBalance, "amount": req.Amount, "note": note,
		})
	}

	return c.JSON(fiber.Map{
		"success":        true,
		"user_id":        id,
		"wallet_balance": newBalance,
		"amount":         req.Amount,
		"message":        "تم تعديل الرصيد بنجاح",
	})
}

// AdminCreateUser allows Super Admin to add another Admin or Subscriber directly.
// POST /api/admin/users
func (h *AdminHandler) AdminCreateUser(c *fiber.Ctx) error {
	var req struct {
		Email     string `json:"email"`
		Password  string `json:"password"`
		FullName  string `json:"full_name"`
		StoreName string `json:"store_name"`
		Phone     string `json:"phone"`
		Role      string `json:"role"` // "admin" or "subscriber"
		Plan      string `json:"plan"` // "مشترك تجريبي", "سوبر اقتصادي", "بلص", "أعمال"
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !validEmail(req.Email) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid email"})
	}
	if !validPassword(req.Password) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "password must be at least 6 characters"})
	}

	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role != "admin" {
		role = "subscriber"
	}

	planName := strings.TrimSpace(req.Plan)
	var intervalStr string
	if role == "admin" {
		planName = "vip"
		intervalStr = "3650 days"
	} else {
		switch planName {
		case "أعمال":
			intervalStr = "365 days"
		case "بلص":
			intervalStr = "180 days"
		case "سوبر اقتصادي":
			intervalStr = "30 days"
		default:
			planName = "مشترك تجريبي"
			intervalStr = "2 days"
		}
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not hash password"})
	}

	var (
		id                 int64
		subscriptionEndsAt time.Time
	)
	err = h.Pool.QueryRow(c.Context(),
		`INSERT INTO users (email, password_hash, full_name, store_name, phone, role, is_approved, status, plan_name, subscription_ends_at)
		 VALUES ($1, $2, $3, $4, $5, $6, TRUE, 'active', $7, now() + ($8 || ' days')::interval)
		 ON CONFLICT (email) DO NOTHING
		 RETURNING id, subscription_ends_at`,
		req.Email, hash, strings.TrimSpace(req.FullName), strings.TrimSpace(req.StoreName),
		strings.TrimSpace(req.Phone), role, planName, strings.TrimSuffix(intervalStr, " days"),
	).Scan(&id, &subscriptionEndsAt)
	if err == pgx.ErrNoRows {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "email already registered"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error: " + err.Error()})
	}

	h.audit(c, "CREATE_USER", "user", itoa(id), map[string]any{
		"email": req.Email, "role": role, "plan": planName, "full_name": req.FullName,
	})

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"id":      id,
		"user": fiber.Map{
			"id":                   id,
			"email":                req.Email,
			"full_name":            req.FullName,
			"store_name":           req.StoreName,
			"phone":                req.Phone,
			"role":                 role,
			"is_approved":          true,
			"status":               "active",
			"plan_name":            planName,
			"subscription_ends_at": subscriptionEndsAt,
		},
	})
}

// superAdminEmail reads SUPER_ADMIN_EMAIL (default Mahmoudraheem76@gmail.com).
func superAdminEmail() string {
	if v := os.Getenv("SUPER_ADMIN_EMAIL"); v != "" {
		return strings.ToLower(strings.TrimSpace(v))
	}
	return "mahmoudraheem76@gmail.com"
}

// GenerateActivationCodes creates new pre-paid subscriber activation codes.
// POST /api/admin/activation-codes/generate
func (h *AdminHandler) GenerateActivationCodes(c *fiber.Ctx) error {
	var req struct {
		PlanName     string  `json:"plan_name"`
		DurationDays int     `json:"duration_days"`
		PriceIQD     float64 `json:"price_iqd"`
		Count        int     `json:"count"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
	}
	if req.PlanName == "" {
		req.PlanName = "سوبر اقتصادي"
	}
	if req.DurationDays <= 0 {
		req.DurationDays = 30
	}
	if req.Count <= 0 {
		req.Count = 1
	}
	if req.Count > 50 {
		req.Count = 50
	}

	codes := make([]fiber.Map, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		code := fmt.Sprintf("VENUS-%dD-%s", req.DurationDays, strings.ToUpper(uuid.New().String()[:8]))
		var id int64
		err := h.Pool.QueryRow(c.Context(),
			`INSERT INTO activation_codes (code, plan_name, duration_days, price_iqd, is_used, created_at)
			 VALUES ($1, $2, $3, $4, FALSE, now())
			 RETURNING id`, code, req.PlanName, req.DurationDays, req.PriceIQD,
		).Scan(&id)
		if err == nil {
			codes = append(codes, fiber.Map{
				"id":            id,
				"code":          code,
				"plan_name":     req.PlanName,
				"duration_days": req.DurationDays,
				"price_iqd":     req.PriceIQD,
				"is_used":       false,
				"created_at":    time.Now(),
			})
		}
	}

	return c.JSON(fiber.Map{
		"success": true,
		"codes":   codes,
	})
}

// ListActivationCodes lists all generated activation codes and their usage status.
// GET /api/admin/activation-codes
func (h *AdminHandler) ListActivationCodes(c *fiber.Ctx) error {
	rows, err := h.Pool.Query(c.Context(),
		`SELECT c.id, c.code, c.plan_name, c.duration_days, c.price_iqd, c.is_used,
		        c.used_at, COALESCE(u.email, ''), c.created_at
		 FROM activation_codes c
		 LEFT JOIN users u ON u.id = c.used_by_user_id
		 ORDER BY c.created_at DESC LIMIT 200`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer rows.Close()

	list := make([]fiber.Map, 0)
	for rows.Next() {
		var (
			id           int64
			code         string
			planName     string
			durationDays int
			price        float64
			isUsed       bool
			usedAt       *time.Time
			usedByEmail  string
			createdAt    time.Time
		)
		if err := rows.Scan(&id, &code, &planName, &durationDays, &price, &isUsed, &usedAt, &usedByEmail, &createdAt); err == nil {
			list = append(list, fiber.Map{
				"id":            id,
				"code":          code,
				"plan_name":     planName,
				"duration_days": durationDays,
				"price_iqd":     price,
				"is_used":       isUsed,
				"used_at":       usedAt,
				"used_by_email": usedByEmail,
				"created_at":    createdAt,
			})
		}
	}
	return c.JSON(list)
}

// itoa is a tiny int64 -> string helper to avoid importing strconv everywhere.
func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
