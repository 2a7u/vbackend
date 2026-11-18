package handlers

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/appconfig"
	"venus_backend/internal/bi"
	"venus_backend/internal/guard"
	"venus_backend/internal/idem"
	"venus_backend/internal/limits"
	"venus_backend/internal/middleware"
	"venus_backend/internal/security"
	"venus_backend/internal/subscribers"
)

type EnterpriseHandler struct {
	Pool          *pgxpool.Pool
	Hub           *WSHub
	SubscriberSvc *subscribers.Service
	AppConfigSvc  *appconfig.Service
	SecuritySvc   *security.Service
	BISvc         *bi.Service
	GuardSvc      *guard.Service
	IdemSvc       *idem.Service
	LimitsSvc     *limits.Service
}

func NewEnterpriseHandler(
	pool *pgxpool.Pool,
	hub *WSHub,
	subSvc *subscribers.Service,
	appCfgSvc *appconfig.Service,
	secSvc *security.Service,
	biSvc *bi.Service,
	guardSvc *guard.Service,
	idemSvc *idem.Service,
	limitsSvc *limits.Service,
) *EnterpriseHandler {
	return &EnterpriseHandler{
		Pool:          pool,
		Hub:           hub,
		SubscriberSvc: subSvc,
		AppConfigSvc:  appCfgSvc,
		SecuritySvc:   secSvc,
		BISvc:         biSvc,
		GuardSvc:      guardSvc,
		IdemSvc:       idemSvc,
		LimitsSvc:     limitsSvc,
	}
}

// -----------------------------------------------------------------------------
// Phase 3: Subscriber 360, Bulk Actions, Impersonation, Device Controls
// -----------------------------------------------------------------------------

func (h *EnterpriseHandler) GetSubscriber360(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}
	overview, err := h.SubscriberSvc.GetSubscriber360(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(overview)
}

func (h *EnterpriseHandler) BulkSubscribersPreview(c *fiber.Ctx) error {
	var req subscribers.BulkActionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	req.DryRun = true
	adminID := middleware.UserID(c)
	res, err := h.SubscriberSvc.ProcessBulkAction(c.Context(), adminID, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}

func (h *EnterpriseHandler) BulkSubscribersExecute(c *fiber.Ctx) error {
	var req subscribers.BulkActionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	req.DryRun = false
	adminID := middleware.UserID(c)
	res, err := h.SubscriberSvc.ProcessBulkAction(c.Context(), adminID, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}

func (h *EnterpriseHandler) KickSessions(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}
	if err := h.SubscriberSvc.EvictAllSessions(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "ok", "message": "تم طرد جميع الجلسات وإلغاء صلاحية الرموز الحالية"})
}

func (h *EnterpriseHandler) ImpersonateSubscriber(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}
	adminID := middleware.UserID(c)
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.BodyParser(&req)
	if req.Reason == "" {
		req.Reason = "مراجعة الدعم الفني والمطابقة المحاسبية"
	}

	token, err := h.SubscriberSvc.CreateImpersonationToken(c.Context(), adminID, id, req.Reason)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"token":      token,
		"scope":      "read_only",
		"expires_in": 3600,
		"message":    "تم إنشاء جلسة محاكاة الحساب بنجاح (للقراءة فقط - كافة طلبات الكتابة محظورة)",
	})
}

// -----------------------------------------------------------------------------
// Phase 5: App Config & Banners
// -----------------------------------------------------------------------------

func (h *EnterpriseHandler) GetPublicAppConfig(c *fiber.Ctx) error {
	configs := h.AppConfigSvc.GetAllConfigs()
	banners, _ := h.AppConfigSvc.ListBanners(c.Context(), true)

	return c.JSON(fiber.Map{
		"maintenance_mode": configs["maintenance_mode"],
		"force_update":     configs["force_update"],
		"feature_flags":    configs["feature_flags"],
		"ticker":           configs["announcement_ticker"],
		"theme_studio":     configs["theme_studio"],
		"banners":          banners,
	})
}

func (h *EnterpriseHandler) GetAllAppConfigs(c *fiber.Ctx) error {
	return c.JSON(h.AppConfigSvc.GetAllConfigs())
}

func (h *EnterpriseHandler) UpdateAppConfig(c *fiber.Ctx) error {
	key := c.Params("key")
	var val map[string]any
	if err := c.BodyParser(&val); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
	}
	adminID := middleware.UserID(c)
	if err := h.AppConfigSvc.Set(c.Context(), key, val, adminID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "ok", "key": key, "value": val})
}

func (h *EnterpriseHandler) ListBanners(c *fiber.Ctx) error {
	list, err := h.AppConfigSvc.ListBanners(c.Context(), false)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(list)
}

func (h *EnterpriseHandler) SaveBanner(c *fiber.Ctx) error {
	var b appconfig.Banner
	if err := c.BodyParser(&b); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	saved, err := h.AppConfigSvc.SaveBanner(c.Context(), b)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(saved)
}

func (h *EnterpriseHandler) DeleteBanner(c *fiber.Ctx) error {
	id, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	if err := h.AppConfigSvc.DeleteBanner(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

// -----------------------------------------------------------------------------
// Phase 7: Rollback, WAF, API Keys, Webhooks
// -----------------------------------------------------------------------------

func (h *EnterpriseHandler) RollbackOperation(c *fiber.Ctx) error {
	var req struct {
		AuditID int64 `json:"audit_id"`
	}
	if err := c.BodyParser(&req); err != nil || req.AuditID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "audit_id is required"})
	}
	adminID := middleware.UserID(c)
	if err := h.SecuritySvc.RollbackOperation(c.Context(), req.AuditID, adminID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "ok", "message": "تم التراجع عن العملية بنجاح عبر قيد عكسي مطابق في دفتر الأستاذ"})
}

func (h *EnterpriseHandler) ListIPRules(c *fiber.Ctx) error {
	rows, err := h.Pool.Query(c.Context(),
		`SELECT id, ip_address, rule_type, reason, ban_expires_at, created_at FROM ip_rules ORDER BY id DESC`,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	type ipRule struct {
		ID           int64      `json:"id"`
		IP           string     `json:"ip_address"`
		Type         string     `json:"rule_type"`
		Reason       string     `json:"reason"`
		BanExpiresAt *time.Time `json:"ban_expires_at,omitempty"`
		CreatedAt    time.Time  `json:"created_at"`
	}
	var list []ipRule
	for rows.Next() {
		var r ipRule
		if err := rows.Scan(&r.ID, &r.IP, &r.Type, &r.Reason, &r.BanExpiresAt, &r.CreatedAt); err == nil {
			list = append(list, r)
		}
	}
	return c.JSON(list)
}

func (h *EnterpriseHandler) SaveIPRule(c *fiber.Ctx) error {
	var req struct {
		IPAddress string `json:"ip_address"`
		RuleType  string `json:"rule_type"`
		Reason    string `json:"reason"`
	}
	if err := c.BodyParser(&req); err != nil || req.IPAddress == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid ip rule body"})
	}
	_, err := h.Pool.Exec(c.Context(),
		`INSERT INTO ip_rules (ip_address, rule_type, reason)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (ip_address) DO UPDATE SET rule_type = $2, reason = $3`,
		req.IPAddress, req.RuleType, req.Reason,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

func (h *EnterpriseHandler) DeleteIPRule(c *fiber.Ctx) error {
	id, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	_, err := h.Pool.Exec(c.Context(), `DELETE FROM ip_rules WHERE id = $1`, id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

func (h *EnterpriseHandler) GenerateAPIKey(c *fiber.Ctx) error {
	var req struct {
		Name   string   `json:"name"`
		Role   string   `json:"role"`
		Scopes []string `json:"scopes"`
	}
	if err := c.BodyParser(&req); err != nil || req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}
	if req.Role == "" {
		req.Role = "agent"
	}
	key, err := h.SecuritySvc.GenerateAPIKey(c.Context(), req.Name, req.Role, req.Scopes)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"api_key": key,
		"message": "يرجى نسخ المفتاح وحفظه الآن، لن يظهر لك هذا المفتاح مرة أخرى لأسباب أمنية",
	})
}

func (h *EnterpriseHandler) ListAPIKeys(c *fiber.Ctx) error {
	rows, err := h.Pool.Query(c.Context(),
		`SELECT id, key_name, key_prefix, role, scopes, is_active, created_at, last_used_at FROM api_keys ORDER BY id DESC`,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	type keyItem struct {
		ID         int64      `json:"id"`
		Name       string     `json:"key_name"`
		Prefix     string     `json:"key_prefix"`
		Role       string     `json:"role"`
		Scopes     []string   `json:"scopes"`
		IsActive   bool       `json:"is_active"`
		CreatedAt  time.Time  `json:"created_at"`
		LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	}
	var list []keyItem
	for rows.Next() {
		var k keyItem
		if err := rows.Scan(&k.ID, &k.Name, &k.Prefix, &k.Role, &k.Scopes, &k.IsActive, &k.CreatedAt, &k.LastUsedAt); err == nil {
			list = append(list, k)
		}
	}
	return c.JSON(list)
}

// -----------------------------------------------------------------------------
// Phase 8: BI Analytics, Backups, Health Monitoring
// -----------------------------------------------------------------------------

func (h *EnterpriseHandler) GetBIDashboard(c *fiber.Ctx) error {
	board, err := h.BISvc.GetBIDashboard(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(board)
}

func (h *EnterpriseHandler) DownloadSecureBackup(c *fiber.Ctx) error {
	cl := middleware.ClaimsFrom(c)
	if cl == nil || cl.Role != "admin" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Super Admin role required"})
	}

	backupBytes, err := h.BISvc.GenerateFullBackup(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	c.Set(fiber.HeaderContentType, "application/json")
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf("attachment; filename=\"venus_backup_%s.json\"", time.Now().Format("20060102_150405")))
	return c.Send(backupBytes)
}

func (h *EnterpriseHandler) Healthz(c *fiber.Ctx) error {
	health := h.BISvc.GetServerHealth(c.Context())
	if health.Status != "healthy" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(health)
	}
	return c.JSON(health)
}

func (h *EnterpriseHandler) Readyz(c *fiber.Ctx) error {
	health := h.BISvc.GetServerHealth(c.Context())
	if !health.DBConnected {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ready": false, "reason": "database unavailable"})
	}
	return c.JSON(fiber.Map{"ready": true, "latency_ms": health.DBLatencyMs})
}

// ExportSubscribersCSV streams all subscribers in UTF-8 with BOM for native Arabic Excel rendering.
func (h *EnterpriseHandler) ExportSubscribersCSV(c *fiber.Ctx) error {
	rows, err := h.Pool.Query(c.Context(),
		`SELECT id, email, full_name, store_name, phone, role, wallet_balance, credit_limit, status, plan_name, subscription_ends_at, created_at
		 FROM users
		 WHERE role = 'subscriber'
		 ORDER BY id ASC`,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf("attachment; filename=\"subscribers_%s.csv\"", time.Now().Format("20060102_150405")))

	// Write UTF-8 BOM so Excel opens Arabic correctly
	c.Context().Response.SetBodyRaw([]byte("\xef\xbb\xbf"))
	writer := csv.NewWriter(c.Response().BodyWriter())

	_ = writer.Write([]string{"المعرف", "البريد الإلكتروني", "الاسم الكامل", "اسم المتجر", "رقم الهاتف", "الرتبة", "رصيد المحفظة (د.ع)", "سقف الائتمان (د.ع)", "الحالة", "الباقة", "تاريخ انتهاء الاشتراك", "تاريخ الانضمام"})

	for rows.Next() {
		var (
			id            int64
			email, fn, sn, ph, role, status, plan string
			bal float64
			limit int64
			ends *time.Time
			cAt time.Time
		)
		if err := rows.Scan(&id, &email, &fn, &sn, &ph, &role, &bal, &limit, &status, &plan, &ends, &cAt); err == nil {
			endsStr := "غير محدد"
			if ends != nil {
				endsStr = ends.Format("2006-01-02 15:04:05")
			}
			_ = writer.Write([]string{
				strconv.FormatInt(id, 10),
				email,
				fn,
				sn,
				ph,
				role,
				strconv.FormatInt(int64(bal), 10),
				strconv.FormatInt(limit, 10),
				status,
				plan,
				endsStr,
				cAt.Format("2006-01-02 15:04:05"),
			})
		}
	}
	writer.Flush()
	return nil
}

// -----------------------------------------------------------------------------
// Stage 1: Financial Security Admin APIs (Guard, Idem, Limits)
// -----------------------------------------------------------------------------

func (h *EnterpriseHandler) ListGuardSettings(c *fiber.Ctx) error {
	merchantID := int64(1) // Default system merchant / admin
	if mParam := c.Query("merchant_id"); mParam != "" {
		if m, err := strconv.ParseInt(mParam, 10, 64); err == nil && m > 0 {
			merchantID = m
		}
	}

	if h.GuardSvc == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "Guard service uninitialized"})
	}

	settings, err := h.GuardSvc.ListSettings(c.Context(), merchantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "ok", "settings": settings})
}

func (h *EnterpriseHandler) SaveGuardSetting(c *fiber.Ctx) error {
	var g guard.GuardSetting
	if err := c.BodyParser(&g); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "بيانات غير صالحة"})
	}

	if g.MerchantID <= 0 {
		g.MerchantID = 1
	}
	if g.ItemID == "" {
		g.ItemID = "*"
	}

	if h.GuardSvc == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "Guard service uninitialized"})
	}

	if err := h.GuardSvc.SaveSetting(c.Context(), &g); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	uid := middleware.UserID(c)
	cl := middleware.ClaimsFrom(c)
	writeAudit(c.Context(), h.Pool, uid, cl.Email, "GUARD_SETTING_SAVE", "guard_settings", g.ItemID, map[string]any{
		"min_margin_iqd":     g.MinMarginIQD,
		"min_margin_percent": g.MinMarginPercent,
		"is_active":          g.IsActive,
	}, c.IP())

	return c.JSON(fiber.Map{"status": "ok", "setting": g})
}

func (h *EnterpriseHandler) DeleteGuardSetting(c *fiber.Ctx) error {
	merchantID := int64(1)
	if mParam := c.Query("merchant_id"); mParam != "" {
		if m, err := strconv.ParseInt(mParam, 10, 64); err == nil && m > 0 {
			merchantID = m
		}
	}
	itemID := c.Params("item_id")

	if h.GuardSvc == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "Guard service uninitialized"})
	}

	if err := h.GuardSvc.DeleteSetting(c.Context(), merchantID, itemID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "ok", "message": "تم حذف هامش الأمان بنجاح"})
}

func (h *EnterpriseHandler) ListLimitsBlocks(c *fiber.Ctx) error {
	if h.LimitsSvc == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "Limits service uninitialized"})
	}

	blocks, err := h.LimitsSvc.ListBlocks(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "ok", "blocks": blocks})
}

func (h *EnterpriseHandler) UnblockLimit(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "معرف حظر غير صالح"})
	}

	if h.LimitsSvc == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "Limits service uninitialized"})
	}

	if err := h.LimitsSvc.Unblock(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	uid := middleware.UserID(c)
	cl := middleware.ClaimsFrom(c)
	writeAudit(c.Context(), h.Pool, uid, cl.Email, "LIMITS_UNBLOCK", "limits_blocks", strconv.FormatInt(id, 10), map[string]any{
		"block_id": id,
	}, c.IP())

	return c.JSON(fiber.Map{"status": "ok", "message": "تم فك الحظر بنجاح"})
}

func (h *EnterpriseHandler) BlockLimitManual(c *fiber.Ctx) error {
	var req struct {
		MerchantID int64  `json:"merchant_id"`
		IP         string `json:"ip"`
		Identifier string `json:"identifier"`
		ActionType string `json:"action_type"`
		DurationM  int    `json:"duration_minutes"`
		Reason     string `json:"reason"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "بيانات غير صالحة"})
	}
	if req.DurationM <= 0 {
		req.DurationM = 60
	}
	if req.ActionType == "" {
		req.ActionType = "manual_admin"
	}
	if req.Reason == "" {
		req.Reason = "حظر يدوي من الإدارة"
	}

	if h.LimitsSvc == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "Limits service uninitialized"})
	}

	_, blockedUntil, err := h.LimitsSvc.RecordFailedAttempt(
		c.Context(),
		req.MerchantID,
		req.IP,
		req.Identifier,
		req.ActionType,
		1, // Immediate block
		time.Duration(req.DurationM)*time.Minute,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	uid := middleware.UserID(c)
	cl := middleware.ClaimsFrom(c)
	writeAudit(c.Context(), h.Pool, uid, cl.Email, "LIMITS_MANUAL_BLOCK", "limits_blocks", req.Identifier, map[string]any{
		"ip":            req.IP,
		"merchant_id":   req.MerchantID,
		"blocked_until": blockedUntil,
		"reason":        req.Reason,
	}, c.IP())

	return c.JSON(fiber.Map{"status": "ok", "blocked_until": blockedUntil})
}

