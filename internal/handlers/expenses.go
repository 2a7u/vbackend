package handlers

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/middleware"
)

// ExpensesHandler owns store expense CRUD for the authenticated subscriber.
type ExpensesHandler struct {
	Pool *pgxpool.Pool
	Hub  *WSHub
}

// ListExpenses returns the caller's store expenses (newest first).
// GET /api/expenses
func (h *ExpensesHandler) ListExpenses(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	rows, err := h.Pool.Query(c.Context(),
		`SELECT id, title, amount, category, notes, created_at
		 FROM store_expenses WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1000`, uid)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer rows.Close()

	type expense struct {
		ID        int64     `json:"id"`
		Title     string    `json:"title"`
		Amount    float64   `json:"amount"`
		Category  string    `json:"category"`
		Notes     string    `json:"notes"`
		CreatedAt time.Time `json:"created_at"`
	}
	out := make([]expense, 0)
	for rows.Next() {
		var e expense
		if err := rows.Scan(&e.ID, &e.Title, &e.Amount, &e.Category, &e.Notes, &e.CreatedAt); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "scan error"})
		}
		out = append(out, e)
	}
	return c.JSON(out)
}

// AddExpense records a store expense for the caller.
// POST /api/expenses
func (h *ExpensesHandler) AddExpense(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	var req struct {
		Title    string  `json:"title"`
		Amount   float64 `json:"amount"`
		Category string  `json:"category"`
		Notes    string  `json:"notes"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "title is required"})
	}
	if req.Amount < 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "amount must be >= 0"})
	}

	var id int64
	var createdAt time.Time
	err := h.Pool.QueryRow(c.Context(),
		`INSERT INTO store_expenses (user_id, title, amount, category, notes)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`,
		uid, req.Title, req.Amount, strings.TrimSpace(req.Category), strings.TrimSpace(req.Notes),
	).Scan(&id, &createdAt)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if h.Hub != nil {
		h.Hub.Broadcast(uid, "expenses", fiber.Map{"action": "created", "expense_id": id})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id": id, "title": req.Title, "amount": req.Amount,
		"category": strings.TrimSpace(req.Category), "notes": strings.TrimSpace(req.Notes),
		"created_at": createdAt,
	})
}

// DeleteExpense permanently removes an expense owned by the caller.
// DELETE /api/expenses/:id
func (h *ExpensesHandler) DeleteExpense(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	id, err := paramID(c)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	res, err := h.Pool.Exec(c.Context(), `DELETE FROM store_expenses WHERE id = $1 AND user_id = $2`, id, uid)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if res.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "expense not found"})
	}
	if h.Hub != nil {
		h.Hub.Broadcast(uid, "expenses", fiber.Map{"action": "deleted", "expense_id": id})
	}
	return c.JSON(fiber.Map{"success": true})
}
