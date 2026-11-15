package handlers

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"venus_backend/internal/middleware"
	"venus_backend/internal/notify"
	"venus_backend/internal/reports"
	"venus_backend/internal/seal"
)

type Stage3Handler struct {
	SealSvc    *seal.Service
	NotifySvc  *notify.Service
	ReportsSvc *reports.Service
}

func NewStage3Handler(sealSvc *seal.Service, notifySvc *notify.Service, reportsSvc *reports.Service) *Stage3Handler {
	return &Stage3Handler{
		SealSvc:    sealSvc,
		NotifySvc:  notifySvc,
		ReportsSvc: reportsSvc,
	}
}

// VerifyReceipt handles public GET /verify/:token
func (h *Stage3Handler) VerifyReceipt(c *fiber.Ctx) error {
	token := c.Params("token")
	if token == "" {
		return c.Status(fiber.StatusBadRequest).SendString("رمز التحقق غير صالح")
	}

	result, err := h.SealSvc.VerifyToken(c.Context(), token)
	if err != nil {
		if c.Accepts("text/html") != "" {
			c.Set("Content-Type", "text/html; charset=utf-8")
			html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="ar" dir="rtl">
<head><meta charset="UTF-8"><title>تحقق الفاتورة - غير صالحة</title><meta name="viewport" content="width=device-width, initial-scale=1.0"><script src="https://cdn.tailwindcss.com"></script></head>
<body class="bg-slate-50 flex items-center justify-center min-h-screen p-4">
  <div class="bg-white max-w-sm w-full p-6 rounded-3xl shadow-lg border border-rose-100 text-center space-y-4">
    <div class="w-16 h-16 mx-auto bg-rose-50 text-rose-500 rounded-2xl flex items-center justify-center text-3xl font-black">✕</div>
    <h1 class="text-xl font-black text-slate-800">فاتورة غير معتمدة</h1>
    <p class="text-xs text-slate-500">تعذر التحقق من التوقيع الرقمي لهذه الفاتورة، أو أن الرمز غير موجود أو منتهي الصلاحية.</p>
  </div>
</body></html>`)
			return c.Status(fiber.StatusNotFound).SendString(html)
		}
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"valid":   false,
			"message": "رمز التحقق غير موجود أو غير صالح",
		})
	}

	if c.Accepts("text/html") != "" {
		c.Set("Content-Type", "text/html; charset=utf-8")
		html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="ar" dir="rtl">
<head>
  <meta charset="UTF-8">
  <title>فاتورة موثقة - أوروك POS</title>
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <script src="https://cdn.tailwindcss.com"></script>
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.4.0/css/all.min.css">
</head>
<body class="bg-slate-50 flex items-center justify-center min-h-screen p-4">
  <div class="bg-white max-w-sm w-full p-6 rounded-3xl shadow-xl border border-emerald-100 text-center space-y-4">
    <div class="w-16 h-16 mx-auto bg-emerald-50 text-emerald-600 rounded-2xl flex items-center justify-center text-3xl font-black shadow-inner">
      <i class="fa-solid fa-circle-check"></i>
    </div>
    <div>
      <span class="px-2.5 py-1 rounded-full text-[10px] font-black bg-emerald-100 text-emerald-800 uppercase tracking-wider">فاتورة أصلية معتمدة وموقعة</span>
      <h1 class="text-xl font-black text-slate-900 mt-2">%s</h1>
      <p class="text-xs text-slate-400 font-bold">%s</p>
    </div>
    <div class="h-px bg-slate-100 my-2"></div>
    <div class="bg-slate-50 p-4 rounded-2xl space-y-2 text-right text-xs">
      <div class="flex justify-between"><span class="text-slate-400 font-bold">رقم الإيصال:</span><span class="font-mono font-black text-slate-800">%s</span></div>
      <div class="flex justify-between"><span class="text-slate-400 font-bold">المبلغ المدفوع:</span><span class="font-black text-emerald-600 text-sm">%d د.ع</span></div>
      <div class="flex justify-between"><span class="text-slate-400 font-bold">نوع العملية:</span><span class="font-bold text-slate-700">%s</span></div>
      <div class="flex justify-between"><span class="text-slate-400 font-bold">توقيت بغداد:</span><span class="font-mono text-slate-600 text-[11px]">%s</span></div>
    </div>
    <div class="text-[10px] text-slate-400 font-bold pt-2 flex items-center justify-center gap-1.5">
      <i class="fa-solid fa-shield-halved text-brand"></i>
      <span>موقعة تشفيرياً عبر HMAC-SHA256 • أوروك للحلول المالية</span>
    </div>
  </div>
</body></html>`,
			result.StoreName, result.MerchantName, result.ReceiptID, result.Amount, result.ReceiptType, result.BaghdadTime,
		)
		return c.SendString(html)
	}

	return c.JSON(result)
}

// CreateSealToken handles POST /api/seal/create
func (h *Stage3Handler) CreateSealToken(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	if uid <= 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "غير مصرح"})
	}

	var req struct {
		ReceiptType string `json:"receipt_type"`
		ReceiptID   string `json:"receipt_id"`
		Amount      int64  `json:"amount"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	token, err := h.SealSvc.CreateToken(c.Context(), uid, req.ReceiptType, req.ReceiptID, req.Amount)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"token": token.Token,
		"url":   h.SealSvc.FormatVerificationURL(token.Token),
	})
}

// ListNotifyChannels handles GET /api/admin/notify/channels
func (h *Stage3Handler) ListNotifyChannels(c *fiber.Ctx) error {
	list, err := h.NotifySvc.ListChannels(c.Context(), 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(list)
}

// SaveNotifyChannel handles POST /api/admin/notify/channels
func (h *Stage3Handler) SaveNotifyChannel(c *fiber.Ctx) error {
	var req struct {
		ChannelType string                 `json:"channel_type"`
		Config      map[string]interface{} `json:"config"`
		Priority    int                    `json:"priority"`
		IsActive    bool                   `json:"is_active"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.NotifySvc.SaveChannel(c.Context(), 0, req.ChannelType, req.Config, req.Priority, req.IsActive); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true})
}

// DeleteNotifyChannel handles DELETE /api/admin/notify/channels/:id
func (h *Stage3Handler) DeleteNotifyChannel(c *fiber.Ctx) error {
	id, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	if err := h.NotifySvc.DeleteChannel(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true})
}

// SendReportsNow handles POST /api/admin/reports/send-now
func (h *Stage3Handler) SendReportsNow(c *fiber.Ctx) error {
	summary, err := h.ReportsSvc.SendReport(c.Context(), 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"success": true,
		"summary": summary,
		"message": "تم توليد وإرسال التقرير اليومي بنجاح",
	})
}
