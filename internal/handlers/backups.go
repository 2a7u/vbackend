package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BackupsHandler implements admin cloud backup & restore of a subscriber's
// full data snapshot. All reads/writes are scoped to the target user_id.
type BackupsHandler struct {
	Pool *pgxpool.Pool
	Hub  *WSHub
}

// backupTables maps table name -> ordered column list used for snapshot and restore.
var backupTables = []struct {
	Name    string
	Columns string
}{
	{"sim_inventory", "id, barcode, operator, batch_name, phone_number, purchase_price, selling_price, reward, status, created_at, sold_at"},
	{"sim_sales", "id, sim_id, barcode, operator, purchase_price, selling_price, reward, profit, notes, sim_cost, added_credit_cost, activation_reward, monthly_commission, sold_by_name, client_request_id, is_deleted, created_at"},
	{"credit_sales", "id, operator, category, cost_price, selling_price, sold_by_name, client_request_id, is_deleted, created_at"},
	{"debt_contracts", "id, debtor_name, phone, description, sale_type, cost_price, total_amount, paid_amount, status, due_date, monthly_installment_amount, installment_months_count, start_date, first_due_date, notes, last_whatsapp_reminder_at, is_deleted, created_at"},
	{"debt_payments", "id, contract_id, amount, previous_balance, remaining_amount, note, created_at"},
	{"store_expenses", "id, title, amount, category, notes, created_at"},
	{"support_tickets", "id, store_name, contact_phone, subject, message, priority, status, reply, replied_at, created_at"},
	{"announcements", "id, title, body, image_url, target_audience, is_active, created_at, updated_at"},
}

// Backup exports a full JSON snapshot of every table for the target subscriber
// and stores it in subscriber_backups.
// GET /api/admin/users/:id/backup
func (h *BackupsHandler) Backup(c *fiber.Ctx) error {
	uid, err := paramID(c)
	if err != nil || uid <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}

	snapshot := make(map[string]json.RawMessage)
	for _, t := range backupTables {
		rows, err := h.Pool.Query(c.Context(),
			fmt.Sprintf(`SELECT row_to_json(r) FROM (SELECT %s FROM %s WHERE user_id = $1) r`, t.Columns, t.Name), uid)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "backup query failed: " + t.Name})
		}
		list := make([]json.RawMessage, 0)
		for rows.Next() {
			var raw json.RawMessage
			if err := rows.Scan(&raw); err != nil {
				rows.Close()
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "scan error"})
			}
			list = append(list, raw)
		}
		rows.Close()
		b, _ := json.Marshal(list)
		snapshot[t.Name] = b
	}

	payload, err := json.Marshal(snapshot)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "marshal error"})
	}

	var backupID int64
	err = h.Pool.QueryRow(c.Context(),
		`INSERT INTO subscriber_backups (user_id, snapshot) VALUES ($1, $2) RETURNING id`, uid, payload,
	).Scan(&backupID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not store backup"})
	}

	if h.Hub != nil {
		h.Hub.Broadcast(uid, "admin", fiber.Map{"action": "backup_created", "backup_id": backupID})
	}
	return c.JSON(fiber.Map{"success": true, "backup_id": backupID, "user_id": uid, "snapshot": snapshot})
}

// Restore wipes the target subscriber's business rows and re-inserts them from
// a JSON snapshot (either the latest stored backup or a body-provided one),
// all inside a single transaction.
// POST /api/admin/users/:id/restore
func (h *BackupsHandler) Restore(c *fiber.Ctx) error {
	uid, err := paramID(c)
	if err != nil || uid <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}

	// Resolve snapshot: explicit body {"snapshot": {...}} wins, else latest backup row.
	var payload []byte
	var body struct {
		Snapshot json.RawMessage `json:"snapshot"`
	}
	if err := c.BodyParser(&body); err == nil && len(body.Snapshot) > 0 {
		payload = body.Snapshot
	} else {
		err = h.Pool.QueryRow(c.Context(),
			`SELECT snapshot FROM subscriber_backups WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`, uid,
		).Scan(&payload)
		if err == pgx.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "no backup found for this user"})
		}
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not load backup"})
		}
	}

	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid snapshot JSON"})
	}

	tx, err := h.Pool.Begin(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}
	defer tx.Rollback(c.Context())

	// Delete existing rows for this subscriber (children before parents).
	deleteOrder := []string{"debt_payments", "debt_contracts", "sim_sales", "sim_inventory", "credit_sales", "store_expenses", "support_tickets", "announcements"}
	for _, t := range deleteOrder {
		if _, err := tx.Exec(c.Context(), fmt.Sprintf(`DELETE FROM %s WHERE user_id = $1`, t), uid); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "delete failed: " + t})
		}
	}

	// Re-insert each table from the snapshot, preserving original ids.
	for _, t := range backupTables {
		raw, ok := snapshot[t.Name]
		if !ok {
			continue
		}
		var rows []map[string]interface{}
		if err := json.Unmarshal(raw, &rows); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid rows for " + t.Name})
		}
		for _, row := range rows {
			cols := make([]string, 0, len(row))
			vals := make([]interface{}, 0, len(row))
			ph := make([]string, 0, len(row))
			i := 1
			for k, v := range row {
				if k == "id" || k == "user_id" {
					continue // re-assigned below
				}
				cols = append(cols, k)
				vals = append(vals, normalizeValue(k, v))
					ph = append(ph, fmt.Sprintf("$%d", i))
				i++
			}
			cols = append(cols, "user_id")
			ph = append(ph, fmt.Sprintf("$%d", i))
			vals = append(vals, uid)
			// Preserve original primary key when present so FK links survive.
			if idv, ok := row["id"]; ok {
				cols = append(cols, "id")
				ph = append(ph, fmt.Sprintf("$%d", i+1))
				vals = append(vals, normalizeValue("id", idv))
			}
			// OVERRIDING SYSTEM VALUE is required to insert explicit ids into
			// GENERATED ALWAYS AS IDENTITY columns (preserves FK relationships).
			stmt := fmt.Sprintf(`INSERT INTO %s (%s) OVERRIDING SYSTEM VALUE VALUES (%s) ON CONFLICT DO NOTHING`, t.Name, joinComma(cols), joinComma(ph))
			if _, err := tx.Exec(c.Context(), stmt, vals...); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "restore insert failed: " + t.Name + ": " + err.Error()})
			}
		}
	}

	if err := tx.Commit(c.Context()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "commit failed"})
	}

	// Re-sync identity sequences past the restored explicit ids so future
	// inserts never collide with restored rows.
	for _, t := range backupTables {
		if _, err := h.Pool.Exec(c.Context(),
			`SELECT setval(pg_get_serial_sequence($1, 'id'), COALESCE((SELECT MAX(id) FROM `+t.Name+` WHERE user_id = $2), 1), true)`,
			t.Name, uid); err != nil {
			// Non-fatal: sequence will self-heal on next conflict-free insert.
			_ = err
		}
	}

	if h.Hub != nil {
		h.Hub.Broadcast(uid, "admin", fiber.Map{"action": "restored"})
	}
	return c.JSON(fiber.Map{"success": true, "user_id": uid})
}

// timeLayouts covers the ISO-8601 shapes row_to_json emits for
// timestamptz/date columns inside a backup snapshot.
var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// normalizeValue converts JSON-decoded values into types pgx can encode into
// the target column types: whole-number floats become int64 (bigint/numeric),
// and ISO time strings for known time columns become time.Time.
func normalizeValue(key string, v interface{}) interface{} {
	switch val := v.(type) {
	case float64:
		if val == float64(int64(val)) {
			return int64(val)
		}
		return val
	case string:
		switch key {
		case "created_at", "sold_at", "replied_at", "due_date", "start_date", "first_due_date", "last_whatsapp_reminder_at", "updated_at":
			for _, layout := range timeLayouts {
				if t, err := time.Parse(layout, val); err == nil {
					return t
				}
			}
		}
		return val
	default:
		return v
	}
}
	
	func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
