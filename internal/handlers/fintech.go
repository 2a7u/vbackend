package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/fraud"
	"venus_backend/internal/hierarchy"
	"venus_backend/internal/ledger"
	"venus_backend/internal/middleware"
	"venus_backend/internal/pricing"
	"venus_backend/internal/radar"
	"venus_backend/internal/reconcile"
	"venus_backend/internal/vault"
)

type FintechHandler struct {
	Pool         *pgxpool.Pool
	Hub          *WSHub
	LedgerSvc    *ledger.Service
	FraudEngine  *fraud.Engine
	ReconcileSvc *reconcile.Service
	VaultMgr     *vault.VaultManager
	PricingSvc   *pricing.Service
	HierarchySvc *hierarchy.Service
	RadarSvc     *radar.Service
}

func NewFintechHandler(
	pool *pgxpool.Pool,
	hub *WSHub,
	ledgerSvc *ledger.Service,
	fraudEngine *fraud.Engine,
	reconcileSvc *reconcile.Service,
	vaultMgr *vault.VaultManager,
	pricingSvc *pricing.Service,
	hierarchySvc *hierarchy.Service,
	radarSvc *radar.Service,
) *FintechHandler {
	return &FintechHandler{
		Pool:         pool,
		Hub:          hub,
		LedgerSvc:    ledgerSvc,
		FraudEngine:  fraudEngine,
		ReconcileSvc: reconcileSvc,
		VaultMgr:     vaultMgr,
		PricingSvc:   pricingSvc,
		HierarchySvc: hierarchySvc,
		RadarSvc:     radarSvc,
	}
}

// GetLedgerEntries returns paginated double-entry rows.
// GET /api/admin/fintech/ledger
func (h *FintechHandler) GetLedgerEntries(c *fiber.Ctx) error {
	accountID, _ := strconv.ParseInt(c.Query("account_id"), 10, 64)
	refType := c.Query("ref_type")
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT le.id, le.tx_id, le.account_id, le.debit, le.credit, le.balance_after,
		       le.currency, le.ref_type, le.ref_id, le.idempotency_key, le.created_at,
		       le.prev_hash, le.hash, COALESCE(u.email, ''), COALESCE(u.full_name, '')
		FROM ledger_entries le
		LEFT JOIN users u ON le.account_id = u.id
		WHERE ($1 = 0 OR le.account_id = $1)
		  AND ($2 = '' OR le.ref_type = $2)
		ORDER BY le.id DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := h.Pool.Query(c.Context(), query, accountID, refType, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "query ledger: " + err.Error()})
	}
	defer rows.Close()

	var entries []ledger.LedgerEntry
	for rows.Next() {
		var e ledger.LedgerEntry
		if err := rows.Scan(
			&e.ID, &e.TxID, &e.AccountID, &e.Debit, &e.Credit, &e.BalanceAfter,
			&e.Currency, &e.RefType, &e.RefID, &e.IdempotencyKey, &e.CreatedAt,
			&e.PrevHash, &e.Hash, &e.AccountEmail, &e.AccountName,
		); err == nil {
			entries = append(entries, e)
		}
	}

	var total int64
	_ = h.Pool.QueryRow(c.Context(),
		`SELECT COUNT(*) FROM ledger_entries le WHERE ($1 = 0 OR le.account_id = $1) AND ($2 = '' OR le.ref_type = $2)`,
		accountID, refType,
	).Scan(&total)

	return c.JSON(fiber.Map{
		"entries": entries,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

// VerifyIntegrityNow triggers cryptographic verification of all accounts and txs.
// POST /api/admin/fintech/integrity/verify
func (h *FintechHandler) VerifyIntegrityNow(c *fiber.Ctx) error {
	summary, err := ledger.VerifyGlobalIntegrity(c.Context(), h.Pool, h.Hub)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "integrity verify failed: " + err.Error()})
	}
	return c.JSON(summary)
}

// ListIntegrityAlerts returns recorded tamper or balance alerts.
// GET /api/admin/fintech/integrity/alerts
func (h *FintechHandler) ListIntegrityAlerts(c *fiber.Ctx) error {
	rows, err := h.Pool.Query(c.Context(),
		`SELECT ia.id, ia.alert_type, ia.account_id, ia.tx_id, ia.entry_id,
		        ia.expected_value, ia.actual_value, ia.details, ia.status, ia.created_at,
		        COALESCE(u.email, '')
		 FROM integrity_alerts ia
		 LEFT JOIN users u ON ia.account_id = u.id
		 ORDER BY ia.id DESC
		 LIMIT 100`,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var alerts []fiber.Map
	for rows.Next() {
		var id int64
		var alertType, expVal, actVal, status, email string
		var accountID, entryID *int64
		var txID *string
		var details any
		var createdAt time.Time
		if err := rows.Scan(&id, &alertType, &accountID, &txID, &entryID, &expVal, &actVal, &details, &status, &createdAt, &email); err == nil {
			alerts = append(alerts, fiber.Map{
				"id":             id,
				"alert_type":     alertType,
				"account_id":     accountID,
				"tx_id":          txID,
				"entry_id":       entryID,
				"expected_value": expVal,
				"actual_value":   actVal,
				"details":        details,
				"status":         status,
				"created_at":     createdAt,
				"user_email":     email,
			})
		}
	}
	return c.JSON(alerts)
}

// ListFraudRules returns active and inactive risk rules.
// GET /api/admin/fintech/fraud/rules
func (h *FintechHandler) ListFraudRules(c *fiber.Ctx) error {
	rows, err := h.Pool.Query(c.Context(),
		`SELECT id, rule_code, name_ar, description, condition_type, threshold_value,
		        time_window_seconds, action, is_active, created_at, updated_at
		 FROM fraud_rules
		 ORDER BY id ASC`,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var rules []fraud.Rule
	for rows.Next() {
		var r fraud.Rule
		if err := rows.Scan(&r.ID, &r.RuleCode, &r.NameAR, &r.Description, &r.ConditionType,
			&r.ThresholdValue, &r.TimeWindowSeconds, &r.Action, &r.IsActive, &r.CreatedAt, &r.UpdatedAt); err == nil {
			rules = append(rules, r)
		}
	}
	return c.JSON(rules)
}

// UpdateFraudRule updates threshold or action on a fraud rule.
// PUT /api/admin/fintech/fraud/rules/:id
func (h *FintechHandler) UpdateFraudRule(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid rule id"})
	}

	var req struct {
		ThresholdValue    int64  `json:"threshold_value"`
		TimeWindowSeconds int    `json:"time_window_seconds"`
		Action            string `json:"action"`
		IsActive          *bool  `json:"is_active"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid payload"})
	}

	_, err = h.Pool.Exec(c.Context(),
		`UPDATE fraud_rules
		 SET threshold_value = $1,
		     time_window_seconds = $2,
		     action = $3,
		     is_active = COALESCE($4, is_active),
		     updated_at = now()
		 WHERE id = $5`,
		req.ThresholdValue, req.TimeWindowSeconds, req.Action, req.IsActive, id,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "update rule: " + err.Error()})
	}

	_ = h.FraudEngine.RefreshRules(c.Context())
	return c.JSON(fiber.Map{"success": true})
}

// ListFraudEvents returns risk events with user details.
// GET /api/admin/fintech/fraud/events
func (h *FintechHandler) ListFraudEvents(c *fiber.Ctx) error {
	rows, err := h.Pool.Query(c.Context(),
		`SELECT fe.id, fe.user_id, COALESCE(u.email, ''), COALESCE(u.store_name, ''),
		        fe.rule_code, COALESCE(fr.name_ar, fe.rule_code), fe.severity,
		        fe.action_taken, fe.details, fe.status, fe.triggered_at, fe.reviewed_at, fe.reviewed_by
		 FROM fraud_events fe
		 LEFT JOIN users u ON fe.user_id = u.id
		 LEFT JOIN fraud_rules fr ON fe.rule_code = fr.rule_code
		 ORDER BY fe.id DESC
		 LIMIT 100`,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var events []fraud.Event
	for rows.Next() {
		var ev fraud.Event
		if err := rows.Scan(&ev.ID, &ev.UserID, &ev.UserEmail, &ev.StoreName, &ev.RuleCode, &ev.RuleNameAR,
			&ev.Severity, &ev.ActionTaken, &ev.Details, &ev.Status, &ev.TriggeredAt, &ev.ReviewedAt, &ev.ReviewedBy); err == nil {
			events = append(events, ev)
		}
	}
	return c.JSON(events)
}

// audit writes structured before/after audit entries into audit_logs.
func (h *FintechHandler) audit(c *fiber.Ctx, action, entity, entityID string, details map[string]any) {
	if h == nil || h.Pool == nil {
		return
	}
	cl := middleware.ClaimsFrom(c)
	uid := int64(0)
	email, name, store := "", "", ""
	if cl != nil {
		uid = cl.UserID
		email = cl.Email
	}
	if uid > 0 {
		_ = h.Pool.QueryRow(c.Context(),
			`SELECT email, full_name, store_name FROM users WHERE id = $1`, uid,
		).Scan(&email, &name, &store)
	}
	ip := c.IP()
	detailsJSON, _ := json.Marshal(details)
	_, _ = h.Pool.Exec(c.Context(),
		`INSERT INTO audit_logs (user_id, user_email, user_name, store_name, action, entity, entity_id, details, ip_address)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9)`,
		nullableInt64(uid), email, name, store, action, entity, entityID, string(detailsJSON), ip)
}

// UnfreezeWallet unlocks a subscriber's account.
// POST /api/admin/fintech/wallets/:id/unfreeze
func (h *FintechHandler) UnfreezeWallet(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}

	adminID := middleware.UserID(c)
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.BodyParser(&req)

	if err := h.FraudEngine.UnfreezeWallet(c.Context(), id, adminID, req.Reason); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "unfreeze: " + err.Error()})
	}

	h.audit(c, "WALLET_UNFREEZE", "user", strconv.FormatInt(id, 10), map[string]any{
		"reason": req.Reason,
		"before": map[string]any{"wallet_status": "frozen"},
		"after":  map[string]any{"wallet_status": "active"},
	})

	return c.JSON(fiber.Map{"success": true, "message": "تم إلغاء تجميد المحفظة بنجاح"})
}

// SetCreditLimit sets or updates a B2B credit limit on a subscriber account.
// POST /api/admin/fintech/wallets/:id/credit-limit
func (h *FintechHandler) SetCreditLimit(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}

	var req struct {
		CreditLimit int64  `json:"credit_limit"`
		Reason      string `json:"reason"`
	}
	if err := c.BodyParser(&req); err != nil || req.CreditLimit < 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "credit_limit must be >= 0"})
	}

	var oldLimit int64
	_ = h.Pool.QueryRow(c.Context(), `SELECT COALESCE(credit_limit, 0) FROM users WHERE id = $1`, id).Scan(&oldLimit)

	res, err := h.Pool.Exec(c.Context(),
		`UPDATE users
		 SET credit_limit = $1, updated_at = now()
		 WHERE id = $2`,
		req.CreditLimit, id,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "update credit limit: " + err.Error()})
	}
	if res.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	h.audit(c, "SET_CREDIT_LIMIT", "user", strconv.FormatInt(id, 10), map[string]any{
		"reason": req.Reason,
		"before": map[string]any{"credit_limit": oldLimit},
		"after":  map[string]any{"credit_limit": req.CreditLimit},
	})

	if h.Hub != nil {
		h.Hub.Broadcast(id, "credit_limit_updated", fiber.Map{
			"user_id":      id,
			"credit_limit": req.CreditLimit,
		})
	}

	return c.JSON(fiber.Map{
		"success":      true,
		"user_id":      id,
		"credit_limit": req.CreditLimit,
		"message":      "تم تحديث سقف الائتمان بنجاح",
	})
}

// ListAgentDebts returns agents utilizing overdraft or with active credit limits.
// GET /api/admin/fintech/wallets/debts
func (h *FintechHandler) ListAgentDebts(c *fiber.Ctx) error {
	rows, err := h.Pool.Query(c.Context(),
		`SELECT id, email, full_name, store_name, phone, wallet_balance, credit_limit, wallet_status
		 FROM users
		 WHERE credit_limit > 0 OR wallet_balance < 0
		 ORDER BY wallet_balance ASC`,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var agents []fiber.Map
	for rows.Next() {
		var id, creditLimit int64
		var email, name, store, phone, status string
		var balance float64
		if err := rows.Scan(&id, &email, &name, &store, &phone, &balance, &creditLimit, &status); err == nil {
			balIQD := int64(balance)
			var utilizedDebt int64
			var utilPercent float64
			if balIQD < 0 {
				utilizedDebt = -balIQD
				if creditLimit > 0 {
					utilPercent = float64(utilizedDebt) / float64(creditLimit) * 100.0
				}
			}
			agents = append(agents, fiber.Map{
				"id":               id,
				"email":            email,
				"full_name":        name,
				"store_name":       store,
				"phone":            phone,
				"wallet_balance":   balIQD,
				"credit_limit":     creditLimit,
				"wallet_status":    status,
				"utilized_debt":    utilizedDebt,
				"utilization_pct":  fmt.Sprintf("%.1f%%", utilPercent),
			})
		}
	}
	return c.JSON(agents)
}

// RunReconciliation runs reconciliation for a given date.
// POST /api/admin/fintech/reconcile/run
func (h *FintechHandler) RunReconciliation(c *fiber.Ctx) error {
	var req struct {
		Date string `json:"date"`
	}
	_ = c.BodyParser(&req)

	targetDate := time.Now().UTC()
	if req.Date != "" {
		if parsed, err := time.Parse("2006-01-02", req.Date); err == nil {
			targetDate = parsed
		}
	}

	report, err := h.ReconcileSvc.RunDailyReconciliation(c.Context(), targetDate, "admin_manual")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "reconciliation error: " + err.Error()})
	}

	h.audit(c, "MANUAL_RECONCILIATION", "reconciliation_reports", strconv.FormatInt(report.ID, 10), map[string]any{
		"report_date": report.ReportDate,
		"status":      report.Status,
		"discrepancy": report.Discrepancy,
		"inflow":      report.TotalGatewayVolume + report.ManualTopups,
		"system_bal":  report.TotalSystemBalance,
	})

	return c.JSON(report)
}

// ListReconciliationReports lists historical reconciliation runs.
// GET /api/admin/fintech/reconcile/reports
func (h *FintechHandler) ListReconciliationReports(c *fiber.Ctx) error {
	rows, err := h.Pool.Query(c.Context(),
		`SELECT id, report_date, total_system_balance, total_gateway_volume,
		        total_ledger_credits, total_ledger_debits, expected_closing_balance,
		        actual_closing_balance, discrepancy, status, details, generated_by, created_at
		 FROM reconciliation_reports
		 ORDER BY id DESC
		 LIMIT 50`,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var reports []reconcile.ReconciliationReport
	for rows.Next() {
		var rep reconcile.ReconciliationReport
		var rDate time.Time
		if err := rows.Scan(
			&rep.ID, &rDate, &rep.TotalSystemBalance, &rep.TotalGatewayVolume,
			&rep.TotalLedgerCredits, &rep.TotalLedgerDebits, &rep.ExpectedClosingBalance,
			&rep.ActualClosingBalance, &rep.Discrepancy, &rep.Status, &rep.Details,
			&rep.GeneratedBy, &rep.CreatedAt,
		); err == nil {
			rep.ReportDate = rDate.Format("2006-01-02")
			reports = append(reports, rep)
		}
	}
	return c.JSON(reports)
}

// ExportReconciliationReportCSV exports an existing report to CSV format (with UTF-8 BOM) or Excel HTML format.
// GET /api/admin/fintech/reconcile/reports/:id/export?format=csv|excel
func (h *FintechHandler) ExportReconciliationReportCSV(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid report id"})
	}

	format := c.Query("format", "csv")

	var rep reconcile.ReconciliationReport
	var rDate time.Time
	err = h.Pool.QueryRow(c.Context(),
		`SELECT id, report_date, total_system_balance, total_gateway_volume,
		        total_ledger_credits, total_ledger_debits, expected_closing_balance,
		        actual_closing_balance, discrepancy, status, details, generated_by, created_at
		 FROM reconciliation_reports
		 WHERE id = $1`, id,
	).Scan(
		&rep.ID, &rDate, &rep.TotalSystemBalance, &rep.TotalGatewayVolume,
		&rep.TotalLedgerCredits, &rep.TotalLedgerDebits, &rep.ExpectedClosingBalance,
		&rep.ActualClosingBalance, &rep.Discrepancy, &rep.Status, &rep.Details,
		&rep.GeneratedBy, &rep.CreatedAt,
	)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "report not found"})
	}
	rep.ReportDate = rDate.Format("2006-01-02")

	if format == "xlsx" || format == "excel" {
		excelData, err := reconcile.GenerateExcelXLSX(&rep)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to generate excel file: " + err.Error()})
		}
		c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=reconciliation_%s.xlsx", rep.ReportDate))
		return c.Send(excelData)
	}

	if format == "pdf" {
		pdfData := reconcile.GeneratePDF(&rep)
		c.Set("Content-Type", "application/pdf")
		c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=reconciliation_%s.pdf", rep.ReportDate))
		return c.Send(pdfData)
	}

	csvData := reconcile.GenerateCSV(&rep)
	c.Set("Content-Type", "text/csv; charset=utf-8")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=reconciliation_%s.csv", rep.ReportDate))
	return c.Send(csvData)
}

// SealFinancialClose generates a cryptographic seal on the books for a day or month.
// POST /api/admin/fintech/close/seal
func (h *FintechHandler) SealFinancialClose(c *fiber.Ctx) error {
	adminID := middleware.UserID(c)
	var req struct {
		PeriodType string `json:"period_type"` // 'daily' or 'monthly'
		Date       string `json:"date"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if req.PeriodType != "daily" && req.PeriodType != "monthly" {
		req.PeriodType = "daily"
	}

	targetDate := time.Now().UTC()
	if req.Date != "" {
		if parsed, err := time.Parse("2006-01-02", req.Date); err == nil {
			targetDate = parsed
		}
	}

	closeRec, err := h.ReconcileSvc.SealFinancialClose(c.Context(), req.PeriodType, targetDate, adminID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "close error: " + err.Error()})
	}

	h.audit(c, "SEAL_FINANCIAL_CLOSE", "financial_closes", strconv.FormatInt(closeRec.ID, 10), map[string]any{
		"period_type":  closeRec.PeriodType,
		"period_date":  closeRec.PeriodDate,
		"closing_hash": closeRec.ClosingHash,
		"net_revenue":  closeRec.NetRevenue,
		"liability":    closeRec.TotalWalletLiability,
		"tx_count":     closeRec.TransactionCount,
	})

	return c.JSON(closeRec)
}

// ListFinancialCloses returns historical sealed closes.
// GET /api/admin/fintech/close/history
func (h *FintechHandler) ListFinancialCloses(c *fiber.Ctx) error {
	rows, err := h.Pool.Query(c.Context(),
		`SELECT id, period_type, period_date, total_inflow, total_outflow, net_revenue,
		        total_wallet_liability, transaction_count, closing_hash, status, closed_by, closed_at
		 FROM financial_closes
		 ORDER BY period_date DESC
		 LIMIT 50`,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var list []fiber.Map
	for rows.Next() {
		var id, inflow, outflow, netRev, liability, txCount int64
		var pType, pDate, hash, status string
		var closedBy *int64
		var closedAt time.Time
		if err := rows.Scan(&id, &pType, &pDate, &inflow, &outflow, &netRev, &liability, &txCount, &hash, &status, &closedBy, &closedAt); err == nil {
			list = append(list, fiber.Map{
				"id":                     id,
				"period_type":            pType,
				"period_date":            pDate,
				"total_inflow":           inflow,
				"total_outflow":          outflow,
				"net_revenue":            netRev,
				"total_wallet_liability": liability,
				"transaction_count":      txCount,
				"closing_hash":           hash,
				"status":                 status,
				"closed_by":              closedBy,
				"closed_at":              closedAt,
			})
		}
	}
	return c.JSON(list)
}

// GetAccountStatement returns full financial statement for a subscriber.
// GET /api/admin/fintech/statements/:id
func (h *FintechHandler) GetAccountStatement(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}

	var user struct {
		ID            int64
		Email         string
		FullName      string
		StoreName     string
		Phone         string
		Balance       float64
		CreditLimit   int64
		WalletStatus  string
		CreatedAt     time.Time
	}

	err = h.Pool.QueryRow(c.Context(),
		`SELECT id, email, full_name, store_name, phone, wallet_balance, credit_limit, wallet_status, created_at
		 FROM users
		 WHERE id = $1`, id,
	).Scan(&user.ID, &user.Email, &user.FullName, &user.StoreName, &user.Phone, &user.Balance, &user.CreditLimit, &user.WalletStatus, &user.CreatedAt)
	if err == pgx.ErrNoRows {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	rows, err := h.Pool.Query(c.Context(),
		`SELECT id, tx_id, debit, credit, balance_after, ref_type, ref_id, created_at, hash
		 FROM ledger_entries
		 WHERE account_id = $1
		 ORDER BY id ASC`, id,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var entries []fiber.Map
	var totalCredits, totalDebits int64
	for rows.Next() {
		var eid, debit, credit, balAfter int64
		var txID, refType, refID, hash string
		var cAt time.Time
		if err := rows.Scan(&eid, &txID, &debit, &credit, &balAfter, &refType, &refID, &cAt, &hash); err == nil {
			totalCredits += credit
			totalDebits += debit
			entries = append(entries, fiber.Map{
				"id":            eid,
				"tx_id":         txID,
				"debit":         debit,
				"credit":        credit,
				"balance_after": balAfter,
				"ref_type":      refType,
				"ref_id":        refID,
				"created_at":    cAt,
				"hash":          hash,
			})
		}
	}

	format := c.Query("format")
	if format == "xlsx" || format == "excel" {
		entriesMap := make([]map[string]any, len(entries))
		for i, e := range entries {
			entriesMap[i] = map[string]any(e)
		}
		xlsxData, err := reconcile.GenerateStatementExcelXLSX(user.Email, user.StoreName, int64(user.Balance), user.CreditLimit, entriesMap)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to generate statement xlsx: " + err.Error()})
		}
		c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="statement_user_%d.xlsx"`, user.ID))
		return c.Send(xlsxData)
	}

	if format == "pdf" {
		pdfData := reconcile.GenerateStatementPDF(user.Email, user.StoreName, int64(user.Balance), user.CreditLimit, len(entries))
		c.Set("Content-Type", "application/pdf")
		c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="statement_user_%d.pdf"`, user.ID))
		return c.Send(pdfData)
	}

	if format == "csv" {
		// CSV with UTF-8 BOM
		c.Set("Content-Type", "text/csv; charset=utf-8")
		c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="statement_user_%d.csv"`, user.ID))
		var b bytes.Buffer
		b.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
		w := csv.NewWriter(&b)
		_ = w.Write([]string{"tx_id", "created_at", "ref_type", "debit", "credit", "balance_after", "hash"})
		for _, e := range entries {
			_ = w.Write([]string{
				fmt.Sprint(e["tx_id"]),
				e["created_at"].(time.Time).Format("2006-01-02 15:04:05"),
				fmt.Sprint(e["ref_type"]),
				fmt.Sprint(e["debit"]),
				fmt.Sprint(e["credit"]),
				fmt.Sprint(e["balance_after"]),
				fmt.Sprint(e["hash"]),
			})
		}
		w.Flush()
		return c.Send(b.Bytes())
	}

	return c.JSON(fiber.Map{
		"user": fiber.Map{
			"id":              user.ID,
			"email":           user.Email,
			"full_name":       user.FullName,
			"store_name":      user.StoreName,
			"phone":           user.Phone,
			"current_balance": int64(user.Balance),
			"credit_limit":    user.CreditLimit,
			"wallet_status":   user.WalletStatus,
			"created_at":      user.CreatedAt,
		},
		"total_credits": totalCredits,
		"total_debits":  totalDebits,
		"entries_count": len(entries),
		"entries":       entries,
		"generated_at":  time.Now().UTC(),
	})
}

// =============================================================================
// PHASE 2 HANDLERS: PRICING TIERS, AGENT COMMISSIONS, INVENTORY RADAR
// =============================================================================

// ListPriceTiers returns all pricing tiers with assigned product rates.
// GET /api/admin/pricing/tiers
func (h *FintechHandler) ListPriceTiers(c *fiber.Ctx) error {
	tiers, err := h.PricingSvc.ListTiers(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	rows, err := h.Pool.Query(c.Context(),
		`SELECT tier_id, product_id, price FROM tier_prices ORDER BY product_id ASC`,
	)
	tierPricesMap := make(map[int64][]fiber.Map)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var tid int64
			var pid string
			var price int64
			if err := rows.Scan(&tid, &pid, &price); err == nil {
				tierPricesMap[tid] = append(tierPricesMap[tid], fiber.Map{
					"product_id": pid,
					"price":      price,
				})
			}
		}
	}

	var result []fiber.Map
	for _, t := range tiers {
		prices := tierPricesMap[t.ID]
		if prices == nil {
			prices = []fiber.Map{}
		}
		result = append(result, fiber.Map{
			"id":          t.ID,
			"code":        t.Code,
			"name_ar":     t.NameAr,
			"description": t.Description,
			"is_default":  t.IsDefault,
			"prices":      prices,
		})
	}

	return c.JSON(result)
}

// SetTierPrice sets a specific price for a product under a tier.
// POST /api/admin/pricing/tiers/:id/prices
func (h *FintechHandler) SetTierPrice(c *fiber.Ctx) error {
	tierID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tier id"})
	}

	var req struct {
		ProductID string `json:"product_id"`
		Price     int64  `json:"price"`
	}
	if err := c.BodyParser(&req); err != nil || req.ProductID == "" || req.Price < 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid product_id or price"})
	}

	if err := h.PricingSvc.SetTierPrice(c.Context(), tierID, req.ProductID, req.Price); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	h.audit(c, "SET_TIER_PRICE", "tier_prices", fmt.Sprintf("%d:%s", tierID, req.ProductID), map[string]any{
		"tier_id":    tierID,
		"product_id": req.ProductID,
		"price":      req.Price,
	})

	return c.JSON(fiber.Map{"success": true})
}

// SetSubscriberPriceOverride configures an exclusive custom price for a subscriber.
// POST /api/admin/pricing/subscribers/:id/overrides
func (h *FintechHandler) SetSubscriberPriceOverride(c *fiber.Ctx) error {
	userID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}

	var req struct {
		ProductID   string `json:"product_id"`
		CustomPrice int64  `json:"custom_price"`
	}
	if err := c.BodyParser(&req); err != nil || req.ProductID == "" || req.CustomPrice < 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid product_id or custom_price"})
	}

	if err := h.PricingSvc.SetSubscriberOverride(c.Context(), userID, req.ProductID, req.CustomPrice); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	h.audit(c, "SET_PRICE_OVERRIDE", "subscriber_price_overrides", fmt.Sprintf("%d:%s", userID, req.ProductID), map[string]any{
		"user_id":      userID,
		"product_id":   req.ProductID,
		"custom_price": req.CustomPrice,
	})

	return c.JSON(fiber.Map{"success": true})
}

// RemoveSubscriberPriceOverride removes a custom price override.
// DELETE /api/admin/pricing/subscribers/:id/overrides/:productId
func (h *FintechHandler) RemoveSubscriberPriceOverride(c *fiber.Ctx) error {
	userID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}
	productID := c.Params("productId")

	if err := h.PricingSvc.RemoveSubscriberOverride(c.Context(), userID, productID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	h.audit(c, "REMOVE_PRICE_OVERRIDE", "subscriber_price_overrides", fmt.Sprintf("%d:%s", userID, productID), nil)

	return c.JSON(fiber.Map{"success": true})
}

// ListSubscriberPriceOverrides returns all custom price overrides.
// GET /api/admin/pricing/overrides
func (h *FintechHandler) ListSubscriberPriceOverrides(c *fiber.Ctx) error {
	rows, err := h.Pool.Query(c.Context(),
		`SELECT spo.id, spo.user_id, u.email, COALESCE(u.store_name, u.full_name, ''),
		        spo.product_id, spo.custom_price, spo.updated_at
		 FROM subscriber_price_overrides spo
		 JOIN users u ON u.id = spo.user_id
		 ORDER BY spo.updated_at DESC`,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var list []fiber.Map
	for rows.Next() {
		var (
			id, uid          int64
			email, name, pid string
			price            int64
			updatedAt        time.Time
		)
		if err := rows.Scan(&id, &uid, &email, &name, &pid, &price, &updatedAt); err == nil {
			list = append(list, fiber.Map{
				"id":           id,
				"user_id":      uid,
				"email":        email,
				"store_name":   name,
				"product_id":   pid,
				"custom_price": price,
				"updated_at":   updatedAt,
			})
		}
	}
	if list == nil {
		list = []fiber.Map{}
	}
	return c.JSON(list)
}

// ResolveSubscriberPrice evaluates the exact resolved price for a subscriber.
// GET /api/admin/pricing/resolve/:userId/:productId
func (h *FintechHandler) ResolveSubscriberPrice(c *fiber.Ctx) error {
	userID, err := strconv.ParseInt(c.Params("userId"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}
	productID := c.Params("productId")
	defaultPrice, _ := strconv.ParseInt(c.Query("default_price", "0"), 10, 64)

	res, err := h.PricingSvc.ResolvePrice(c.Context(), userID, productID, defaultPrice)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}

// SetParentAgent assigns a parent agent with DAG cycle prevention.
// POST /api/admin/hierarchy/users/:id/parent
func (h *FintechHandler) SetParentAgent(c *fiber.Ctx) error {
	childID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid child id"})
	}

	var req struct {
		ParentAgentID *int64 `json:"parent_agent_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	if req.ParentAgentID != nil && *req.ParentAgentID > 0 {
		if err := h.HierarchySvc.CheckCyclePreventsInfiniteLoop(c.Context(), childID, *req.ParentAgentID); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
	}

	_, err = h.Pool.Exec(c.Context(),
		`UPDATE users SET parent_agent_id = $1 WHERE id = $2`,
		req.ParentAgentID, childID,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	h.audit(c, "SET_PARENT_AGENT", "users", strconv.FormatInt(childID, 10), map[string]any{
		"child_id":        childID,
		"parent_agent_id": req.ParentAgentID,
	})

	return c.JSON(fiber.Map{"success": true})
}

// SetAgentCommission configures an agent's commission rate.
// POST /api/admin/hierarchy/agents/:id/commissions
func (h *FintechHandler) SetAgentCommission(c *fiber.Ctx) error {
	agentID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid agent id"})
	}

	var req struct {
		ProductID             string   `json:"product_id"`
		CommissionType        string   `json:"commission_type"`
		CommissionBasisPoints int      `json:"commission_basis_points"`
		FixedAmount           int64    `json:"fixed_amount"`
		CapToMargin           *bool    `json:"cap_to_margin"`
		CommissionValue       float64  `json:"commission_value"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.ProductID == "" {
		req.ProductID = "*"
	}
	if req.CommissionType != "percentage" && req.CommissionType != "fixed" {
		req.CommissionType = "percentage"
	}

	capToMargin := true
	if req.CapToMargin != nil {
		capToMargin = *req.CapToMargin
	}

	basisPoints := req.CommissionBasisPoints
	fixedAmt := req.FixedAmount
	cVal := req.CommissionValue

	if req.CommissionType == "percentage" {
		if basisPoints <= 0 && cVal > 0 {
			basisPoints = int(math.Round(cVal * 100))
		}
		if cVal <= 0 && basisPoints > 0 {
			cVal = float64(basisPoints) / 100.0
		}
	} else {
		if fixedAmt <= 0 && cVal > 0 {
			fixedAmt = int64(math.Round(cVal))
		}
		if cVal <= 0 && fixedAmt > 0 {
			cVal = float64(fixedAmt)
		}
	}

	_, err = h.Pool.Exec(c.Context(),
		`INSERT INTO agent_commissions (agent_id, product_id, commission_type, commission_value, commission_basis_points, fixed_amount, cap_to_margin)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (agent_id, product_id)
		 DO UPDATE SET commission_type = EXCLUDED.commission_type,
		               commission_value = EXCLUDED.commission_value,
		               commission_basis_points = EXCLUDED.commission_basis_points,
		               fixed_amount = EXCLUDED.fixed_amount,
		               cap_to_margin = EXCLUDED.cap_to_margin`,
		agentID, req.ProductID, req.CommissionType, cVal, basisPoints, fixedAmt, capToMargin,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	h.audit(c, "SET_AGENT_COMMISSION", "agent_commissions", fmt.Sprintf("%d:%s", agentID, req.ProductID), map[string]any{
		"agent_id":                agentID,
		"product_id":              req.ProductID,
		"commission_type":         req.CommissionType,
		"commission_basis_points": basisPoints,
		"fixed_amount":            fixedAmt,
		"cap_to_margin":           capToMargin,
		"commission_value":        cVal,
	})

	return c.JSON(fiber.Map{"success": true})
}

// ListAgentCommissions returns active commission rules.
// GET /api/admin/hierarchy/commissions
func (h *FintechHandler) ListAgentCommissions(c *fiber.Ctx) error {
	rows, err := h.Pool.Query(c.Context(),
		`SELECT ac.id, ac.agent_id, u.email, COALESCE(u.store_name, u.full_name, ''),
		        ac.product_id, ac.commission_type, ac.commission_basis_points, ac.fixed_amount, ac.cap_to_margin, ac.commission_value, ac.created_at
		 FROM agent_commissions ac
		 JOIN users u ON u.id = ac.agent_id
		 ORDER BY ac.id DESC`,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var list []fiber.Map
	for rows.Next() {
		var id, aid int64
		var email, name, pid, cType string
		var basisPts int
		var fixedAmt int64
		var capMargin bool
		var cVal float64
		var cAt time.Time
		if err := rows.Scan(&id, &aid, &email, &name, &pid, &cType, &basisPts, &fixedAmt, &capMargin, &cVal, &cAt); err == nil {
			list = append(list, fiber.Map{
				"id":                      id,
				"agent_id":                aid,
				"agent_email":             email,
				"agent_name":              name,
				"product_id":              pid,
				"commission_type":         cType,
				"commission_basis_points": basisPts,
				"fixed_amount":            fixedAmt,
				"cap_to_margin":           capMargin,
				"commission_value":        cVal,
				"created_at":              cAt,
			})
		}
	}
	return c.JSON(list)
}

// GetAgentCommissionReport generates a comprehensive commission statement and activity report for an agent.
// GET /api/admin/hierarchy/agents/:id/report
func (h *FintechHandler) GetAgentCommissionReport(c *fiber.Ctx) error {
	agentID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid agent id"})
	}

	// 1. Agent profile
	var (
		email, fullName, storeName string
		balance                    int64
	)
	err = h.Pool.QueryRow(c.Context(),
		`SELECT email, COALESCE(full_name, ''), COALESCE(store_name, ''), wallet_balance
		 FROM users WHERE id = $1`, agentID,
	).Scan(&email, &fullName, &storeName, &balance)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	}

	// 2. Child Points
	childRows, err := h.Pool.Query(c.Context(),
		`SELECT id, email, COALESCE(store_name, full_name, ''), wallet_balance, created_at
		 FROM users WHERE parent_agent_id = $1 ORDER BY id DESC`, agentID,
	)
	var childPoints []fiber.Map
	if err == nil {
		defer childRows.Close()
		for childRows.Next() {
			var (
				cid           int64
				cEmail, cName string
				cBal          int64
				cCreated      time.Time
			)
			if err := childRows.Scan(&cid, &cEmail, &cName, &cBal, &cCreated); err == nil {
				childPoints = append(childPoints, fiber.Map{
					"id":             cid,
					"email":          cEmail,
					"name":           cName,
					"wallet_balance": cBal,
					"created_at":     cCreated,
				})
			}
		}
	}
	if childPoints == nil {
		childPoints = []fiber.Map{}
	}

	// 3. Totals
	var totalEarned, totalReversed int64
	var countEarned, countReversed int64
	_ = h.Pool.QueryRow(c.Context(),
		`SELECT COALESCE(SUM(credit), 0), COUNT(*)
		 FROM ledger_entries
		 WHERE account_id = $1 AND ref_type = 'agent_commission'`, agentID,
	).Scan(&totalEarned, &countEarned)

	_ = h.Pool.QueryRow(c.Context(),
		`SELECT COALESCE(SUM(debit), 0), COUNT(*)
		 FROM ledger_entries
		 WHERE account_id = $1 AND ref_type = 'agent_commission_reversal'`, agentID,
	).Scan(&totalReversed, &countReversed)

	// 4. Recent commission entries
	entryRows, err := h.Pool.Query(c.Context(),
		`SELECT tx_id, debit, credit, balance_after, ref_type, ref_id, created_at
		 FROM ledger_entries
		 WHERE account_id = $1 AND ref_type IN ('agent_commission', 'agent_commission_reversal')
		 ORDER BY id DESC LIMIT 50`, agentID,
	)
	var entries []fiber.Map
	if err == nil {
		defer entryRows.Close()
		for entryRows.Next() {
			var (
				txID, refType, refID string
				debit, credit, bal   int64
				createdAt            time.Time
			)
			if err := entryRows.Scan(&txID, &debit, &credit, &bal, &refType, &refID, &createdAt); err == nil {
				entries = append(entries, fiber.Map{
					"tx_id":         txID,
					"debit":         debit,
					"credit":        credit,
					"balance_after": bal,
					"ref_type":      refType,
					"ref_id":        refID,
					"created_at":    createdAt,
				})
			}
		}
	}
	if entries == nil {
		entries = []fiber.Map{}
	}

	return c.JSON(fiber.Map{
		"agent": fiber.Map{
			"id":             agentID,
			"email":          email,
			"full_name":      fullName,
			"store_name":     storeName,
			"wallet_balance": balance,
		},
		"total_commission_earned":   totalEarned,
		"total_commission_reversed": totalReversed,
		"net_commission":            totalEarned - totalReversed,
		"commission_sales_count":    countEarned,
		"reversals_count":           countReversed,
		"child_points_count":        len(childPoints),
		"child_points":              childPoints,
		"recent_entries":            entries,
	})
}

// GetInventoryRadar returns real-time inventory levels, burn rates, and low stock alarms.
// GET /api/admin/radar/summary
func (h *FintechHandler) GetInventoryRadar(c *fiber.Ctx) error {
	if h.RadarSvc == nil {
		return c.JSON(fiber.Map{"has_low_stock": false, "categories": []any{}})
	}
	summary, err := h.RadarSvc.GetRadarSummary(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(summary)
}

// SetCategoryThreshold configures a minimum stock alarm threshold for a category.
// POST /api/admin/radar/thresholds
func (h *FintechHandler) SetCategoryThreshold(c *fiber.Ctx) error {
	var req struct {
		CategoryID   string `json:"category_id"`
		MinThreshold int    `json:"min_threshold"`
	}
	if err := c.BodyParser(&req); err != nil || req.CategoryID == "" || req.MinThreshold < 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid category_id or min_threshold"})
	}

	_, err := h.Pool.Exec(c.Context(),
		`INSERT INTO category_thresholds (category_id, min_threshold, updated_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (category_id)
		 DO UPDATE SET min_threshold = EXCLUDED.min_threshold, updated_at = now()`,
		req.CategoryID, req.MinThreshold,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	h.audit(c, "SET_CATEGORY_THRESHOLD", "category_thresholds", req.CategoryID, map[string]any{
		"category_id":   req.CategoryID,
		"min_threshold": req.MinThreshold,
	})

	return c.JSON(fiber.Map{"success": true})
}

// RotateVaultKeys re-encrypts stored voucher PINs to a target key version in batches.
// POST /api/admin/vault/rotate-keys
func (h *FintechHandler) RotateVaultKeys(c *fiber.Ctx) error {
	var req struct {
		TargetVersion int `json:"target_version"`
		BatchSize     int `json:"batch_size"`
	}
	if err := c.BodyParser(&req); err != nil || req.TargetVersion <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid target_version"})
	}
	if req.BatchSize <= 0 {
		req.BatchSize = 100
	}

	report, err := vault.ReencryptVaultVouchers(c.Context(), h.Pool, h.VaultMgr, req.TargetVersion, req.BatchSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	h.audit(c, "ROTATE_VAULT_KEYS", "vouchers_vault", fmt.Sprintf("v%d", req.TargetVersion), map[string]any{
		"target_version": req.TargetVersion,
		"reencrypted":    report.Reencrypted,
		"failed":         report.Failed,
		"total":          report.TotalVouchers,
	})

	return c.JSON(report)
}
