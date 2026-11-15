package handlers

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	apiPkg "venus_backend/internal/api"
	"venus_backend/internal/forecast"
	"venus_backend/internal/middleware"
	"venus_backend/internal/restock"
	"venus_backend/internal/rewards"
	"venus_backend/internal/risk"
	"venus_backend/internal/surge"
	"venus_backend/internal/tills"
)

type Stage4Handler struct {
	RewardsSvc  *rewards.Service
	RestockSvc  *restock.Service
	RiskSvc     *risk.Service
	ForecastSvc *forecast.Service
	TillsSvc    *tills.Service
	ApiSvc      *apiPkg.Service
	SurgeSvc    *surge.Service
}

func NewStage4Handler(
	rewardsSvc *rewards.Service,
	restockSvc *restock.Service,
	riskSvc *risk.Service,
	forecastSvc *forecast.Service,
	tillsSvc *tills.Service,
	apiSvc *apiPkg.Service,
	surgeSvc *surge.Service,
) *Stage4Handler {
	return &Stage4Handler{
		RewardsSvc:  rewardsSvc,
		RestockSvc:  restockSvc,
		RiskSvc:     riskSvc,
		ForecastSvc: forecastSvc,
		TillsSvc:    tillsSvc,
		ApiSvc:      apiSvc,
		SurgeSvc:    surgeSvc,
	}
}

// GetForecast handles GET /forecast (liquidity projection for next week)
func (h *Stage4Handler) GetForecast(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	if uid <= 0 {
		uid = 1
	}
	f, err := h.ForecastSvc.GetForecast(c.Context(), uid)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(f)
}

// GetSurgeQuote handles GET /api/surge/quote?base_price=X
func (h *Stage4Handler) GetSurgeQuote(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	basePrice, _ := strconv.ParseInt(c.Query("base_price", "0"), 10, 64)
	quote := h.SurgeSvc.GetQuote(c.Context(), uid, basePrice)
	return c.JSON(quote)
}

// GetActiveShift handles GET /api/tills/active
func (h *Stage4Handler) GetActiveShift(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	if uid <= 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "غير مصرح"})
	}

	shift, err := h.TillsSvc.GetActiveShift(c.Context(), uid)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	role := "cashier"
	if cl := middleware.ClaimsFrom(c); cl != nil && cl.Role != "" {
		role = cl.Role
	}
	perms := tills.GetPermissions(role)

	return c.JSON(fiber.Map{
		"shift":       shift,
		"has_shift":   shift != nil,
		"permissions": perms,
	})
}

// OpenShift handles POST /api/tills/open
func (h *Stage4Handler) OpenShift(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	if uid <= 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "غير مصرح"})
	}

	var req struct {
		DeviceID     string `json:"device_id"`
		StartingCash int64  `json:"starting_cash"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	shift, err := h.TillsSvc.OpenShift(c.Context(), uid, uid, req.DeviceID, req.StartingCash)
	if err != nil {
		if err == tills.ErrShiftAlreadyOpen {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "يوجد وردية مفتوحة بالفعل لهذا الكاشير"})
		}
		if err == tills.ErrDeviceMismatch {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "الحساب مقفل على جهاز آخر، لا يمكن فتح الوردية"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "shift": shift})
}

// CloseShift handles POST /api/tills/close
func (h *Stage4Handler) CloseShift(c *fiber.Ctx) error {
	var req struct {
		ShiftID    int64  `json:"shift_id"`
		ActualCash int64  `json:"actual_cash"`
		Note       string `json:"note"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	shift, err := h.TillsSvc.CloseShift(c.Context(), req.ShiftID, req.ActualCash, req.Note)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "shift": shift})
}

// GetOpenAPISpec handles GET /api/docs/openapi.json
func (h *Stage4Handler) GetOpenAPISpec(c *fiber.Ctx) error {
	return c.JSON(h.ApiSvc.GetOpenAPISpec())
}

// Admin: Rewards
func (h *Stage4Handler) ListRewardRules(c *fiber.Ctx) error {
	rules, err := h.RewardsSvc.ListRules(c.Context(), 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(rules)
}

func (h *Stage4Handler) SaveRewardRule(c *fiber.Ctx) error {
	var r rewards.RewardRule
	if err := c.BodyParser(&r); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.RewardsSvc.SaveRule(c.Context(), r); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true})
}

func (h *Stage4Handler) EvaluateReward(c *fiber.Ctx) error {
	merchantID, _ := strconv.ParseInt(c.Params("merchant_id"), 10, 64)
	ruleID, _ := strconv.ParseInt(c.Params("rule_id"), 10, 64)

	res, err := h.RewardsSvc.EvaluateAndReward(c.Context(), merchantID, ruleID)
	if err != nil {
		if err == rewards.ErrRewardAlreadyClaimed {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "تم صرف المكافأة مسبقاً لهذه الفترة"})
		}
		if strings.Contains(err.Error(), "target has not been reached") {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "claim": res})
}

// Admin: Restock
func (h *Stage4Handler) ListRestockOrders(c *fiber.Ctx) error {
	orders, err := h.RestockSvc.ListOrders(c.Context(), 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(orders)
}

func (h *Stage4Handler) TriggerRestock(c *fiber.Ctx) error {
	var req struct {
		MerchantID int64  `json:"merchant_id"`
		Provider   string `json:"provider"`
		ItemID     string `json:"item_id"`
		Quantity   int    `json:"quantity"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	orderID, err := h.RestockSvc.AutoRestock(c.Context(), req.MerchantID, req.Provider, req.ItemID, req.Quantity)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "order_id": orderID})
}

// Admin: Risk
func (h *Stage4Handler) ListRiskProfiles(c *fiber.Ctx) error {
	profiles, err := h.RiskSvc.ListRiskProfiles(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(profiles)
}

func (h *Stage4Handler) OverrideRisk(c *fiber.Ctx) error {
	merchantID, _ := strconv.ParseInt(c.Params("merchant_id"), 10, 64)
	var req struct {
		NewScore int  `json:"new_score"`
		Unfreeze bool `json:"unfreeze"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.RiskSvc.OverrideRiskProfile(c.Context(), merchantID, req.NewScore, req.Unfreeze); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true})
}

// Admin: Surge
func (h *Stage4Handler) GetSurgeConfig(c *fiber.Ctx) error {
	cfg := h.SurgeSvc.GetConfig(c.Context(), 0)
	return c.JSON(cfg)
}

func (h *Stage4Handler) UpdateSurgeConfig(c *fiber.Ctx) error {
	var cfg surge.SurgeConfig
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.SurgeSvc.UpdateConfig(c.Context(), cfg); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true})
}

// Admin: API Keys v2
func (h *Stage4Handler) ListApiKeys(c *fiber.Ctx) error {
	keys, err := h.ApiSvc.ListKeys(c.Context(), 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(keys)
}

func (h *Stage4Handler) GenerateApiKey(c *fiber.Ctx) error {
	var req struct {
		Label     string   `json:"label"`
		Scopes    []string `json:"scopes"`
		RateLimit int      `json:"rate_limit"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	res, err := h.ApiSvc.GenerateKey(c.Context(), 1, req.Label, req.Scopes, req.RateLimit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(res)
}

func (h *Stage4Handler) RevokeApiKey(c *fiber.Ctx) error {
	id, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	if err := h.ApiSvc.RevokeKey(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true})
}
