package handlers

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/middleware"
)

// AnnouncementsHandler owns admin notices: every authenticated user can list
// them; only admins can create/update/delete (user_id NULL = broadcast).
type AnnouncementsHandler struct {
	Pool *pgxpool.Pool
	Hub  *WSHub
}

type announcementRow struct {
	ID             int64      `json:"id"`
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	ImageURL       string     `json:"image_url"`
	TargetAudience string     `json:"target_audience"`
	IsActive       bool       `json:"is_active"`
	CreatedBy      *int64     `json:"created_by,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
}

// ListAnnouncements returns active announcements for the caller (admins also
// see inactive ones so the console can manage them).
// GET /api/announcements
func (h *AnnouncementsHandler) ListAnnouncements(c *fiber.Ctx) error {
	cl := middleware.ClaimsFrom(c)
	if cl == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthenticated"})
	}
	query := `SELECT id, title, body, image_url, target_audience, is_active, created_by, created_at, updated_at
	          FROM announcements`
	if cl.Role != "admin" {
		query += ` WHERE is_active = TRUE`
	}
	query += ` ORDER BY created_at DESC LIMIT 200`
	rows, err := h.Pool.Query(c.Context(), query)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer rows.Close()

	out := make([]announcementRow, 0)
	for rows.Next() {
		var a announcementRow
		if err := rows.Scan(&a.ID, &a.Title, &a.Body, &a.ImageURL, &a.TargetAudience,
			&a.IsActive, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "scan error"})
		}
		out = append(out, a)
	}
	return c.JSON(out)
}

// SaveAnnouncement (admin) creates or updates an announcement. When "id" is
// present the existing row is updated; otherwise a new broadcast is inserted.
// POST /api/admin/announcements
func (h *AnnouncementsHandler) SaveAnnouncement(c *fiber.Ctx) error {
	cl := middleware.ClaimsFrom(c)
	if cl == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthenticated"})
	}
	var req struct {
		ID             *int64 `json:"id"`
		Title          string `json:"title"`
		Body           string `json:"body"`
		ImageURL       string `json:"image_url"`
		TargetAudience string `json:"target_audience"`
		IsActive       *bool  `json:"is_active"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "title is required"})
	}
	audience := strings.TrimSpace(req.TargetAudience)
	if audience == "" {
		audience = "all"
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	var a announcementRow
	if req.ID != nil && *req.ID > 0 {
		err := h.Pool.QueryRow(c.Context(),
			`UPDATE announcements SET title = $1, body = $2, image_url = $3, target_audience = $4,
			        is_active = $5, updated_at = now()
			 WHERE id = $6
			 RETURNING id, title, body, image_url, target_audience, is_active, created_by, created_at, updated_at`,
			req.Title, req.Body, strings.TrimSpace(req.ImageURL), audience, isActive, *req.ID,
		).Scan(&a.ID, &a.Title, &a.Body, &a.ImageURL, &a.TargetAudience, &a.IsActive, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt)
		if err == pgx.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "announcement not found"})
		}
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
		}
	} else {
		err := h.Pool.QueryRow(c.Context(),
			`INSERT INTO announcements (title, body, image_url, target_audience, is_active, created_by)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 RETURNING id, title, body, image_url, target_audience, is_active, created_by, created_at, updated_at`,
			req.Title, req.Body, strings.TrimSpace(req.ImageURL), audience, isActive, cl.UserID,
		).Scan(&a.ID, &a.Title, &a.Body, &a.ImageURL, &a.TargetAudience, &a.IsActive, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
		}
	}
	if h.Hub != nil {
		h.Hub.Broadcast(0, "announcements", fiber.Map{"action": "saved", "announcement": a}) // 0 = all users
	}
	return c.JSON(a)
}

// DeleteAnnouncement (admin) removes an announcement.
// DELETE /api/admin/announcements/:id
func (h *AnnouncementsHandler) DeleteAnnouncement(c *fiber.Ctx) error {
	id, err := paramID(c)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	res, err := h.Pool.Exec(c.Context(), `DELETE FROM announcements WHERE id = $1`, id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if res.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "announcement not found"})
	}
	if h.Hub != nil {
		h.Hub.Broadcast(0, "announcements", fiber.Map{"action": "deleted", "announcement_id": id})
	}
	return c.JSON(fiber.Map{"success": true})
}
