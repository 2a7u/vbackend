package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/middleware"
)

// DebtsHandler owns debt contracts and installment payments.
type DebtsHandler struct {
	Pool *pgxpool.Pool
	Hub  *WSHub
}

type contractRow struct {
	ID                 int64      `json:"id"`
	DebtorName         string     `json:"debtor_name"`
	Phone              string     `json:"phone"`
	Description        string     `json:"description"`
	SaleType           string     `json:"sale_type"`
	CostPrice          float64    `json:"cost_price"`
	TotalAmount        float64    `json:"total_amount"`
	PaidAmount         float64    `json:"paid_amount"`
	Remaining          float64    `json:"remaining_amount"`
	MonthlyInstallment float64    `json:"monthly_installment_amount"`
	InstallmentMonths  int        `json:"installment_months_count"`
	StartDate          *time.Time `json:"start_date,omitempty"`
	FirstDueDate       *time.Time `json:"first_due_date,omitempty"`
	Notes              string     `json:"notes"`
	LastReminderAt     *time.Time `json:"last_whatsapp_reminder_at,omitempty"`
	Status             string     `json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
}

// toMap serializes the contract with canonical snake_case keys AND every
// alias key legacy Flutter clients read, so no consumer breaks regardless of
// which field name it expects.
func (r contractRow) toMap() fiber.Map {
	m := fiber.Map{
		"id":                         r.ID,
		"debtor_name":                r.DebtorName,
		"customer_name":              r.DebtorName,
		"client_name":                r.DebtorName,
		"name":                       r.DebtorName,
		"phone":                      r.Phone,
		"debtor_phone":               r.Phone,
		"customer_phone":             r.Phone,
		"phone_number":               r.Phone,
		"description":                r.Description,
		"item_name":                  r.Description,
		"service_name":               r.Description,
		"item":                       r.Description,
		"item_description":           r.Description,
		"sale_type":                  r.SaleType,
		"contract_type":              r.SaleType,
		"cost_price":                 r.CostPrice,
		"total_amount":               r.TotalAmount,
		"amount":                     r.TotalAmount,
		"total_selling_price":        r.TotalAmount,
		"paid_amount":                r.PaidAmount,
		"paid":                       r.PaidAmount,
		"down_payment":               r.PaidAmount,
		"remaining_amount":           r.Remaining,
		"remaining":                  r.Remaining,
		"remaining_balance":          r.Remaining,
		"monthly_installment_amount": r.MonthlyInstallment,
		"installment_months_count":   r.InstallmentMonths,
		"installments_count":         r.InstallmentMonths,
		"installment_count":          r.InstallmentMonths,
		"notes":                      r.Notes,
		"contract_notes":             r.Notes,
		"status":                     r.Status,
		"created_at":                 r.CreatedAt,
	}
	if r.StartDate != nil {
		m["start_date"] = r.StartDate.Format("2006-01-02")
	}
	if r.FirstDueDate != nil {
		m["first_due_date"] = r.FirstDueDate.Format("2006-01-02")
		m["due_date"] = r.FirstDueDate.Format("2006-01-02")
		m["next_due_date"] = r.FirstDueDate.Format("2006-01-02")
	}
	if r.LastReminderAt != nil {
		m["last_whatsapp_reminder_at"] = r.LastReminderAt
	}
	return m
}

// contractColumns is the shared SELECT list for contractRow scans.
const contractColumns = `id, debtor_name, phone, description, sale_type, cost_price,
	total_amount, paid_amount, (total_amount - paid_amount) AS remaining_amount,
	monthly_installment_amount, installment_months_count, start_date, first_due_date,
	notes, last_whatsapp_reminder_at, status, created_at`

// ListDebts returns the caller's debt contracts (soft-deleted hidden).
// GET /api/debts
func (h *DebtsHandler) ListDebts(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	rows, err := h.Pool.Query(c.Context(),
		`SELECT `+contractColumns+` FROM debt_contracts WHERE user_id = $1 AND is_deleted = FALSE ORDER BY created_at DESC`, uid)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer rows.Close()

	out := make([]fiber.Map, 0)
	for rows.Next() {
		var r contractRow
		if err := scanContract(rows.Scan, &r); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "scan error"})
		}
		out = append(out, r.toMap())
	}
	return c.JSON(out)
}

// scanContract adapts a row-scan function to contractRow's column order.
func scanContract(scan func(dest ...interface{}) error, r *contractRow) error {
	return scan(&r.ID, &r.DebtorName, &r.Phone, &r.Description, &r.SaleType, &r.CostPrice,
		&r.TotalAmount, &r.PaidAmount, &r.Remaining, &r.MonthlyInstallment,
		&r.InstallmentMonths, &r.StartDate, &r.FirstDueDate, &r.Notes,
		&r.LastReminderAt, &r.Status, &r.CreatedAt)
}

// CreateDebt creates a new debt contract for the caller. Accepts the extended
// app payload (sale_type, cost_price, down_payment, installment plan, notes)
// while remaining compatible with the minimal original shape.
// POST /api/debts
func (h *DebtsHandler) CreateDebt(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	var req struct {
		DebtorName         string  `json:"debtor_name"`
		Phone              string  `json:"phone"`
		Description        string  `json:"description"`
		SaleType           string  `json:"sale_type"`
		CostPrice          float64 `json:"cost_price"`
		TotalAmount        float64 `json:"total_amount"`
		DownPayment        float64 `json:"down_payment"`
		MonthlyInstallment float64 `json:"monthly_installment_amount"`
		InstallmentMonths  int     `json:"installment_months_count"`
		StartDate          *string `json:"start_date"`
		FirstDueDate       *string `json:"first_due_date"`
		DueDate            *string `json:"due_date"` // legacy alias for first_due_date
		Notes              string  `json:"notes"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	req.DebtorName = strings.TrimSpace(req.DebtorName)
	if req.DebtorName == "" || req.TotalAmount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "debtor_name and total_amount > 0 are required"})
	}
	if req.DownPayment < 0 || req.DownPayment > req.TotalAmount {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "down_payment must be between 0 and total_amount"})
	}
	saleType := strings.TrimSpace(req.SaleType)
	if saleType == "" {
		saleType = "cash"
	}
	if saleType != "cash" && saleType != "installment" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "sale_type must be 'cash' or 'installment'"})
	}

	firstDue, ok := parseFlexibleDate(deref(req.FirstDueDate))
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid first_due_date format (use YYYY-MM-DD)"})
	}
	if firstDue == nil {
		firstDue, ok = parseFlexibleDate(deref(req.DueDate))
		if !ok {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid due_date format (use YYYY-MM-DD)"})
		}
	}
	startDate, ok := parseFlexibleDate(deref(req.StartDate))
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid start_date format (use YYYY-MM-DD)"})
	}

	status := "active"
	if req.DownPayment >= req.TotalAmount {
		status = "completed"
	}

	var r contractRow
	err := h.Pool.QueryRow(c.Context(),
		`INSERT INTO debt_contracts (user_id, debtor_name, phone, description, sale_type, cost_price,
		                            total_amount, paid_amount, monthly_installment_amount,
		                            installment_months_count, start_date, first_due_date, due_date, notes, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $12, $13, $14)
		 RETURNING `+contractColumns,
		uid, req.DebtorName, strings.TrimSpace(req.Phone), strings.TrimSpace(req.Description),
		saleType, req.CostPrice, req.TotalAmount, req.DownPayment,
		req.MonthlyInstallment, req.InstallmentMonths, startDate, firstDue,
		strings.TrimSpace(req.Notes), status,
	).Scan(&r.ID, &r.DebtorName, &r.Phone, &r.Description, &r.SaleType, &r.CostPrice,
		&r.TotalAmount, &r.PaidAmount, &r.Remaining, &r.MonthlyInstallment,
		&r.InstallmentMonths, &r.StartDate, &r.FirstDueDate, &r.Notes,
		&r.LastReminderAt, &r.Status, &r.CreatedAt)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if h.Hub != nil {
		h.Hub.Broadcast(uid, "debts", fiber.Map{"action": "created", "contract": r.toMap()})
	}
	if cl := middleware.ClaimsFrom(c); cl != nil {
		writeAudit(c.Context(), h.Pool, uid, cl.Email, "CREATE_DEBT", "debt_contracts", itoa(r.ID), map[string]any{
			"item": r.Description, "customer_name": r.DebtorName, "phone": r.Phone,
			"amount": r.TotalAmount, "paid": r.PaidAmount, "remaining": r.Remaining,
			"created_at": r.CreatedAt,
		}, c.IP())
	}
	return c.Status(fiber.StatusCreated).JSON(r.toMap())
}

// UpdateDebt edits editable fields of a contract. Only supplied fields change;
// total_amount/paid_amount stay consistent (paid_amount is clamped to the new
// total when the total is lowered below it).
// PUT /api/debts/:id
func (h *DebtsHandler) UpdateDebt(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	id, err := paramID(c)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req struct {
		DebtorName         *string  `json:"debtor_name"`
		Phone              *string  `json:"phone"`
		Description        *string  `json:"description"`
		SaleType           *string  `json:"sale_type"`
		CostPrice          *float64 `json:"cost_price"`
		TotalAmount        *float64 `json:"total_amount"`
		DownPayment        *float64 `json:"down_payment"`
		MonthlyInstallment *float64 `json:"monthly_installment_amount"`
		InstallmentMonths  *int     `json:"installment_months_count"`
		StartDate          *string  `json:"start_date"`
		FirstDueDate       *string  `json:"first_due_date"`
		Notes              *string  `json:"notes"`
		LastReminderAt     *string  `json:"last_whatsapp_reminder_at"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}

	tx, err := h.Pool.Begin(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer tx.Rollback(c.Context())

	var total, paid float64
	err = tx.QueryRow(c.Context(),
		`SELECT total_amount, paid_amount FROM debt_contracts WHERE id = $1 AND user_id = $2 FOR UPDATE`, id, uid,
	).Scan(&total, &paid)
	if err == pgx.ErrNoRows {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "contract not found"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}

	sets := make([]string, 0, 14)
	args := make([]interface{}, 0, 15)
	add := func(col string, val interface{}) {
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)+1))
		args = append(args, val)
	}
	if req.DebtorName != nil {
		v := strings.TrimSpace(*req.DebtorName)
		if v == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "debtor_name cannot be empty"})
		}
		add("debtor_name", v)
	}
	if req.Phone != nil {
		add("phone", strings.TrimSpace(*req.Phone))
	}
	if req.Description != nil {
		add("description", strings.TrimSpace(*req.Description))
	}
	if req.SaleType != nil {
		v := strings.TrimSpace(*req.SaleType)
		if v != "cash" && v != "installment" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "sale_type must be 'cash' or 'installment'"})
		}
		add("sale_type", v)
	}
	if req.CostPrice != nil {
		if *req.CostPrice < 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cost_price must be >= 0"})
		}
		add("cost_price", *req.CostPrice)
	}
	if req.TotalAmount != nil {
		if *req.TotalAmount <= 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "total_amount must be > 0"})
		}
		add("total_amount", *req.TotalAmount)
		total = *req.TotalAmount
	}
	if req.DownPayment == nil && paid > total {
		// Clamp paid_amount so the CHECK (paid_amount <= total_amount) holds.
		add("paid_amount", total)
		paid = total
	}
	if req.DownPayment != nil {
		if *req.DownPayment < 0 || *req.DownPayment > total {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "down_payment must be between 0 and total_amount"})
		}
		add("paid_amount", *req.DownPayment)
		paid = *req.DownPayment
	}
	if req.MonthlyInstallment != nil {
		if *req.MonthlyInstallment < 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "monthly_installment_amount must be >= 0"})
		}
		add("monthly_installment_amount", *req.MonthlyInstallment)
	}
	if req.InstallmentMonths != nil {
		if *req.InstallmentMonths < 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "installment_months_count must be >= 0"})
		}
		add("installment_months_count", *req.InstallmentMonths)
	}
	if req.StartDate != nil {
		d, ok := parseFlexibleDate(*req.StartDate)
		if !ok {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid start_date format (use YYYY-MM-DD)"})
		}
		add("start_date", d)
	}
	if req.FirstDueDate != nil {
		d, ok := parseFlexibleDate(*req.FirstDueDate)
		if !ok {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid first_due_date format (use YYYY-MM-DD)"})
		}
		add("first_due_date", d)
		add("due_date", d)
	}
	if req.Notes != nil {
		add("notes", strings.TrimSpace(*req.Notes))
	}
	if req.LastReminderAt != nil {
		d, ok := parseFlexibleDate(*req.LastReminderAt)
		if !ok {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid last_whatsapp_reminder_at format"})
		}
		add("last_whatsapp_reminder_at", d)
	}
	if len(sets) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "nothing to update"})
	}

	// Keep status consistent with the new balances.
	newStatus := "active"
	if paid >= total {
		newStatus = "completed"
	}
	add("status", newStatus)

	args = append(args, id, uid)
	var r contractRow
	err = tx.QueryRow(c.Context(),
		`UPDATE debt_contracts SET `+strings.Join(sets, ", ")+`
		 WHERE id = $`+fmt.Sprintf("%d", len(args)-1)+` AND user_id = $`+fmt.Sprintf("%d", len(args))+`
		 RETURNING `+contractColumns, args...,
	).Scan(&r.ID, &r.DebtorName, &r.Phone, &r.Description, &r.SaleType, &r.CostPrice,
		&r.TotalAmount, &r.PaidAmount, &r.Remaining, &r.MonthlyInstallment,
		&r.InstallmentMonths, &r.StartDate, &r.FirstDueDate, &r.Notes,
		&r.LastReminderAt, &r.Status, &r.CreatedAt)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not update contract"})
	}
	if err := tx.Commit(c.Context()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "commit failed"})
	}
	if h.Hub != nil {
		h.Hub.Broadcast(uid, "debts", fiber.Map{"action": "updated", "contract": r.toMap()})
	}
	return c.JSON(r.toMap())
}

// DeleteDebt permanently deletes a contract (payments cascade).
// R4: bounded deletion (5/day, 24h window for non-admins) + audit archive.
// DELETE /api/debts/:id
func (h *DebtsHandler) DeleteDebt(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	cl := middleware.ClaimsFrom(c)
	isAdmin := cl != nil && cl.Role == "admin"
	id, err := paramID(c)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var (
		debtorName string
		phone      string
		desc       string
		total      float64
		paid       float64
		createdAt  time.Time
	)
	err = h.Pool.QueryRow(c.Context(),
		`SELECT debtor_name, phone, description, total_amount, paid_amount, created_at
		 FROM debt_contracts WHERE id = $1 AND user_id = $2 AND is_deleted = FALSE`, id, uid,
	).Scan(&debtorName, &phone, &desc, &total, &paid, &createdAt)
	if err == pgx.ErrNoRows {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "contract not found"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if ok, status, msg := checkDeletionGuard(c.Context(), h.Pool, uid, isAdmin, createdAt); !ok {
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	res, err := h.Pool.Exec(c.Context(), `DELETE FROM debt_contracts WHERE id = $1 AND user_id = $2`, id, uid)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if res.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "contract not found"})
	}
	email := ""
	if cl != nil {
		email = cl.Email
	}
	writeAudit(c.Context(), h.Pool, uid, email, "DELETE_DEBT", "debt_contracts", itoa(id), map[string]any{
		"item": desc, "customer_name": debtorName, "phone": phone,
		"amount": total, "paid": paid, "remaining": total - paid,
		"deleted_at": time.Now(),
	}, c.IP())
	if h.Hub != nil {
		h.Hub.Broadcast(uid, "debts", fiber.Map{"action": "deleted", "contract_id": id})
	}
	return c.JSON(fiber.Map{"success": true})
}

// deref returns the pointed-to string, or "" for nil.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// PayDebt records an installment inside a strict transaction:
// SELECT ... FOR UPDATE on the contract row, then insert payment and update
// paid_amount/status. Returns exact previous/paid/remaining balances.
// POST /api/debts/:id/pay
func (h *DebtsHandler) PayDebt(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	id, err := paramID(c)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	var req struct {
		Amount float64 `json:"amount"`
		Note   string  `json:"note"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	if req.Amount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "amount must be > 0"})
	}

	tx, err := h.Pool.Begin(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer tx.Rollback(c.Context())

	// Lock the contract row; ownership enforced by user_id in the WHERE clause.
	var (
		total  float64
		paid   float64
		status string
	)
	err = tx.QueryRow(c.Context(),
		`SELECT total_amount, paid_amount, status FROM debt_contracts
		 WHERE id = $1 AND user_id = $2 AND is_deleted = FALSE FOR UPDATE`, id, uid,
	).Scan(&total, &paid, &status)
	if err == pgx.ErrNoRows {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "contract not found"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if status == "completed" {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "contract already completed"})
	}

	previousBalance := total - paid
	if req.Amount > previousBalance {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "payment exceeds remaining balance", "remaining_amount": previousBalance,
		})
	}

	newPaid := paid + req.Amount
	remaining := total - newPaid
	newStatus := status
	if remaining == 0 {
		newStatus = "completed"
	}

	var paymentID int64
	var createdAt time.Time
	err = tx.QueryRow(c.Context(),
		`INSERT INTO debt_payments (user_id, contract_id, amount, previous_balance, remaining_amount, note)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at`,
		uid, id, req.Amount, previousBalance, remaining, strings.TrimSpace(req.Note),
	).Scan(&paymentID, &createdAt)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not record payment"})
	}

	if _, err := tx.Exec(c.Context(),
		`UPDATE debt_contracts SET paid_amount = $1, status = $2 WHERE id = $3 AND user_id = $4`,
		newPaid, newStatus, id, uid); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not update contract"})
	}

	if err := tx.Commit(c.Context()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "commit failed"})
	}

	result := fiber.Map{
		"success": true, "payment_id": paymentID, "contract_id": id,
		"amount": req.Amount, "previous_balance": previousBalance,
		"paid_amount": newPaid, "remaining_amount": remaining,
		"status": newStatus, "created_at": createdAt,
	}
	if h.Hub != nil {
		h.Hub.Broadcast(uid, "debts", fiber.Map{"action": "payment", "contract_id": id, "remaining_amount": remaining, "status": newStatus})
	}
	if cl := middleware.ClaimsFrom(c); cl != nil {
		writeAudit(c.Context(), h.Pool, uid, cl.Email, "PAY_DEBT", "debt_contracts", itoa(id), map[string]any{
			"payment_id": paymentID, "amount": req.Amount,
			"previous_balance": previousBalance, "remaining_amount": remaining,
			"status": newStatus, "paid_at": createdAt,
		}, c.IP())
	}
	return c.JSON(result)
}

// ListPayments returns the payment ledger for one contract.
// GET /api/debts/:id/payments
func (h *DebtsHandler) ListPayments(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	id, err := paramID(c)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	rows, err := h.Pool.Query(c.Context(),
		`SELECT p.id, p.contract_id, p.amount, p.previous_balance, p.remaining_amount, p.note, p.created_at
		 FROM debt_payments p
		 JOIN debt_contracts d ON d.id = p.contract_id
		 WHERE p.contract_id = $1 AND d.user_id = $2 AND d.is_deleted = FALSE ORDER BY p.created_at DESC`, id, uid)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer rows.Close()

	type payment struct {
		ID              int64     `json:"id"`
		ContractID      int64     `json:"contract_id"`
		Amount          float64   `json:"amount"`
		PreviousBalance float64   `json:"previous_balance"`
		RemainingAmount float64   `json:"remaining_amount"`
		Note            string    `json:"note"`
		CreatedAt       time.Time `json:"created_at"`
	}
	out := make([]payment, 0)
	for rows.Next() {
		var p payment
		if err := rows.Scan(&p.ID, &p.ContractID, &p.Amount, &p.PreviousBalance, &p.RemainingAmount, &p.Note, &p.CreatedAt); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "scan error"})
		}
		out = append(out, p)
	}
	return c.JSON(out)
}
