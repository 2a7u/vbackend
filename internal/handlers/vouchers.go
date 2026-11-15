package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/fraud"
	"venus_backend/internal/hierarchy"
	"venus_backend/internal/ledger"
	"venus_backend/internal/middleware"
	"venus_backend/internal/pricing"
	"venus_backend/internal/vault"
)

// VouchersHandler owns the encrypted digital voucher vault endpoints.
type VouchersHandler struct {
	Pool         *pgxpool.Pool
	Hub          *WSHub
	LedgerSvc    *ledger.Service
	FraudEngine  *fraud.Engine
	VaultMgr     *vault.VaultManager
	PricingSvc   *pricing.Service
	HierarchySvc *hierarchy.Service
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

// UploadVouchers handles bulk uploading of voucher codes to the encrypted vault with duplicate prevention.
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

	vm := h.VaultMgr
	if vm == nil {
		vm = vault.Default()
	}

	tx, err := h.Pool.Begin(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}
	defer tx.Rollback(c.Context())

	succeeded := 0
	invalid := 0
	var duplicates []string

	for idx, p := range req.RawPins {
		pin := strings.TrimSpace(p.PIN)
		serial := strings.TrimSpace(p.Serial)
		if pin == "" {
			invalid++
			continue
		}
		if serial == "" {
			serial = fmt.Sprintf("SN%s%04d", time.Now().Format("20060102150405"), idx+1)
		}

		// 1. Permanent global duplicate check via HMAC-SHA256
		hmacHash := vm.ComputeHMAC(pin)
		var exists bool
		_ = tx.QueryRow(c.Context(),
			`SELECT EXISTS(SELECT 1 FROM vouchers_vault WHERE pin_hmac = $1)`,
			hmacHash,
		).Scan(&exists)

		if exists {
			duplicates = append(duplicates, serial)
			continue
		}

		// 2. Encrypt PIN using AES-256-GCM
		encHex, nonceHex, keyVer, encErr := vm.EncryptPIN(pin)
		if encErr != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"error":   "فشل تشفير البطاقة رقمياً: " + encErr.Error(),
			})
		}

		// Masked PIN for safe display (never store plaintext)
		maskedPIN := pin
		if len(pin) > 4 {
			maskedPIN = strings.Repeat("*", len(pin)-4) + pin[len(pin)-4:]
		}

		_, err := tx.Exec(c.Context(),
			`INSERT INTO vouchers_vault (user_id, brand_id, voucher_id, pin, pin_encrypted, nonce, key_version, pin_hmac, serial_number, cost_price, selling_price, is_sold)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, FALSE)`,
			uid, req.BrandID, req.VoucherID, maskedPIN, encHex, nonceHex, keyVer, hmacHash, serial, req.CostPrice, req.SellingPrice,
		)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"error":   "فشل حفظ الأكواد في الخزنة: " + err.Error(),
			})
		}
		succeeded++
	}

	if err := tx.Commit(c.Context()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	writeAudit(c.Context(), h.Pool, uid, cl.Email, "VOUCHERS_UPLOAD", "vouchers_vault", req.VoucherID, map[string]any{
		"brand_id":         req.BrandID,
		"voucher_id":       req.VoucherID,
		"succeeded_count":  succeeded,
		"duplicates_count": len(duplicates),
		"invalid_count":    invalid,
	}, c.IP())

	if succeeded == 0 && len(duplicates) > 0 {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"success":           false,
			"error":             fmt.Sprintf("تم رفض الرفع: تم رصد %d بطاقة مكررة مسجلة مسبقاً في الخزنة", len(duplicates)),
			"succeeded":         0,
			"duplicates":        len(duplicates),
			"duplicate_serials": duplicates,
			"invalid":           invalid,
		})
	}

	return c.JSON(fiber.Map{
		"success":           true,
		"succeeded":         succeeded,
		"duplicates":        len(duplicates),
		"duplicate_serials": duplicates,
		"invalid":           invalid,
		"count":             succeeded,
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

// DispenseVoucher atomically claims and sells voucher PINs with dynamic pricing and commission processing.
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

	vm := h.VaultMgr
	if vm == nil {
		vm = vault.Default()
	}

	tx, err := h.Pool.Begin(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}
	defer tx.Rollback(c.Context())

	// 1. Resolve unit selling price for this specific subscriber strictly inside the transaction
	// (Resolution order: Override -> Tier -> Catalog Default; NO stale in-memory cache used for money)
	unitSellingPrice := int64(req.SellingPrice)
	if h.PricingSvc != nil && uid > 0 {
		resolved, pErr := h.PricingSvc.ResolvePriceTx(c.Context(), tx, uid, req.VoucherID, unitSellingPrice)
		if pErr == nil && resolved.Price > 0 {
			unitSellingPrice = resolved.Price
		}
	}

	// 2. Select available PINs with SKIP LOCKED
	rows, err := tx.Query(c.Context(),
		`SELECT id, pin, pin_encrypted, nonce, key_version, serial_number, cost_price, selling_price
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
			"error":   "خطأ في قراءة الأكواد من الخزنة: " + err.Error(),
		})
	}

	type claimedPin struct {
		ID           int64
		LegacyPIN    string
		EncryptedPIN string
		Nonce        string
		KeyVer       int
		Serial       string
		Cost         float64
		Sell         float64
	}
	var claimed []claimedPin
	for rows.Next() {
		var cp claimedPin
		if err := rows.Scan(&cp.ID, &cp.LegacyPIN, &cp.EncryptedPIN, &cp.Nonce, &cp.KeyVer, &cp.Serial, &cp.Cost, &cp.Sell); err == nil {
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
	var auditSerials []string
	var totalNetProfit int64

	for _, cp := range claimed {
		ids = append(ids, cp.ID)
		auditSerials = append(auditSerials, cp.Serial)

		// Decrypt PIN at actual sale time only
		var revealedPIN string
		if cp.EncryptedPIN != "" && cp.Nonce != "" {
			decrypted, dErr := vm.DecryptPIN(cp.EncryptedPIN, cp.Nonce, cp.KeyVer)
			if dErr == nil {
				revealedPIN = decrypted
			} else {
				revealedPIN = cp.LegacyPIN
			}
		} else {
			revealedPIN = cp.LegacyPIN
		}

		unitCost := int64(cp.Cost)
		itemProfit := pricing.ComputeProfit(unitCost, unitSellingPrice)
		totalNetProfit += itemProfit

		resultPins = append(resultPins, fiber.Map{
			"id":            cp.ID,
			"pin":           revealedPIN,
			"serial":        cp.Serial,
			"serial_number": cp.Serial,
			"cost_price":    cp.Cost,
			"selling_price": unitSellingPrice,
		})
	}

	storeName := "نقطة بيع"
	if cl != nil && cl.Email != "" {
		storeName = cl.Email
	}

	totalPrice := unitSellingPrice * int64(len(claimed))
	if totalPrice > 0 {
		// Risk & Fraud Check
		if h.FraudEngine != nil {
			eval, fErr := h.FraudEngine.Evaluate(c.Context(), tx, fraud.CheckRequest{
				UserID:    uid,
				AmountIQD: totalPrice,
				TxType:    "voucher_dispense",
				IPAddress: c.IP(),
			})
			if fErr == nil && eval != nil && !eval.Allowed {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"success": false,
					"error":   "تم تعليق سحب البطاقات مؤقتاً: " + eval.Reason,
				})
			}
		}

		// Double-Entry Ledger deduction (checks balance + credit limit)
		if h.LedgerSvc != nil {
			idempotencyKey := c.Get("Idempotency-Key")
			_, lErr := h.LedgerSvc.RecordPurchase(c.Context(), tx, uid, ledger.SystemEmailVouchers, totalPrice, "voucher_dispense", req.VoucherID, idempotencyKey)
			if lErr != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"success":                 false,
					"error":                   lErr.Error(),
					"is_insufficient_balance": true,
				})
			}
		}

		_, _ = tx.Exec(c.Context(),
			`INSERT INTO wallet_ledger (user_id, amount, kind, provider, status, note, completed_at)
			 VALUES ($1, $2, 'payment', 'vouchers', 'success', $3, now())`,
			uid, totalPrice, "شراء بطاقات رقمية: "+req.VoucherID,
		)
	}

	// 3. Process agent hierarchy commission in the SAME transaction
	if h.HierarchySvc != nil && uid > 0 && totalPrice > 0 {
		_, _ = h.HierarchySvc.ProcessSaleCommission(c.Context(), tx, uid, req.VoucherID, totalPrice, fmt.Sprintf("%d", ids[0]))
	}

	// 4. Update rows to sold, saving actual sell_price and computed net_profit
	batch := &pgx.Batch{}
	for _, cp := range claimed {
		unitCost := int64(cp.Cost)
		itemProfit := pricing.ComputeProfit(unitCost, unitSellingPrice)
		batch.Queue(
			`UPDATE vouchers_vault
			 SET is_sold = TRUE, sold_to_user_id = $1, sold_to_store_name = $2, sold_at = now(), sold_price = $3, net_profit = $4
			 WHERE id = $5`,
			uid, storeName, unitSellingPrice, itemProfit, cp.ID,
		)
	}
	br := tx.SendBatch(c.Context(), batch)
	for range ids {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"error":   "فشل تحديث حالة الكود في الخزنة: " + err.Error(),
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

	// Audit PIN reveal WITHOUT logging plaintext PIN
	writeAudit(c.Context(), h.Pool, uid, cl.Email, "VOUCHER_PIN_REVEAL", "vouchers_vault", req.VoucherID, map[string]any{
		"brand_id":     req.BrandID,
		"voucher_id":   req.VoucherID,
		"quantity":     len(resultPins),
		"serials":      auditSerials,
		"total_price":  totalPrice,
		"net_profit":   totalNetProfit,
		"voucher_name": req.VoucherName,
	}, c.IP())

	if totalPrice > 0 && h.Hub != nil {
		var newBal float64
		_ = h.Pool.QueryRow(c.Context(), `SELECT wallet_balance FROM users WHERE id = $1`, uid).Scan(&newBal)
		h.Hub.Broadcast(uid, "wallet_updated", fiber.Map{
			"user_id":        uid,
			"wallet_balance": newBal,
			"amount":         -totalPrice,
			"note":           "شراء بطاقات رقمية: " + req.VoucherID,
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"pins":    resultPins,
		"count":   len(resultPins),
	})
}
