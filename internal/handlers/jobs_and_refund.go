package handlers

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"venus_backend/internal/jobs"
	"venus_backend/internal/middleware"
	"venus_backend/internal/refund"
)

type JobsAndRefundHandler struct {
	JobsSvc   *jobs.Service
	RefundSvc *refund.Service
}

func NewJobsAndRefundHandler(jobsSvc *jobs.Service, refundSvc *refund.Service) *JobsAndRefundHandler {
	return &JobsAndRefundHandler{
		JobsSvc:   jobsSvc,
		RefundSvc: refundSvc,
	}
}

// PostRefund handles POST /api/refund
func (h *JobsAndRefundHandler) PostRefund(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	if uid <= 0 {
		uid = extractUserIDFromHeaders(c)
	}
	if uid <= 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "المستخدم غير مسجل",
		})
	}

	var req refund.RefundRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "بيانات الطلب غير صالحة: " + err.Error(),
		})
	}

	// Default to calling merchant if not admin overriding
	if req.MerchantID <= 0 {
		req.MerchantID = uid
	}

	// Idempotency key from header if not in body
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = strings.TrimSpace(c.Get("Idempotency-Key"))
	}

	result, err := h.RefundSvc.ProcessRefund(c.Context(), req)
	if err != nil {
		if err == refund.ErrInvalidRefundAmount {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
				"error": "مبلغ الإرجاع يجب أن يكون أكبر من الصفر",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "فشل تنفيذ عملية الإرجاع المالي: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"refund":  result,
	})
}

// ListDeadJobs handles GET /api/admin/jobs/dead
func (h *JobsAndRefundHandler) ListDeadJobs(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	list, err := h.JobsSvc.ListDeadJobs(c.Context(), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "فشل جلب قائمة المهام المتعثرة: " + err.Error(),
		})
	}

	count, _ := h.JobsSvc.CountDeadJobs(c.Context())

	return c.JSON(fiber.Map{
		"total": count,
		"items": list,
	})
}

// RetryDeadJob handles POST /api/admin/jobs/:id/retry
func (h *JobsAndRefundHandler) RetryDeadJob(c *fiber.Ctx) error {
	idParam := c.Params("id")
	jobID, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil || jobID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "معرف المهمة غير صالح",
		})
	}

	if err := h.JobsSvc.RetryJob(c.Context(), jobID); err != nil {
		if err == jobs.ErrJobNotDead {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "المهمة ليست في قائمة المهام المتعثرة (Dead)",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "فشل إعادة تشغيل المهمة: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "تمت إعادة جدولة المهمة بنجاح",
	})
}
