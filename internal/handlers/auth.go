// Package handlers implements all HTTP endpoints of the VENUS POS backend.
package handlers

import (
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
		id           int64
		email        string
		passwordHash string
		role         string
		isApproved   bool
	)
	err := h.Pool.QueryRow(c.Context(),
		`SELECT id, email, password_hash, role, is_approved FROM users WHERE email = $1`, req.Email,
	).Scan(&id, &email, &passwordHash, &role, &isApproved)
	if err == pgx.ErrNoRows {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid credentials"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)) != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid credentials"})
	}
	if role == "subscriber" && !isApproved {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "account pending admin approval"})
	}

	token, err := middleware.SignToken(h.JWTSecret, id, email, role, h.TokenTTL)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not sign token"})
	}
	return c.JSON(fiber.Map{
		"token": token,
		"user":  fiber.Map{"id": id, "email": email, "role": role, "is_approved": isApproved},
	})
}

// Register creates a new subscriber account (pending approval unless it is the
// configured super admin email, which is auto-approved as admin).
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
	approved := false
	if h.SuperAdminEmail != "" && req.Email == strings.ToLower(strings.TrimSpace(h.SuperAdminEmail)) {
		role = "admin"
		approved = true
	}

	var id int64
	err = h.Pool.QueryRow(c.Context(),
		`INSERT INTO users (email, password_hash, full_name, store_name, phone, role, is_approved)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (email) DO NOTHING
		 RETURNING id`,
		req.Email, hash, strings.TrimSpace(req.FullName), strings.TrimSpace(req.StoreName),
		strings.TrimSpace(req.Phone), role, approved,
	).Scan(&id)
	if err == pgx.ErrNoRows {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "email already registered"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id": id, "email": req.Email, "full_name": strings.TrimSpace(req.FullName),
		"store_name": strings.TrimSpace(req.StoreName), "phone": strings.TrimSpace(req.Phone),
		"role": role, "is_approved": approved,
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
		id        int64
		email     string
		fullName  string
		storeName string
		phone     string
		role      string
		approved  bool
		createdAt time.Time
	)
	err := h.Pool.QueryRow(c.Context(),
		`SELECT id, email, full_name, store_name, phone, role, is_approved, created_at
		 FROM users WHERE id = $1`, cl.UserID,
	).Scan(&id, &email, &fullName, &storeName, &phone, &role, &approved, &createdAt)
	if err == pgx.ErrNoRows {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	return c.JSON(fiber.Map{
		"id": id, "email": email, "full_name": fullName, "store_name": storeName,
		"phone": phone, "role": role, "is_approved": approved, "created_at": createdAt,
	})
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
		`SELECT id, email, full_name, store_name, phone, role, is_approved, created_at FROM users ORDER BY created_at DESC`)
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
		)
		if err := rows.Scan(&id, &email, &fullName, &storeName, &phone, &role, &approved, &createdAt); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "scan error"})
		}
		users = append(users, fiber.Map{
			"id": id, "email": email, "full_name": fullName, "store_name": storeName,
			"phone": phone, "role": role, "is_approved": approved, "created_at": createdAt,
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
