package handlers

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/middleware"
)

// TicketsHandler owns subscriber support tickets and admin replies.
type TicketsHandler struct {
	Pool *pgxpool.Pool
	Hub  *WSHub
}

type ticketRow struct {
	ID           int64      `json:"id"`
	UserID       int64      `json:"user_id"`
	StoreName    string     `json:"store_name"`
	ContactPhone string     `json:"contact_phone"`
	Subject      string     `json:"subject"`
	Message      string     `json:"message"`
	Priority     string     `json:"priority"`
	Status       string     `json:"status"`
	Reply        *string    `json:"reply,omitempty"`
	RepliedAt    *time.Time `json:"replied_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// ticketColumns is the shared SELECT list for ticketRow scans.
const ticketColumns = `id, user_id, store_name, contact_phone, subject, message, priority, status, reply, replied_at, created_at`

// ListMyTickets returns the caller's own tickets.
// GET /api/tickets
func (h *TicketsHandler) ListMyTickets(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	rows, err := h.Pool.Query(c.Context(),
		`SELECT `+ticketColumns+` FROM support_tickets WHERE user_id = $1 ORDER BY created_at DESC`, uid)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer rows.Close()

	out := make([]ticketRow, 0)
	for rows.Next() {
		var t ticketRow
		if err := scanTicket(rows.Scan, &t); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "scan error"})
		}
		out = append(out, t)
	}
	return c.JSON(out)
}

// scanTicket adapts a row-scan function to ticketRow's column order.
func scanTicket(scan func(dest ...interface{}) error, t *ticketRow) error {
	return scan(&t.ID, &t.UserID, &t.StoreName, &t.ContactPhone, &t.Subject, &t.Message,
		&t.Priority, &t.Status, &t.Reply, &t.RepliedAt, &t.CreatedAt)
}

// CreateTicket lets a subscriber open a support ticket.
// POST /api/tickets
func (h *TicketsHandler) CreateTicket(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	var req struct {
		Subject      string `json:"subject"`
		Message      string `json:"message"`
		Priority     string `json:"priority"`
		StoreName    string `json:"store_name"`
		ContactPhone string `json:"contact_phone"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	req.Subject = strings.TrimSpace(req.Subject)
	req.Message = strings.TrimSpace(req.Message)
	if req.Subject == "" || req.Message == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "subject and message are required"})
	}
	priority := strings.ToLower(strings.TrimSpace(req.Priority))
	if priority != "low" && priority != "normal" && priority != "high" && priority != "urgent" {
		priority = "normal"
	}

	// Fall back to the account's stored store name / phone when not supplied.
	storeName := strings.TrimSpace(req.StoreName)
	contactPhone := strings.TrimSpace(req.ContactPhone)
	if storeName == "" || contactPhone == "" {
		var sn, ph string
		if err := h.Pool.QueryRow(c.Context(),
			`SELECT store_name, phone FROM users WHERE id = $1`, uid,
		).Scan(&sn, &ph); err == nil {
			if storeName == "" {
				storeName = sn
			}
			if contactPhone == "" {
				contactPhone = ph
			}
		}
	}

	var t ticketRow
	err := h.Pool.QueryRow(c.Context(),
		`INSERT INTO support_tickets (user_id, store_name, contact_phone, subject, message, priority)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING `+ticketColumns,
		uid, storeName, contactPhone, req.Subject, req.Message, priority,
	).Scan(&t.ID, &t.UserID, &t.StoreName, &t.ContactPhone, &t.Subject, &t.Message,
		&t.Priority, &t.Status, &t.Reply, &t.RepliedAt, &t.CreatedAt)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if h.Hub != nil {
		h.Hub.Broadcast(0, "tickets", fiber.Map{"action": "new_ticket", "ticket_id": t.ID, "user_id": uid, "subject": t.Subject}) // 0 = admin broadcast
		h.Hub.Broadcast(uid, "tickets", fiber.Map{"action": "created", "ticket": t})
	}
	return c.Status(fiber.StatusCreated).JSON(t)
}

// ListAllTickets (admin) returns every ticket across subscribers.
// GET /api/admin/tickets
func (h *TicketsHandler) ListAllTickets(c *fiber.Ctx) error {
	rows, err := h.Pool.Query(c.Context(),
		`SELECT t.id, t.user_id, t.store_name, t.contact_phone, t.subject, t.message, t.priority,
		        t.status, t.reply, t.replied_at, t.created_at, u.email, u.store_name AS owner_store_name
		 FROM support_tickets t JOIN users u ON u.id = t.user_id
		 ORDER BY t.created_at DESC`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer rows.Close()

	type adminTicket struct {
		ticketRow
		UserEmail       string `json:"user_email"`
		OwnerStoreName  string `json:"owner_store_name"`
	}
	out := make([]adminTicket, 0)
	for rows.Next() {
		var t adminTicket
		if err := rows.Scan(&t.ID, &t.UserID, &t.StoreName, &t.ContactPhone, &t.Subject, &t.Message,
			&t.Priority, &t.Status, &t.Reply, &t.RepliedAt, &t.CreatedAt, &t.UserEmail, &t.OwnerStoreName); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "scan error"})
		}
		out = append(out, t)
	}
	return c.JSON(out)
}

// ReplyTicket (admin) stores the admin reply and marks the ticket replied.
// PUT /api/admin/tickets/:id/reply
func (h *TicketsHandler) ReplyTicket(c *fiber.Ctx) error {
	id, err := paramID(c)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req struct {
		Reply  string `json:"reply"`
		Status string `json:"status"` // optional: "replied" (default) or "closed"
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	req.Reply = strings.TrimSpace(req.Reply)
	if req.Reply == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "reply is required"})
	}
	status := req.Status
	if status == "" {
		status = "replied"
	}
	if status != "replied" && status != "closed" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "status must be 'replied' or 'closed'"})
	}

	var ownerID int64
	err = h.Pool.QueryRow(c.Context(),
		`UPDATE support_tickets SET reply = $1, status = $2, replied_at = now()
		 WHERE id = $3 RETURNING user_id`, req.Reply, status, id,
	).Scan(&ownerID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if h.Hub != nil {
		h.Hub.Broadcast(ownerID, "tickets", fiber.Map{"action": "replied", "ticket_id": id, "reply": req.Reply, "status": status})
	}
	return c.JSON(fiber.Map{"success": true, "ticket_id": id, "status": status})
}
