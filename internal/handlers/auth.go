// Package handlers implements all HTTP endpoints of the VENUS POS backend.
package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"venus_backend/internal/middleware"
)

// AuthHandler groups the authentication and credential endpoints.
type AuthHandler struct {
	Pool            *pgxpool.Pool
	JWTSecret       string
	TokenTTL        time.Duration
	SuperAdminEmail string
}

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type registerRequest struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	FullName  string `json:"full_name"`
	StoreName string `json:"store_name"`
	Phone     string `json:"phone"`
}

func validEmail(email string) bool {
	e := strings.TrimSpace(email)
	return len(e) >= 3 && strings.Contains(e, "@")
}

func validPassword(pw string) bool {
	return len(pw) >= 6
}

func hashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Login verifies email+password and returns a signed JWT.
// POST /api/auth/login
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !validEmail(req.Email) || !validPassword(req.Password) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid email or password"})
	}

	var (
		id                 int64
		email              string
		passwordHash       string
		fullName           string
		storeName          string
		phone              string
		role               string
		isApproved         bool
		walletBalance      float64
		status             string
		planName           string
		subscriptionEndsAt *time.Time
	)
	err := h.Pool.QueryRow(c.Context(),
		`SELECT id, email, password_hash, COALESCE(full_name, ''), COALESCE(store_name, ''), COALESCE(phone, ''),
		        role, is_approved, COALESCE(wallet_balance, 0), COALESCE(status, 'active'),
		        COALESCE(plan_name, 'مشترك تجريبي'), subscription_ends_at
		 FROM users WHERE email = $1`, req.Email,
	).Scan(&id, &email, &passwordHash, &fullName, &storeName, &phone, &role, &isApproved, &walletBalance, &status, &planName, &subscriptionEndsAt)
	if err == pgx.ErrNoRows {
		// R1: re-seed the Super Admin if the clean reset removed it.
		if h.SuperAdminEmail != "" && req.Email == strings.ToLower(strings.TrimSpace(h.SuperAdminEmail)) {
			if hash, herr := hashPassword(req.Password); herr == nil {
				var newID int64
				if ierr := h.Pool.QueryRow(c.Context(),
					`INSERT INTO users (email, password_hash, full_name, store_name, role, is_approved, status, plan_name, subscription_ends_at)
					 VALUES ($1, $2, 'Super Admin', 'VENUS POS', 'admin', TRUE, 'active', 'vip', now() + interval '3650 days')
					 ON CONFLICT (email) DO UPDATE SET role = 'admin', is_approved = TRUE, status = 'active', plan_name = 'vip'
					 RETURNING id`, req.Email, hash,
				).Scan(&newID); ierr == nil {
					id, email, role, isApproved, status, planName = newID, req.Email, "admin", true, "active", "vip"
					fullName, storeName = "Super Admin", "VENUS POS"
					passwordHash = hash
					recordLoginHistory(c.Context(), h.Pool, id, email, true, c.IP())
					writeAudit(c.Context(), h.Pool, id, email, "LOGIN", "auth", itoa(id), map[string]any{"note": "super admin re-seeded", "at": time.Now()}, c.IP())
					token, terr := middleware.SignToken(h.JWTSecret, id, email, role, h.TokenTTL)
					if terr != nil {
						return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not sign token"})
					}
					return c.JSON(fiber.Map{
						"token": token,
						"user": fiber.Map{
							"id":                   id,
							"uid":                  fmt.Sprintf("V%06d", id),
							"email":                email,
							"name":                 fullName,
							"full_name":            fullName,
							"store_name":           storeName,
							"phone":                phone,
							"role":                 role,
							"is_approved":          isApproved,
							"status":               status,
							"plan_name":            planName,
							"subscription_plan":    planName,
							"wallet_balance":       0,
							"subscription_ends_at": time.Now().AddDate(10, 0, 0),
						},
					})
				}
			}
		}
		recordLoginHistory(c.Context(), h.Pool, 0, req.Email, false, c.IP())
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid credentials"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)) != nil {
		recordLoginHistory(c.Context(), h.Pool, id, email, false, c.IP())
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid credentials"})
	}
	if role == "subscriber" && !isApproved {
		recordLoginHistory(c.Context(), h.Pool, id, email, false, c.IP())
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "account pending admin approval"})
	}

	token, err := middleware.SignToken(h.JWTSecret, id, email, role, h.TokenTTL)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not sign token"})
	}
	recordLoginHistory(c.Context(), h.Pool, id, email, true, c.IP())
	writeAudit(c.Context(), h.Pool, id, email, "LOGIN", "auth", itoa(id), map[string]any{"role": role, "at": time.Now()}, c.IP())
	userMap := fiber.Map{
		"id":                      id,
		"uid":                     fmt.Sprintf("V%06d", id),
		"email":                   email,
		"name":                    fullName,
		"full_name":               fullName,
		"store_name":              storeName,
		"phone":                   phone,
		"role":                    role,
		"is_approved":             isApproved,
		"status":                  status,
		"plan_name":               planName,
		"subscription_plan":       planName,
		"wallet_balance":          walletBalance,
		"subscription_ends_at":    subscriptionEndsAt,
		"subscription_expires_at": subscriptionEndsAt,
	}
	return c.JSON(fiber.Map{
		"token": token,
		"user":  userMap,
	})
}

// Register creates a new subscriber account with an automatic 2-day trial.
// POST /api/auth/register
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req registerRequest
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

	hash, err := hashPassword(req.Password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not hash password"})
	}

	role := "subscriber"
	planName := "مشترك تجريبي"
	approved := true
	status := "active"
	trialInterval := "2 days"
	if h.SuperAdminEmail != "" && req.Email == strings.ToLower(strings.TrimSpace(h.SuperAdminEmail)) {
		role = "admin"
		planName = "vip"
		trialInterval = "3650 days"
	}

	var (
		id                 int64
		subscriptionEndsAt time.Time
	)
	err = h.Pool.QueryRow(c.Context(),
		`INSERT INTO users (email, password_hash, full_name, store_name, phone, role, is_approved, status, plan_name, subscription_ends_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now() + interval '`+trialInterval+`')
		 ON CONFLICT (email) DO NOTHING
		 RETURNING id, subscription_ends_at`,
		req.Email, hash, strings.TrimSpace(req.FullName), strings.TrimSpace(req.StoreName),
		strings.TrimSpace(req.Phone), role, approved, status, planName,
	).Scan(&id, &subscriptionEndsAt)
	if err == pgx.ErrNoRows {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "email already registered"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}

	token, _ := middleware.SignToken(h.JWTSecret, id, req.Email, role, h.TokenTTL)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"token": token,
		"user": fiber.Map{
			"id":                     id,
			"email":                  req.Email,
			"full_name":              strings.TrimSpace(req.FullName),
			"store_name":             strings.TrimSpace(req.StoreName),
			"phone":                  strings.TrimSpace(req.Phone),
			"role":                   role,
			"is_approved":            approved,
			"status":                 status,
			"plan_name":              planName,
			"subscription_plan":      planName,
			"wallet_balance":         0,
			"subscription_ends_at":   subscriptionEndsAt,
			"subscription_expires_at": subscriptionEndsAt,
		},
		"id":                      id,
		"email":                   req.Email,
		"full_name":               strings.TrimSpace(req.FullName),
		"store_name":              strings.TrimSpace(req.StoreName),
		"phone":                   strings.TrimSpace(req.Phone),
		"role":                    role,
		"is_approved":             approved,
		"status":                  status,
		"plan_name":               planName,
		"subscription_plan":       planName,
		"subscription_ends_at":    subscriptionEndsAt,
		"subscription_expires_at": subscriptionEndsAt,
	})
}

// Me returns the authenticated user's full profile (used to restore sessions).
// GET /api/auth/me
func (h *AuthHandler) Me(c *fiber.Ctx) error {
	cl := middleware.ClaimsFrom(c)
	if cl == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthenticated"})
	}
	var (
		id                 int64
		email              string
		fullName           string
		storeName          string
		phone              string
		role               string
		approved           bool
		createdAt          time.Time
		avatarURL          string
		address            string
		walletBalance      float64
		status             string
		planName           string
		subscriptionEndsAt *time.Time
	)
	err := h.Pool.QueryRow(c.Context(),
		`SELECT id, email, full_name, store_name, phone, role, is_approved, created_at,
		        COALESCE(avatar_url, ''), COALESCE(address, ''), COALESCE(wallet_balance, 0),
		        COALESCE(status, 'active'), COALESCE(plan_name, 'مشترك تجريبي'), subscription_ends_at
		 FROM users WHERE id = $1`, cl.UserID,
	).Scan(&id, &email, &fullName, &storeName, &phone, &role, &approved, &createdAt,
		&avatarURL, &address, &walletBalance, &status, &planName, &subscriptionEndsAt)
	if err == pgx.ErrNoRows {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	resp := fiber.Map{
		"id":             id,
		"email":          email,
		"full_name":      fullName,
		"store_name":     storeName,
		"phone":          phone,
		"role":           role,
		"is_approved":    approved,
		"created_at":     createdAt,
		"avatar_url":     avatarURL,
		"address":        address,
		"wallet_balance": walletBalance,
		"status":         status,
		"plan_name":      planName,
		"subscription_plan": planName,
	}
	if subscriptionEndsAt != nil {
		resp["subscription_ends_at"] = subscriptionEndsAt
		resp["subscription_expires_at"] = subscriptionEndsAt
	}
	return c.JSON(resp)
}

// ChangeCredentials lets an authenticated subscriber change their own email
// and/or password. POST body: {"email": "...", "password": "..."} (either may
// be omitted; password must be re-hashed with bcrypt).
// PUT /api/auth/credentials
func (h *AuthHandler) ChangeCredentials(c *fiber.Ctx) error {
	cl := middleware.ClaimsFrom(c)
	if cl == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthenticated"})
	}
	var req credentialsRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if req.Email == "" && req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "nothing to update"})
	}
	if req.Email != "" && !validEmail(req.Email) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid email"})
	}
	if req.Password != "" && !validPassword(req.Password) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "password must be at least 6 characters"})
	}

	tx, err := h.Pool.Begin(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer tx.Rollback(c.Context())

	if req.Email != "" {
		// Ensure the new email is not taken by another account.
		var other int64
		err = tx.QueryRow(c.Context(), `SELECT id FROM users WHERE email = $1 AND id <> $2`, req.Email, cl.UserID).Scan(&other)
		if err == nil {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "email already in use"})
		}
		if err != pgx.ErrNoRows {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
		}
		if _, err = tx.Exec(c.Context(), `UPDATE users SET email = $1, updated_at = now() WHERE id = $2`, req.Email, cl.UserID); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not update email"})
		}
	}
	if req.Password != "" {
		hash, err := hashPassword(req.Password)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not hash password"})
		}
		if _, err = tx.Exec(c.Context(), `UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`, hash, cl.UserID); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not update password"})
		}
	}

	if err := tx.Commit(c.Context()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "commit failed"})
	}
	writeAudit(c.Context(), h.Pool, cl.UserID, cl.Email, "UPDATE_PROFILE", "users", itoa(cl.UserID), map[string]any{
		"fields": map[string]bool{"email": req.Email != "", "password": req.Password != ""}, "at": time.Now(),
	}, c.IP())
	return c.JSON(fiber.Map{"success": true})
}

// AdminChangeUserCredentials lets an admin override any subscriber's email
// and/or password. PUT /api/admin/users/:id/credentials
func (h *AuthHandler) AdminChangeUserCredentials(c *fiber.Ctx) error {
	id, err := paramID(c)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}
	var req credentialsRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" && req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "nothing to update"})
	}
	if req.Email != "" && !validEmail(req.Email) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid email"})
	}
	if req.Password != "" && !validPassword(req.Password) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "password must be at least 6 characters"})
	}

	tx, err := h.Pool.Begin(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer tx.Rollback(c.Context())

	if req.Email != "" {
		var other int64
		err = tx.QueryRow(c.Context(), `SELECT id FROM users WHERE email = $1 AND id <> $2`, req.Email, id).Scan(&other)
		if err == nil {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "email already in use"})
		}
		if err != pgx.ErrNoRows {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
		}
		if _, err = tx.Exec(c.Context(), `UPDATE users SET email = $1, updated_at = now() WHERE id = $2`, req.Email, id); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not update email"})
		}
	}
	if req.Password != "" {
		hash, err := hashPassword(req.Password)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not hash password"})
		}
		if _, err = tx.Exec(c.Context(), `UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`, hash, id); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not update password"})
		}
	}

	if err := tx.Commit(c.Context()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "commit failed"})
	}
	return c.JSON(fiber.Map{"success": true})
}

// ListUsers (admin) returns all accounts for the admin console.
// GET /api/admin/users
func (h *AuthHandler) ListUsers(c *fiber.Ctx) error {
	rows, err := h.Pool.Query(c.Context(),
		`SELECT id, email, full_name, store_name, phone, role, is_approved, created_at, avatar_url, address, wallet_balance,
		        COALESCE(status, 'active'), COALESCE(plan_name, 'مشترك تجريبي'), subscription_ends_at
		 FROM users ORDER BY created_at DESC`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer rows.Close()

	users := make([]fiber.Map, 0)
	for rows.Next() {
		var (
			id        int64
			email     string
			fullName  string
			storeName string
			phone     string
			role      string
			approved  bool
			createdAt time.Time
			avatarURL string
			address   string
			walletBal float64
			status    string
			planName  string
			endsAt    *time.Time
		)
		if err := rows.Scan(&id, &email, &fullName, &storeName, &phone, &role, &approved, &createdAt, &avatarURL, &address, &walletBal, &status, &planName, &endsAt); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "scan error: " + err.Error()})
		}
		users = append(users, fiber.Map{
			"id": id, "email": email, "full_name": fullName, "store_name": storeName,
			"phone": phone, "role": role, "is_approved": approved, "created_at": createdAt,
			"avatar_url": avatarURL, "address": address, "wallet_balance": walletBal,
			"status": status, "plan_name": planName, "subscription_ends_at": endsAt,
		})
	}
	return c.JSON(users)
}

// ApproveUser (admin) flips is_approved for a subscriber.
// PUT /api/admin/users/:id/approve
func (h *AuthHandler) ApproveUser(c *fiber.Ctx) error {
	id, err := paramID(c)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}
	if _, err := h.Pool.Exec(c.Context(),
		`UPDATE users SET is_approved = TRUE, updated_at = now() WHERE id = $1`, id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	return c.JSON(fiber.Map{"success": true})
}

// RedeemActivationCode validates and redeems a prepaid activation voucher key.
// POST /api/subscriptions/redeem-code
func (h *AuthHandler) RedeemActivationCode(c *fiber.Ctx) error {
	var req struct {
		Code string `json:"code"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
	}
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "يرجى إدخال رمز التفعيل"})
	}

	uid := middleware.UserID(c)
	if uid <= 0 {
		uid = 1
	}

	tx, err := h.Pool.Begin(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer tx.Rollback(c.Context())

	var (
		codeID       int64
		planName     string
		durationDays int
		isUsed       bool
	)
	err = tx.QueryRow(c.Context(),
		`SELECT id, plan_name, duration_days, is_used 
		 FROM activation_codes 
		 WHERE UPPER(code) = $1 
		 FOR UPDATE`, code,
	).Scan(&codeID, &planName, &durationDays, &isUsed)
	if err == pgx.ErrNoRows || isUsed {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "رمز التفعيل غير صالح أو تم استخدامه مسبقاً"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}

	// Mark code used
	_, err = tx.Exec(c.Context(),
		`UPDATE activation_codes 
		 SET is_used = TRUE, used_by_user_id = $1, used_at = now() 
		 WHERE id = $2`, uid, codeID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update code"})
	}

	// Activate user plan and extend subscription
	var newEndsAt time.Time
	err = tx.QueryRow(c.Context(),
		`UPDATE users 
		 SET plan_name = $1, 
		     status = 'active',
		     is_approved = TRUE,
		     subscription_ends_at = GREATEST(COALESCE(subscription_ends_at, now()), now()) + ($2 || ' days')::interval,
		     updated_at = now() 
		 WHERE id = $3
		 RETURNING subscription_ends_at`, planName, itoa(int64(durationDays)), uid,
	).Scan(&newEndsAt)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to activate subscription"})
	}

	if err := tx.Commit(c.Context()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "transaction commit failed"})
	}

	writeAudit(c.Context(), h.Pool, uid, "", "REDEEM_CODE", "activation_codes", itoa(codeID), map[string]any{
		"code": code, "plan_name": planName, "duration_days": durationDays, "ends_at": newEndsAt,
	}, c.IP())

	return c.JSON(fiber.Map{
		"success":              true,
		"message":              "تم تفعيل الاشتراك بنجاح!",
		"plan_name":            planName,
		"duration_days":        durationDays,
		"subscription_ends_at": newEndsAt,
		"user": fiber.Map{
			"id":                      uid,
			"plan_name":               planName,
			"subscription_plan":       planName,
			"status":                  "active",
			"subscription_ends_at":    newEndsAt,
			"subscription_expires_at": newEndsAt,
		},
	})
}
