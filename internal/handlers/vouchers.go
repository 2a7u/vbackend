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

// VouchersHandler owns the encrypted digital voucher vault endpoints.
type VouchersHandler struct {
	Pool *pgxpool.Pool
	Hub  *WSHub
}

type rawPinItem struct {
	PIN    string `json:"pin"`
	Serial string `json:"serial"`
}

type uploadVouchersRequest struct {
	BrandID      string       `json:"brand_id"`
	VoucherID    string       `json:"voucher_id"`
	RawPins      []rawPinItem `json:"raw_pins"`
	CostPrice    float64      `json:"cost_price"`
	SellingPrice float64      `json:"selling_price"`
}

// UploadVouchers handles bulk uploading of voucher codes to the vault.
// POST /api/vouchers/upload
func (h *VouchersHandler) UploadVouchers(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	cl := middleware.ClaimsFrom(c)

	var req uploadVouchersRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "بيانات غير صالحة",
		})
	}

	if len(req.RawPins) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "قائمة الأكواد فارغة",
		})
	}

	tx, err := h.Pool.Begin(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}
	defer tx.Rollback(c.Context())

	inserted := 0
	for _, p := range req.RawPins {
		pin := strings.TrimSpace(p.PIN)
		serial := strings.TrimSpace(p.Serial)
		if pin == "" {
			continue
		}
		if serial == "" {
			serial = fmt.Sprintf("SN%s%04d", time.Now().Format("20060102150405"), inserted+1)
		}

		_, err := tx.Exec(c.Context(),
			`INSERT INTO vouchers_vault (user_id, brand_id, voucher_id, pin, serial_number, cost_price, selling_price, is_sold)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, FALSE)`,
			uid, req.BrandID, req.VoucherID, pin, serial, req.CostPrice, req.SellingPrice,
		)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"error":   "فشل حفظ الأكواد: " + err.Error(),
			})
		}
		inserted++
	}

	if err := tx.Commit(c.Context()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	writeAudit(c.Context(), h.Pool, uid, cl.Email, "VOUCHERS_UPLOAD", "vouchers_vault", req.VoucherID, map[string]any{
		"brand_id":   req.BrandID,
		"voucher_id": req.VoucherID,
		"count":      inserted,
	}, c.IP())

	return c.JSON(fiber.Map{
		"success": true,
		"count":   inserted,
	})
}

// GetVouchers returns available stock per voucher and inspector items.
// GET /api/vouchers
func (h *VouchersHandler) GetVouchers(c *fiber.Ctx) error {
	voucherID := strings.TrimSpace(c.Query("voucher_id"))
	brandID := strings.TrimSpace(c.Query("brand_id"))
	search := strings.TrimSpace(c.Query("search"))

	// 1. Stock aggregate map
	stockMap := make(map[string]int)
	rows, err := h.Pool.Query(c.Context(),
		`SELECT voucher_id, COUNT(*) 
		 FROM vouchers_vault 
		 WHERE is_sold = FALSE 
		 GROUP BY voucher_id`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var vid string
			var count int
			if err := rows.Scan(&vid, &count); err == nil {
				stockMap[vid] = count
			}
		}
	}

	// 2. Inspector items
	query := `SELECT id, brand_id, voucher_id, pin, serial_number, cost_price, selling_price, is_sold,
	                 COALESCE(sold_to_store_name, ''), sold_at, reprint_count, created_at
	          FROM vouchers_vault WHERE 1=1`
	var args []any
	argIdx := 1

	if voucherID != "" {
		query += fmt.Sprintf(" AND voucher_id = $%d", argIdx)
		args = append(args, voucherID)
		argIdx++
	}
	if brandID != "" {
		query += fmt.Sprintf(" AND brand_id = $%d", argIdx)
		args = append(args, brandID)
		argIdx++
	}
	if search != "" {
		query += fmt.Sprintf(" AND (serial_number ILIKE $%d OR pin ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}

	query += " ORDER BY id DESC LIMIT 100"

	itemRows, err := h.Pool.Query(c.Context(), query, args...)
	var items []fiber.Map
	if err == nil {
		defer itemRows.Close()
		for itemRows.Next() {
			var (
				id            int64
				bId, vId      string
				pin, sn       string
				cost, sell    float64
				isSold        bool
				soldStore     string
				soldAt        *time.Time
				reprintCount  int
				createdAt     time.Time
			)
			if err := itemRows.Scan(&id, &bId, &vId, &pin, &sn, &cost, &sell, &isSold, &soldStore, &soldAt, &reprintCount, &createdAt); err == nil {
				// Mask all but last 4 chars of PIN for security
				maskedPIN := pin
				if len(pin) > 4 {
					maskedPIN = strings.Repeat("*", len(pin)-4) + pin[len(pin)-4:]
				}
				items = append(items, fiber.Map{
					"id":                 id,
					"brand_id":           bId,
					"voucher_id":         vId,
					"pin":                pin,
					"pin_masked":         maskedPIN,
					"serial_number":      sn,
					"cost_price":         cost,
					"selling_price":      sell,
					"is_sold":            isSold,
					"sold_to_store_name": soldStore,
					"sold_at":            soldAt,
					"reprint_count":      reprintCount,
					"created_at":         createdAt,
				})
			}
		}
	}
	if items == nil {
		items = []fiber.Map{}
	}

	return c.JSON(fiber.Map{
		"success": true,
		"stock":   stockMap,
		"items":   items,
	})
}

type dispenseVoucherRequest struct {
	VoucherID     string  `json:"voucher_id"`
	BrandID       string  `json:"brand_id"`
	Quantity      int     `json:"quantity"`
	SellingPrice  float64 `json:"selling_price"`
	CostPrice     float64 `json:"cost_price"`
	VoucherName   string  `json:"voucher_name"`
	CustomerName  string  `json:"customer_name"`
	CustomerPhone string  `json:"customer_phone"`
}

// DispenseVoucher atomically claims and sells voucher PINs.
// POST /api/vouchers/dispense
func (h *VouchersHandler) DispenseVoucher(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	cl := middleware.ClaimsFrom(c)

	var req dispenseVoucherRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "بيانات غير صالحة",
		})
	}

	if req.Quantity <= 0 {
		req.Quantity = 1
	}

	tx, err := h.Pool.Begin(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}
	defer tx.Rollback(c.Context())

	// Select available PINs with SKIP LOCKED
	rows, err := tx.Query(c.Context(),
		`SELECT id, pin, serial_number, cost_price, selling_price
		 FROM vouchers_vault
		 WHERE voucher_id = $1 AND is_sold = FALSE
		 ORDER BY id ASC
		 LIMIT $2
		 FOR UPDATE SKIP LOCKED`,
		req.VoucherID, req.Quantity,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "خطأ في قراءة الأكواد: " + err.Error(),
		})
	}

	type claimedPin struct {
		ID     int64
		PIN    string
		Serial string
		Cost   float64
		Sell   float64
	}
	var claimed []claimedPin
	for rows.Next() {
		var cp claimedPin
		if err := rows.Scan(&cp.ID, &cp.PIN, &cp.Serial, &cp.Cost, &cp.Sell); err == nil {
			claimed = append(claimed, cp)
		}
	}
	rows.Close()

	if len(claimed) < req.Quantity {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"success":                 false,
			"error":                   "نفد المخزون من هذه الفئة أو الرصيد غير كافٍ في الخزنة",
			"is_insufficient_balance": false,
			"available":               len(claimed),
		})
	}

	var ids []int64
	var resultPins []fiber.Map
	for _, cp := range claimed {
		ids = append(ids, cp.ID)
		resultPins = append(resultPins, fiber.Map{
			"id":            cp.ID,
			"pin":           cp.PIN,
			"serial":        cp.Serial,
			"serial_number": cp.Serial,
			"cost_price":    cp.Cost,
			"selling_price": cp.Sell,
		})
	}

	storeName := "نقطة بيع"
	if cl != nil && cl.Email != "" {
		storeName = cl.Email
	}

	// Update rows to sold
	batch := &pgx.Batch{}
	for _, id := range ids {
		batch.Queue(
			`UPDATE vouchers_vault
			 SET is_sold = TRUE, sold_to_user_id = $1, sold_to_store_name = $2, sold_at = now(), sold_price = $3
			 WHERE id = $4`,
			uid, storeName, req.SellingPrice, id,
		)
	}
	br := tx.SendBatch(c.Context(), batch)
	for range ids {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"error":   "فشل تحديث حالة الكود: " + err.Error(),
			})
		}
	}
	br.Close()

	if err := tx.Commit(c.Context()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	writeAudit(c.Context(), h.Pool, uid, cl.Email, "VOUCHER_DISPENSE", "vouchers_vault", req.VoucherID, map[string]any{
		"brand_id":    req.BrandID,
		"voucher_id":  req.VoucherID,
		"quantity":    req.Quantity,
		"pins_count":  len(resultPins),
		"voucher_name": req.VoucherName,
	}, c.IP())

	return c.JSON(fiber.Map{
		"success": true,
		"pins":    resultPins,
		"count":   len(resultPins),
	})
}
