package handlers

import (
	"github.com/gofiber/fiber/v2"

	"venus_backend/internal/middleware"
	"venus_backend/internal/tills"
)

type Stage4Handler struct {
	TillsSvc *tills.Service
}

func NewStage4Handler(tillsSvc *tills.Service) *Stage4Handler {
	return &Stage4Handler{
		TillsSvc: tillsSvc,
	}
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
