package handlers

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/middleware"
)

// SimHandler owns SIM inventory and SIM/credit sales endpoints.
type SimHandler struct {
	Pool *pgxpool.Pool
	Hub  *WSHub
}

type simRow struct {
	ID           int64      `json:"id"`
	Barcode      string     `json:"barcode"`
	Operator     string     `json:"operator"`
	BatchName    string     `json:"batch_name"`
	PhoneNumber  string     `json:"phone_number"`
	PurchasePrice float64   `json:"purchase_price"`
	SellingPrice float64    `json:"selling_price"`
	Reward       float64    `json:"reward"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	SoldAt       *time.Time `json:"sold_at,omitempty"`
}

type addSimsRequest struct {
	Operator       string   `json:"operator"`
	BatchName      string   `json:"batch_name"`
	PurchasePrice  float64  `json:"purchase_price"`
	SellingPrice   float64  `json:"selling_price"`
	Reward         float64  `json:"reward"`
	Barcodes       []string `json:"barcodes"` // batch add
}

type sellSimRequest struct {
	SimID             int64   `json:"sim_id"`
	Barcode           string  `json:"barcode"`
	SellingPrice      float64 `json:"selling_price"`
	Reward            float64 `json:"reward"`
	Notes             string  `json:"notes"`
	ClientRequestID   string  `json:"client_request_id"`
	SoldByName        string  `json:"sold_by_name"`
	SimCost           float64 `json:"sim_cost"`
	AddedCreditCost   float64 `json:"added_credit_cost"`
	ActivationReward  float64 `json:"activation_reward"`
	MonthlyCommission float64 `json:"monthly_commission"`
}

// ListSims returns the caller's inventory (optionally filtered by ?status=).
// GET /api/sims
func (h *SimHandler) ListSims(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	status := strings.TrimSpace(c.Query("status"))

	var rows pgx.Rows
	var err error
	if status != "" {
		rows, err = h.Pool.Query(c.Context(),
			`SELECT id, barcode, operator, batch_name, phone_number, purchase_price, selling_price, reward, status, created_at, sold_at
			 FROM sim_inventory WHERE user_id = $1 AND status = $2 ORDER BY created_at DESC`, uid, status)
	} else {
		rows, err = h.Pool.Query(c.Context(),
			`SELECT id, barcode, operator, batch_name, phone_number, purchase_price, selling_price, reward, status, created_at, sold_at
			 FROM sim_inventory WHERE user_id = $1 ORDER BY created_at DESC`, uid)
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer rows.Close()

	sims := make([]simRow, 0)
	for rows.Next() {
		var s simRow
		if err := rows.Scan(&s.ID, &s.Barcode, &s.Operator, &s.BatchName, &s.PhoneNumber, &s.PurchasePrice, &s.SellingPrice, &s.Reward, &s.Status, &s.CreatedAt, &s.SoldAt); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "scan error"})
		}
		sims = append(sims, s)
	}
	return c.JSON(sims)
}

// AddSims batch-inserts barcodes into the caller's inventory.
// POST /api/sims
func (h *SimHandler) AddSims(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	var req addSimsRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	req.Operator = strings.TrimSpace(req.Operator)
	if req.Operator == "" || len(req.Barcodes) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "operator and at least one barcode are required"})
	}
	if req.PurchasePrice < 0 || req.SellingPrice < 0 || req.Reward < 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "prices must be >= 0"})
	}

	tx, err := h.Pool.Begin(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer tx.Rollback(c.Context())

	added := make([]simRow, 0, len(req.Barcodes))
	duplicates := make([]string, 0)
	for _, raw := range req.Barcodes {
		barcode := strings.TrimSpace(raw)
		if barcode == "" {
			continue
		}
		var s simRow
		err := tx.QueryRow(c.Context(),
			`INSERT INTO sim_inventory (user_id, barcode, operator, batch_name, purchase_price, selling_price, reward)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (user_id, barcode) DO NOTHING
			 RETURNING id, barcode, operator, batch_name, phone_number, purchase_price, selling_price, reward, status, created_at, sold_at`,
			uid, barcode, req.Operator, strings.TrimSpace(req.BatchName), req.PurchasePrice, req.SellingPrice, req.Reward,
		).Scan(&s.ID, &s.Barcode, &s.Operator, &s.BatchName, &s.PhoneNumber, &s.PurchasePrice, &s.SellingPrice, &s.Reward, &s.Status, &s.CreatedAt, &s.SoldAt)
		if err == pgx.ErrNoRows {
			duplicates = append(duplicates, barcode)
			continue
		}
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "insert failed: " + err.Error()})
		}
		added = append(added, s)
	}
	if err := tx.Commit(c.Context()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "commit failed"})
	}

	if h.Hub != nil {
		h.Hub.Broadcast(uid, "inventory", fiber.Map{"action": "added", "count": len(added)})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"added": added, "duplicates": duplicates})
}

// SellSim atomically marks a SIM sold and records the sale with exact profit.
// profit = (selling_price + reward) - purchase_price
// Idempotent: when client_request_id is supplied and a sale with that key
// already exists, the original sale is returned with already_sold=true.
// When the barcode exists but is already sold (and no idempotency key was
// matched), a 409 with already_sold=true is returned.
// POST /api/sims/sell
func (h *SimHandler) SellSim(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	var req sellSimRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	req.Barcode = strings.TrimSpace(req.Barcode)
	req.ClientRequestID = strings.TrimSpace(req.ClientRequestID)
	if req.SimID <= 0 && req.Barcode == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "sim_id or barcode is required"})
	}
	if req.SellingPrice < 0 || req.Reward < 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "prices must be >= 0"})
	}

	// Idempotency replay: return the original sale instead of double-selling.
	if req.ClientRequestID != "" {
		var saleID, simID int64
		var barcode, operator string
		var purchasePrice, sellingPrice, reward, profit float64
		var notes string
		var createdAt time.Time
		err := h.Pool.QueryRow(c.Context(),
			`SELECT id, sim_id, barcode, operator, purchase_price, selling_price, reward, profit, notes, created_at
			 FROM sim_sales WHERE user_id = $1 AND client_request_id = $2 AND is_deleted = FALSE`,
			uid, req.ClientRequestID,
		).Scan(&saleID, &simID, &barcode, &operator, &purchasePrice, &sellingPrice, &reward, &profit, &notes, &createdAt)
		if err == nil {
			return c.JSON(fiber.Map{
				"success": true, "already_sold": true, "sale_id": saleID, "sim_id": simID,
				"barcode": barcode, "operator": operator,
				"purchase_price": purchasePrice, "selling_price": sellingPrice,
				"reward": reward, "profit": profit, "notes": notes, "created_at": createdAt,
			})
		}
		if err != pgx.ErrNoRows {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
		}
	}

	// Short transaction: lock the inventory row, verify ownership + status,
	// mark sold, insert the sale. No external calls inside the tx.
	tx, err := h.Pool.Begin(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer tx.Rollback(c.Context())

	var (
		simID         int64
		barcode       string
		operator      string
		purchasePrice float64
		dbSelling     float64
		dbReward      float64
	)
	query := `SELECT id, barcode, operator, purchase_price, selling_price, reward
	          FROM sim_inventory WHERE user_id = $1 AND status = 'in_stock' AND `
	args := []interface{}{uid}
	if req.SimID > 0 {
		query += `id = $2 FOR UPDATE`
		args = append(args, req.SimID)
	} else {
		query += `barcode = $2 FOR UPDATE`
		args = append(args, req.Barcode)
	}
	err = tx.QueryRow(c.Context(), query, args...).Scan(&simID, &barcode, &operator, &purchasePrice, &dbSelling, &dbReward)
	if err == pgx.ErrNoRows {
		// Distinguish "already sold" from "not found" so the client can react.
		var exists bool
		qerr := h.Pool.QueryRow(c.Context(),
			`SELECT EXISTS(SELECT 1 FROM sim_inventory WHERE user_id = $1 AND barcode = $2 AND status = 'sold')`,
			uid, req.Barcode,
		).Scan(&exists)
		if qerr == nil && exists {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "SIM already sold", "already_sold": true, "barcode": req.Barcode,
			})
		}
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "SIM not found in stock for this account"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}

	sellingPrice := req.SellingPrice
	if sellingPrice == 0 {
		sellingPrice = dbSelling
	}
	reward := req.Reward
	if reward == 0 {
		reward = dbReward
	}
	profit := (sellingPrice + reward) - purchasePrice

	if _, err := tx.Exec(c.Context(),
		`UPDATE sim_inventory SET status = 'sold', sold_at = now() WHERE id = $1 AND user_id = $2`, simID, uid); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not mark SIM sold"})
	}

	var saleID int64
	var createdAt time.Time
	err = tx.QueryRow(c.Context(),
		`INSERT INTO sim_sales (user_id, sim_id, barcode, operator, purchase_price, selling_price, reward, profit, notes,
		                        sim_cost, added_credit_cost, activation_reward, monthly_commission, sold_by_name, client_request_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		 RETURNING id, created_at`,
		uid, simID, barcode, operator, purchasePrice, sellingPrice, reward, profit, strings.TrimSpace(req.Notes),
		req.SimCost, req.AddedCreditCost, req.ActivationReward, req.MonthlyCommission,
		strings.TrimSpace(req.SoldByName), nullableString(req.ClientRequestID),
	).Scan(&saleID, &createdAt)
	if err != nil {
		// Unique-violation on client_request_id means a concurrent duplicate:
		// treat it as already_sold for idempotent clients.
		if req.ClientRequestID != "" && strings.Contains(err.Error(), "uq_sim_sales_client_request") {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "sale already recorded", "already_sold": true, "barcode": barcode,
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not record sale"})
	}

	if err := tx.Commit(c.Context()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "commit failed"})
	}

	if h.Hub != nil {
		h.Hub.Broadcast(uid, "sales", fiber.Map{
			"action": "sim_sold", "sale_id": saleID, "sim_id": simID, "barcode": barcode,
			"profit": profit, "selling_price": sellingPrice, "reward": reward, "created_at": createdAt,
		})
	}
	return c.JSON(fiber.Map{
		"success": true, "sale_id": saleID, "sim_id": simID, "barcode": barcode, "operator": operator,
		"purchase_price": purchasePrice, "selling_price": sellingPrice, "reward": reward, "profit": profit,
		"sim_cost": req.SimCost, "added_credit_cost": req.AddedCreditCost,
		"activation_reward": req.ActivationReward, "monthly_commission": req.MonthlyCommission,
		"sold_by_name": strings.TrimSpace(req.SoldByName), "created_at": createdAt,
	})
}

// nullableString returns nil for empty strings so Postgres stores NULL
// (keeps the partial unique index on client_request_id sparse).
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// ListSales returns the caller's SIM sales ledger (soft-deleted rows hidden).
// GET /api/sales
func (h *SimHandler) ListSales(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	rows, err := h.Pool.Query(c.Context(),
		`SELECT id, sim_id, barcode, operator, purchase_price, selling_price, reward, profit, notes,
		        sim_cost, added_credit_cost, activation_reward, monthly_commission, sold_by_name, created_at
		 FROM sim_sales WHERE user_id = $1 AND is_deleted = FALSE ORDER BY created_at DESC LIMIT 500`, uid)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer rows.Close()

	type sale struct {
		ID                int64     `json:"id"`
		SimID             *int64    `json:"sim_id"`
		Barcode           string    `json:"barcode"`
		Operator          string    `json:"operator"`
		PurchasePrice     float64   `json:"purchase_price"`
		SellingPrice      float64   `json:"selling_price"`
		Reward            float64   `json:"reward"`
		Profit            float64   `json:"profit"`
		Notes             string    `json:"notes"`
		SimCost           float64   `json:"sim_cost"`
		AddedCreditCost   float64   `json:"added_credit_cost"`
		ActivationReward  float64   `json:"activation_reward"`
		MonthlyCommission float64   `json:"monthly_commission"`
		SoldByName        string    `json:"sold_by_name"`
		CreatedAt         time.Time `json:"created_at"`
	}
	sales := make([]sale, 0)
	for rows.Next() {
		var s sale
		if err := rows.Scan(&s.ID, &s.SimID, &s.Barcode, &s.Operator, &s.PurchasePrice, &s.SellingPrice, &s.Reward, &s.Profit, &s.Notes,
			&s.SimCost, &s.AddedCreditCost, &s.ActivationReward, &s.MonthlyCommission, &s.SoldByName, &s.CreatedAt); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "scan error"})
		}
		sales = append(sales, s)
	}
	return c.JSON(sales)
}

// DeleteSim permanently deletes an inventory row owned by the caller.
// The path parameter accepts a numeric id OR the barcode string.
// DELETE /api/sims/:idOrBarcode
func (h *SimHandler) DeleteSim(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	raw := strings.TrimSpace(c.Params("id"))
	if raw == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	res, err := h.Pool.Exec(c.Context(),
		`DELETE FROM sim_inventory WHERE user_id = $1 AND (id::text = $2 OR barcode = $2)`, uid, raw)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if res.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "SIM not found"})
	}
	if h.Hub != nil {
		h.Hub.Broadcast(uid, "inventory", fiber.Map{"action": "deleted", "target": raw})
	}
	return c.JSON(fiber.Map{"success": true})
}

// DeleteSale soft-deletes a sale record owned by the caller (is_deleted flag,
// preserving the immutable ledger). The path parameter accepts a numeric id OR
// the barcode string.
// DELETE /api/sales/:idOrBarcode
func (h *SimHandler) DeleteSale(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	raw := strings.TrimSpace(c.Params("id"))
	if raw == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	res, err := h.Pool.Exec(c.Context(),
		`UPDATE sim_sales SET is_deleted = TRUE WHERE user_id = $1 AND is_deleted = FALSE AND (id::text = $2 OR barcode = $2)`, uid, raw)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if res.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "sale not found"})
	}
	if h.Hub != nil {
		h.Hub.Broadcast(uid, "sales", fiber.Map{"action": "deleted", "target": raw})
	}
	return c.JSON(fiber.Map{"success": true})
}

// AddCreditSale records a recharge sale; profit is computed by the DB.
// Idempotent on client_request_id when supplied.
// POST /api/credit-sales
func (h *SimHandler) AddCreditSale(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	var req struct {
		Operator        string  `json:"operator"`
		Category        string  `json:"category"`
		CostPrice       float64 `json:"cost_price"`
		SellingPrice    float64 `json:"selling_price"`
		SoldByName      string  `json:"sold_by_name"`
		ClientRequestID string  `json:"client_request_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	req.ClientRequestID = strings.TrimSpace(req.ClientRequestID)
	if req.Operator == "" || req.CostPrice < 0 || req.SellingPrice < 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "operator, cost_price and selling_price are required"})
	}

	// Idempotency replay.
	if req.ClientRequestID != "" {
		var id int64
		var profit float64
		var createdAt time.Time
		err := h.Pool.QueryRow(c.Context(),
			`SELECT id, profit, created_at FROM credit_sales
			 WHERE user_id = $1 AND client_request_id = $2 AND is_deleted = FALSE`,
			uid, req.ClientRequestID,
		).Scan(&id, &profit, &createdAt)
		if err == nil {
			return c.Status(fiber.StatusCreated).JSON(fiber.Map{
				"id": id, "profit": profit, "created_at": createdAt, "already_sold": true,
			})
		}
		if err != pgx.ErrNoRows {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
		}
	}

	var id int64
	var profit float64
	var createdAt time.Time
	err := h.Pool.QueryRow(c.Context(),
		`INSERT INTO credit_sales (user_id, operator, category, cost_price, selling_price, sold_by_name, client_request_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, profit, created_at`,
		uid, req.Operator, strings.TrimSpace(req.Category), req.CostPrice, req.SellingPrice,
		strings.TrimSpace(req.SoldByName), nullableString(req.ClientRequestID),
	).Scan(&id, &profit, &createdAt)
	if err != nil {
		if req.ClientRequestID != "" && strings.Contains(err.Error(), "uq_credit_sales_client_request") {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "sale already recorded", "already_sold": true,
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if h.Hub != nil {
		h.Hub.Broadcast(uid, "sales", fiber.Map{"action": "credit_sold", "sale_id": id, "profit": profit})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id": id, "profit": profit, "created_at": createdAt, "sold_by_name": strings.TrimSpace(req.SoldByName),
	})
}

// ListCreditSales returns the caller's credit sales ledger (soft-deleted hidden).
// GET /api/credit-sales
func (h *SimHandler) ListCreditSales(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	rows, err := h.Pool.Query(c.Context(),
		`SELECT id, operator, category, cost_price, selling_price, profit, sold_by_name, created_at
		 FROM credit_sales WHERE user_id = $1 AND is_deleted = FALSE ORDER BY created_at DESC LIMIT 500`, uid)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer rows.Close()

	type cs struct {
		ID           int64     `json:"id"`
		Operator     string    `json:"operator"`
		Category     string    `json:"category"`
		CostPrice    float64   `json:"cost_price"`
		SellingPrice float64   `json:"selling_price"`
		Profit       float64   `json:"profit"`
		SoldByName   string    `json:"sold_by_name"`
		CreatedAt    time.Time `json:"created_at"`
	}
	out := make([]cs, 0)
	for rows.Next() {
		var r cs
		if err := rows.Scan(&r.ID, &r.Operator, &r.Category, &r.CostPrice, &r.SellingPrice, &r.Profit, &r.SoldByName, &r.CreatedAt); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "scan error"})
		}
		out = append(out, r)
	}
	return c.JSON(out)
}

// DeleteCreditSale soft-deletes a credit sale owned by the caller.
// DELETE /api/credit-sales/:id
func (h *SimHandler) DeleteCreditSale(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	raw := strings.TrimSpace(c.Params("id"))
	if raw == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	res, err := h.Pool.Exec(c.Context(),
		`UPDATE credit_sales SET is_deleted = TRUE WHERE user_id = $1 AND is_deleted = FALSE AND id::text = $2`, uid, raw)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	if res.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "credit sale not found"})
	}
	if h.Hub != nil {
		h.Hub.Broadcast(uid, "sales", fiber.Map{"action": "credit_deleted", "target": raw})
	}
	return c.JSON(fiber.Map{"success": true})
}
