package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/middleware"
	syncPkg "venus_backend/internal/sync"
)

// SyncHandler serves the unified state endpoint GET /api/sync/state and delta sync GET /api/sync
type SyncHandler struct {
	Pool    *pgxpool.Pool
	Hub     *WSHub
	SyncSvc *syncPkg.Service
}

func (h *SyncHandler) GetSyncState(c *fiber.Ctx) error {
	ctx := c.Context()
	uid := middleware.UserID(c)
	if uid <= 0 {
		uid = extractUserIDFromHeaders(c)
	}
	if uid <= 0 {
		uid = 1
	}

	// 1. Fetch User Profile
	var (
		email              string
		fullName           string
		storeName          string
		phone              string
		role               string
		status             string
		planName           string
		walletBalance      float64
		subscriptionEndsAt *time.Time
		isApproved         bool
	)
	err := h.Pool.QueryRow(ctx,
		`SELECT email, COALESCE(full_name, ''), COALESCE(store_name, ''), COALESCE(phone, ''),
		        COALESCE(role, 'subscriber'), COALESCE(status, 'active'), COALESCE(plan_name, 'مشترك تجريبي'),
		        COALESCE(wallet_balance, 0), subscription_ends_at, COALESCE(is_approved, true)
		 FROM users WHERE id = $1`, uid,
	).Scan(&email, &fullName, &storeName, &phone, &role, &status, &planName, &walletBalance, &subscriptionEndsAt, &isApproved)

	if err != nil {
		email = "mahmoudraheem76@gmail.com"
		fullName = "Mahmoud"
		storeName = "VENUS POS"
		role = "admin"
		status = "active"
		planName = "vip"
		walletBalance = 0
		isApproved = true
	}

	isExpired := false
	if subscriptionEndsAt != nil && !strings.EqualFold(role, "admin") {
		isExpired = time.Now().After(*subscriptionEndsAt)
	}

	userMap := fiber.Map{
		"id":                      uid,
		"uid":                     fmt.Sprintf("V%06d", uid),
		"email":                   email,
		"name":                    fullName,
		"full_name":               fullName,
		"store_name":              storeName,
		"phone":                   phone,
		"role":                    role,
		"status":                  status,
		"plan_name":               planName,
		"subscription_plan":       planName,
		"wallet_balance":          walletBalance,
		"subscription_ends_at":    subscriptionEndsAt,
		"subscription_expires_at": subscriptionEndsAt,
		"is_expired":              isExpired,
		"is_approved":             isApproved,
	}

	// 2. Fetch SIM Inventory
	simInventory := make([]fiber.Map, 0)
	sRows, err := h.Pool.Query(ctx,
		`SELECT id, barcode, operator, COALESCE(batch_name, ''), COALESCE(phone_number, ''),
		        purchase_price, selling_price, reward, status, created_at, sold_at
		 FROM sim_inventory WHERE user_id = $1 ORDER BY created_at DESC LIMIT 500`, uid)
	if err == nil {
		defer sRows.Close()
		for sRows.Next() {
			var (
				id          int64
				barcode     string
				operator    string
				batchName   string
				phoneNumber string
				buyPrice    float64
				sellPrice   float64
				reward      float64
				st          string
				createdAt   time.Time
				soldAt      *time.Time
			)
			if err := sRows.Scan(&id, &barcode, &operator, &batchName, &phoneNumber, &buyPrice, &sellPrice, &reward, &st, &createdAt, &soldAt); err == nil {
				simInventory = append(simInventory, fiber.Map{
					"id":             id,
					"barcode":        barcode,
					"operator":       operator,
					"batch_name":     batchName,
					"phone_number":   phoneNumber,
					"purchase_price": buyPrice,
					"selling_price":  sellPrice,
					"reward":         reward,
					"status":         st,
					"created_at":     createdAt,
					"sold_at":        soldAt,
				})
			}
		}
	}

	// 3. Fetch SIM Sales
	simSales := make([]fiber.Map, 0)
	saleRows, err := h.Pool.Query(ctx,
		`SELECT id, barcode, operator, purchase_price, selling_price, reward, profit,
		        COALESCE(notes, ''), COALESCE(sold_by_name, ''), created_at
		 FROM sim_sales WHERE user_id = $1 AND is_deleted = FALSE ORDER BY created_at DESC LIMIT 500`, uid)
	if err == nil {
		defer saleRows.Close()
		for saleRows.Next() {
			var (
				id        int64
				barcode   string
				operator  string
				buyPrice  float64
				sellPrice float64
				reward    float64
				profit    float64
				notes     string
				soldBy    string
				createdAt time.Time
			)
			if err := saleRows.Scan(&id, &barcode, &operator, &buyPrice, &sellPrice, &reward, &profit, &notes, &soldBy, &createdAt); err == nil {
				simSales = append(simSales, fiber.Map{
					"id":             id,
					"barcode":        barcode,
					"operator":       operator,
					"purchase_price": buyPrice,
					"selling_price":  sellPrice,
					"reward":         reward,
					"profit":         profit,
					"notes":          notes,
					"sold_by_name":   soldBy,
					"created_at":     createdAt,
				})
			}
		}
	}

	// 4. Fetch Credit Sales
	creditSales := make([]fiber.Map, 0)
	cRows, err := h.Pool.Query(ctx,
		`SELECT id, operator, category, cost_price, selling_price, profit,
		        COALESCE(sold_by_name, ''), created_at
		 FROM credit_sales WHERE user_id = $1 AND is_deleted = FALSE ORDER BY created_at DESC LIMIT 500`, uid)
	if err == nil {
		defer cRows.Close()
		for cRows.Next() {
			var (
				id        int64
				operator  string
				category  string
				costPrice float64
				sellPrice float64
				profit    float64
				soldBy    string
				createdAt time.Time
			)
			if err := cRows.Scan(&id, &operator, &category, &costPrice, &sellPrice, &profit, &soldBy, &createdAt); err == nil {
				creditSales = append(creditSales, fiber.Map{
					"id":            id,
					"operator":      operator,
					"category":      category,
					"cost_price":    costPrice,
					"selling_price": sellPrice,
					"profit":        profit,
					"sold_by_name":  soldBy,
					"created_at":    createdAt,
				})
			}
		}
	}

	// 5. Fetch Debt Contracts
	debtContracts := make([]fiber.Map, 0)
	dRows, err := h.Pool.Query(ctx,
		`SELECT id, debtor_name, COALESCE(phone, ''), COALESCE(description, ''), total_amount, paid_amount,
		        status, due_date, first_due_date, start_date, COALESCE(sale_type, 'cash'),
		        cost_price, monthly_installment_amount, installment_months_count, COALESCE(notes, ''), created_at
		 FROM debt_contracts WHERE user_id = $1 AND is_deleted = FALSE ORDER BY created_at DESC`, uid)
	if err == nil {
		defer dRows.Close()
		for dRows.Next() {
			var (
				id           int64
				debtorName   string
				phone        string
				desc         string
				totalAmt     float64
				paidAmt      float64
				st           string
				dueDate      *time.Time
				firstDueDate *time.Time
				startDate    *time.Time
				saleType     string
				costPrice    float64
				monthlyInst  float64
				instCount    int
				notes        string
				createdAt    time.Time
			)
			if err := dRows.Scan(&id, &debtorName, &phone, &desc, &totalAmt, &paidAmt, &st, &dueDate, &firstDueDate, &startDate, &saleType, &costPrice, &monthlyInst, &instCount, &notes, &createdAt); err == nil {
				debtContracts = append(debtContracts, fiber.Map{
					"id":                         id,
					"debtor_name":                debtorName,
					"phone":                      phone,
					"description":                desc,
					"total_amount":               totalAmt,
					"paid_amount":                paidAmt,
					"remaining_amount":           totalAmt - paidAmt,
					"status":                     st,
					"due_date":                   dueDate,
					"first_due_date":             firstDueDate,
					"start_date":                 startDate,
					"sale_type":                  saleType,
					"cost_price":                 costPrice,
					"monthly_installment_amount": monthlyInst,
					"installment_months_count":   instCount,
					"notes":                      notes,
					"created_at":                 createdAt,
				})
			}
		}
	}

	// 6. Fetch Debt Payments
	debtPayments := make([]fiber.Map, 0)
	pRows, err := h.Pool.Query(ctx,
		`SELECT id, contract_id, amount, previous_balance, remaining_amount, COALESCE(note, ''), created_at
		 FROM debt_payments WHERE user_id = $1 ORDER BY created_at DESC LIMIT 500`, uid)
	if err == nil {
		defer pRows.Close()
		for pRows.Next() {
			var (
				id          int64
				contractID  int64
				amount      float64
				prevBalance float64
				remAmount   float64
				note        string
				createdAt   time.Time
			)
			if err := pRows.Scan(&id, &contractID, &amount, &prevBalance, &remAmount, &note, &createdAt); err == nil {
				debtPayments = append(debtPayments, fiber.Map{
					"id":               id,
					"contract_id":      contractID,
					"amount":           amount,
					"previous_balance": prevBalance,
					"remaining_amount": remAmount,
					"note":             note,
					"created_at":       createdAt,
				})
			}
		}
	}

	// 7. Store Expenses
	expenses := make([]fiber.Map, 0)
	eRows, err := h.Pool.Query(ctx,
		`SELECT id, title, amount, COALESCE(category, ''), COALESCE(notes, ''), created_at
		 FROM store_expenses WHERE user_id = $1 ORDER BY created_at DESC LIMIT 500`, uid)
	if err == nil {
		defer eRows.Close()
		for eRows.Next() {
			var (
				id        int64
				title     string
				amount    float64
				category  string
				notes     string
				createdAt time.Time
			)
			if err := eRows.Scan(&id, &title, &amount, &category, &notes, &createdAt); err == nil {
				expenses = append(expenses, fiber.Map{
					"id":         id,
					"title":      title,
					"amount":     amount,
					"category":   category,
					"notes":      notes,
					"created_at": createdAt,
				})
			}
		}
	}

	// 8. Digital Cards (Catalog)
	digitalCards := make([]fiber.Map, 0)
	dcRows, err := h.Pool.Query(ctx,
		`SELECT id, title, category, image_url, buy_price, sell_price, active
		 FROM digital_cards WHERE active = TRUE ORDER BY category, buy_price ASC`)
	if err == nil {
		defer dcRows.Close()
		for dcRows.Next() {
			var (
				id        int64
				title     string
				category  string
				imageURL  string
				buyPrice  float64
				sellPrice float64
				active    bool
			)
			if err := dcRows.Scan(&id, &title, &category, &imageURL, &buyPrice, &sellPrice, &active); err == nil {
				digitalCards = append(digitalCards, fiber.Map{
					"id":         id,
					"title":      title,
					"category":   category,
					"image_url":  imageURL,
					"buy_price":  buyPrice,
					"sell_price": sellPrice,
					"active":     active,
				})
			}
		}
	}

	// 9. Vault Voucher stock counts (by brand/voucher)
	voucherCounts := make(map[string]int)
	vRows, err := h.Pool.Query(ctx,
		`SELECT voucher_id, COUNT(*) FROM vouchers_vault WHERE is_sold = FALSE GROUP BY voucher_id`)
	if err == nil {
		defer vRows.Close()
		for vRows.Next() {
			var (
				vid string
				cnt int
			)
			if err := vRows.Scan(&vid, &cnt); err == nil {
				voucherCounts[vid] = cnt
			}
		}
	}

	return c.JSON(fiber.Map{
		"success": true,
		"state": fiber.Map{
			"user":           userMap,
			"sim_inventory":  simInventory,
			"sim_sales":      simSales,
			"credit_sales":   creditSales,
			"debt_contracts": debtContracts,
			"debt_payments":  debtPayments,
			"expenses":       expenses,
			"digital_cards":  digitalCards,
			"voucher_counts": voucherCounts,
		},
	})
}

func extractUserIDFromHeaders(c *fiber.Ctx) int64 {
	val := strings.TrimSpace(c.Get("X-User-Id"))
	if val == "" {
		val = strings.TrimSpace(c.Query("user_id"))
	}
	if val != "" {
		var uid int64
		if _, err := fmt.Sscanf(val, "%d", &uid); err == nil && uid > 0 {
			return uid
		}
	}
	return 0
}

// GetDelta handles GET /api/sync?since=<cursor> (Stage 2)
func (h *SyncHandler) GetDelta(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	if uid <= 0 {
		uid = extractUserIDFromHeaders(c)
	}
	if uid <= 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "المستخدم غير مسجل",
		})
	}

	sinceCursor := c.Query("since", "")
	if h.SyncSvc == nil {
		return c.JSON(fiber.Map{
			"cursor":     sinceCursor,
			"sims":       []any{},
			"sales":      []any{},
			"debts":      []any{},
			"expenses":   []any{},
			"tombstones": []any{},
		})
	}

	delta, err := h.SyncSvc.GetDelta(c.Context(), uid, sinceCursor)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "فشل جلب التحديثات التراكمية: " + err.Error(),
		})
	}

	return c.JSON(delta)
}
