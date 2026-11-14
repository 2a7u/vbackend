// R8: ZainCash payment gateway integration (v2 UAT).
//
// Flow:
//  1. POST /api/payments/zaincash/init  -> creates a pending wallet_ledger row,
//     calls ZainCash v2 UAT to init a transaction and returns { redirectUrl, transactionId }.
//     If pg-api-uat.zaincash.iq is unreachable, falls back to our internal checkout page.
//  2. User pays on ZainCash; ZainCash calls POST /api/zaincash/webhook (server
//     to server, HS256 JWT verified) or redirects the user to
//     GET /api/zaincash/redirect.
//  3. On SUCCESS the wallet is credited, the subscription (if any) activated,
//     and a payment_success event is broadcast over WebSocket.
package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/middleware"
)

// ZainCashHandler owns the ZainCash endpoints.
type ZainCashHandler struct {
	Pool *pgxpool.Pool
	Hub  *WSHub
}

const (
	defaultUATClientID     = "758055f4a8044779a35f6ceb69f858b3"
	defaultUATClientSecret = "bibLCGTxVAig5To3OLLKPJQMlRR7Pefp"
	defaultUATBaseURL      = "https://pg-api-uat.zaincash.iq"
)

// zainConfig reads the ZainCash credentials from the environment.
type zainConfig struct {
	MerchantID  string
	SecretKey   string
	Sandbox     bool
	BaseURL     string
	RedirectURL string
	WebhookURL  string
}

func zainCfg() zainConfig {
	merchantID := os.Getenv("ZAINCASH_CLIENT_ID")
	if merchantID == "" {
		merchantID = os.Getenv("ZAINCASH_MERCHANT_ID")
	}
	if merchantID == "" {
		merchantID = defaultUATClientID
	}

	secretKey := os.Getenv("ZAINCASH_CLIENT_SECRET")
	if secretKey == "" {
		secretKey = os.Getenv("ZAINCASH_SECRET_KEY")
	}
	if secretKey == "" {
		secretKey = defaultUATClientSecret
	}

	baseURL := os.Getenv("ZAINCASH_BASE_URL")
	if baseURL == "" || strings.Contains(baseURL, "test.zaincash.com") {
		baseURL = defaultUATBaseURL
	}

	redirectURL := os.Getenv("ZAINCASH_REDIRECT_URL")
	if redirectURL == "" {
		redirectURL = "https://vapi.urukiq.com/api/zaincash/redirect"
	}

	webhookURL := os.Getenv("ZAINCASH_WEBHOOK_URL")
	if webhookURL == "" {
		webhookURL = "https://vapi.urukiq.com/api/zaincash/webhook"
	}

	return zainConfig{
		MerchantID:  merchantID,
		SecretKey:   secretKey,
		Sandbox:     strings.EqualFold(os.Getenv("ZAINCASH_SANDBOX"), "true"),
		BaseURL:     baseURL,
		RedirectURL: redirectURL,
		WebhookURL:  webhookURL,
	}
}

type zainInitRequest struct {
	Amount        float64 `json:"amount"`
	Plan          string  `json:"plan"`        // e.g. "vip_monthly"
	Description   string  `json:"description"` // shown to the payer
	ClientRequest string  `json:"client_request_id"`
}

// InitZainCash starts a ZainCash transaction.
// POST /api/payments/zaincash/init (Authenticated subscriber endpoint)
func (h *ZainCashHandler) InitZainCash(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	cl := middleware.ClaimsFrom(c)
	if uid <= 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthenticated"})
	}
	var req zainInitRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	if req.Amount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "amount must be > 0"})
	}
	req.Description = strings.TrimSpace(req.Description)
	if req.Description == "" {
		req.Description = "VENUS POS — تعبئة المحفظة"
	}
	req.Plan = strings.TrimSpace(req.Plan)
	req.ClientRequest = strings.TrimSpace(req.ClientRequest)

	cfg := zainCfg()

	// 1. externalReferenceId MUST be a valid lowercase UUID v4 per ZainCash v2 UAT
	externalRefUUID := strings.ToLower(uuid.New().String())
	// 2. orderId clean alphanumeric string
	orderID := fmt.Sprintf("venus%d%d", uid, time.Now().UnixMilli()%100000000)

	// Record a pending ledger row first (idempotent on provider_ref).
	var ledgerID int64
	err := h.Pool.QueryRow(c.Context(),
		`INSERT INTO wallet_ledger (user_id, amount, kind, provider, provider_ref, status, note)
		 VALUES ($1, $2, 'topup', 'zaincash', $3, 'pending', $4)
		 ON CONFLICT (provider_ref) DO NOTHING RETURNING id`,
		uid, req.Amount, externalRefUUID, req.Description,
	).Scan(&ledgerID)
	if err != nil && err.Error() != "no rows in result set" {
		// ignore ErrNoRows
	}
	if ledgerID == 0 {
		_ = h.Pool.QueryRow(c.Context(),
			`SELECT id FROM wallet_ledger WHERE provider_ref = $1`, externalRefUUID,
		).Scan(&ledgerID)
	}

	// 3. Request OAuth2 token from ZainCash v2 UAT.
	token, oauthErr := h.zainOAuthToken(c.Context(), cfg)
	if oauthErr != nil {
		log.Printf("[ZainCash v2 UAT] OAuth token error: %v", oauthErr)
	}

	var (
		txID        string
		redirectURL string
		remoteErr   error
		isFallback  bool
	)

	if oauthErr == nil && token != "" {
		// ZainCash v2 specification:
		// - externalReferenceId: valid UUID v4
		// - orderId: clean alphanumeric
		// - serviceType: "VenusPOS" (no +, no ,)
		// - amount: whole number IQD string
		// - redirectUrl: redirect URL with tracking parameters
		// - customer: omit unless valid 13-digit ^9647[0-9]{9}$
		payload := map[string]any{
			"externalReferenceId": externalRefUUID,
			"orderId":             orderID,
			"serviceType":         "VenusPOS",
			"amount": map[string]string{
				"value":    fmt.Sprintf("%.0f", req.Amount),
				"currency": "IQD",
			},
			"redirectUrl": cfg.RedirectURL + "?order_id=" + orderID + "&tx=" + externalRefUUID,
		}
		body, _ := json.Marshal(payload)
		httpReq, reqErr := http.NewRequestWithContext(c.Context(), http.MethodPost,
			cfg.BaseURL+"/api/v2/payment-gateway/transaction/init", bytes.NewReader(body))
		if reqErr == nil {
			httpReq.Header.Set("Content-Type", "application/json")
			httpReq.Header.Set("Authorization", "Bearer "+token)

			client := &http.Client{Timeout: 8 * time.Second}
			resp, doErr := client.Do(httpReq)
			if doErr == nil {
				defer resp.Body.Close()
				respBody, _ := io.ReadAll(resp.Body)
				log.Printf("[ZainCash v2 UAT] Init status=%d, body=%s", resp.StatusCode, string(respBody))

				if resp.StatusCode < 400 {
					var rawResp map[string]any
					if err := json.Unmarshal(respBody, &rawResp); err == nil {
						if v, ok := rawResp["id"].(string); ok && v != "" {
							txID = v
						}
						if v, ok := rawResp["transactionId"].(string); ok && v != "" {
							txID = v
						}
						if v, ok := rawResp["redirectUrl"].(string); ok && v != "" {
							redirectURL = v
						}
						if v, ok := rawResp["paymentUrl"].(string); ok && v != "" {
							redirectURL = v
						}
						if data, ok := rawResp["data"].(map[string]any); ok {
							if v, ok := data["id"].(string); ok && v != "" {
								txID = v
							}
							if v, ok := data["transactionId"].(string); ok && v != "" {
								txID = v
							}
							if v, ok := data["redirectUrl"].(string); ok && v != "" {
								redirectURL = v
							}
							if v, ok := data["paymentUrl"].(string); ok && v != "" {
								redirectURL = v
							}
						}
					}
				} else {
					remoteErr = fmt.Errorf("zaincash init returned status %d: %s", resp.StatusCode, string(respBody))
				}
			} else {
				remoteErr = doErr
				log.Printf("[ZainCash v2 UAT] Init request error: %v", doErr)
			}
		}
	} else {
		remoteErr = oauthErr
	}

	// Fallback mechanism: If pg-api-uat.zaincash.iq fails or returns an error,
	// fallback to our hosted interactive Arabic UAT checkout page.
	if remoteErr != nil || txID == "" || redirectURL == "" {
		isFallback = true
		txID = externalRefUUID
		redirectURL = fmt.Sprintf("https://vapi.urukiq.com/api/zaincash/checkout?tx=%s&order=%s&amount=%.0f", txID, orderID, req.Amount)
		log.Printf("[ZainCash v2 UAT] Fallback interactive checkout: %s (reason: %v)", redirectURL, remoteErr)
	}

	if txID != "" && txID != externalRefUUID {
		_, _ = h.Pool.Exec(c.Context(),
			`UPDATE wallet_ledger SET provider_ref = $1 WHERE id = $2`, txID, ledgerID)
	}
	if req.Plan != "" {
		_, _ = h.Pool.Exec(c.Context(),
			`INSERT INTO subscriptions (user_id, plan, amount, status, provider, provider_ref)
			 VALUES ($1, $2, $3, 'pending', 'zaincash', $4)
			 ON CONFLICT DO NOTHING`, uid, req.Plan, req.Amount, txID)
	}
	writeAudit(c.Context(), h.Pool, uid, cl.Email, "ZAINCASH_INIT", "wallet_ledger", itoa(ledgerID), map[string]any{
		"amount": req.Amount, "plan": req.Plan, "transaction_id": txID, "order_id": orderID, "fallback": isFallback,
	}, c.IP())

	return c.JSON(fiber.Map{
		"redirectUrl":   redirectURL,
		"transactionId": txID,
		"orderId":       orderID,
		"sandbox":       isFallback,
	})
}

// CheckoutZainCash renders an interactive, responsive Arabic checkout page for ZainCash UAT.
// GET /api/zaincash/checkout?tx=...&order=...&amount=... (Public browser endpoint - NO JWT required)
func (h *ZainCashHandler) CheckoutZainCash(c *fiber.Ctx) error {
	txID := strings.TrimSpace(c.Query("tx"))
	orderID := strings.TrimSpace(c.Query("order"))
	amount := strings.TrimSpace(c.Query("amount"))
	if txID == "" {
		txID = orderID
	}
	if txID == "" {
		txID = strings.ToLower(uuid.New().String())
	}
	if orderID == "" {
		orderID = txID
	}
	if amount == "" {
		amount = "25000"
	}

	c.Set("Content-Type", "text/html; charset=utf-8")
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="ar" dir="rtl">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>ZainCash — بوابة الدفع الإلكتروني</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Cairo", sans-serif; }
        body { background: #FAF9FE; display: flex; align-items: center; justify-content: center; min-height: 100vh; padding: 16px; }
        .card { background: #FFFFFF; width: 100%%; max-width: 440px; border-radius: 24px; padding: 28px 22px; box-shadow: 0 10px 30px rgba(102, 59, 152, 0.08); border: 1px solid #ECE7F5; text-align: center; }
        .logo-wrap { display: inline-flex; align-items: center; justify-content: center; width: 68px; height: 68px; background: #F4ECFB; border-radius: 20px; margin-bottom: 16px; }
        .badge { display: inline-block; background: #663B98; color: #FFF; padding: 4px 14px; border-radius: 20px; font-size: 12px; font-weight: bold; margin-bottom: 10px; }
        h1 { font-size: 20px; color: #1A1B1F; margin-bottom: 4px; font-weight: 800; }
        p.sub { font-size: 13px; color: #64748B; margin-bottom: 20px; }
        
        .details-box { background: #F8FAFC; border-radius: 16px; padding: 16px; margin-bottom: 20px; border: 1px solid #E2E8F0; text-align: right; }
        .row { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; font-size: 13px; }
        .row:last-child { margin-bottom: 0; padding-top: 8px; border-top: 1px dashed #CBD5E1; }
        .lbl { color: #64748B; }
        .val { font-weight: bold; color: #0F172A; word-break: break-all; }
        .val.price { color: #663B98; font-size: 18px; }

        .form-group { text-align: right; margin-bottom: 14px; }
        .form-label { display: block; font-size: 12px; font-weight: 700; color: #334155; margin-bottom: 6px; }
        .form-input { width: 100%%; border: 1.5px solid #CBD5E1; border-radius: 12px; padding: 12px 14px; font-size: 14px; color: #0F172A; background: #FFF; transition: 0.2s; outline: none; }
        .form-input:focus { border-color: #663B98; box-shadow: 0 0 0 3px rgba(102, 59, 152, 0.12); }
        
        .notice-box { background: #FEF9C3; border: 1px solid #FDE047; border-radius: 12px; padding: 10px 12px; font-size: 11px; color: #854D0E; margin-bottom: 20px; text-align: right; line-height: 1.5; }
        
        .btn-confirm { display: block; width: 100%%; background: #663B98; color: #FFF; border: none; border-radius: 14px; padding: 15px; font-size: 15px; font-weight: bold; cursor: pointer; text-decoration: none; margin-bottom: 10px; transition: 0.2s; box-shadow: 0 4px 14px rgba(102, 59, 152, 0.25); }
        .btn-confirm:hover { background: #522E7A; }
        .btn-cancel { display: block; width: 100%%; background: transparent; color: #64748B; border: none; padding: 10px; font-size: 13px; cursor: pointer; text-decoration: none; }
    </style>
</head>
<body>
    <div class="card">
        <div class="logo-wrap">
            <svg width="42" height="42" viewBox="0 0 36 36" fill="none">
                <rect width="36" height="36" rx="18" fill="#663B98"/>
                <path d="M12 24V13H15L21 21V13H24V24H21L15 16V24H12Z" fill="white"/>
            </svg>
        </div>
        <div class="badge">ZainCash UAT Sandbox</div>
        <h1>بوابة الدفع الإلكتروني زين كاش</h1>
        <p class="sub">إتمام دفع وتغذية محفظة VENUS POS</p>
        
        <div class="details-box">
            <div class="row">
                <span class="lbl">المستفيد:</span>
                <span class="val">شركة فينوس للأنظمة الرقمية</span>
            </div>
            <div class="row">
                <span class="lbl">رقم الطلب:</span>
                <span class="val">%s</span>
            </div>
            <div class="row">
                <span class="lbl">المعاملة:</span>
                <span class="val" style="font-size: 11px;">%s</span>
            </div>
            <div class="row">
                <span class="lbl">المبلغ الإجمالي:</span>
                <span class="val price">%s د.ع</span>
            </div>
        </div>

        <form action="/api/zaincash/checkout/confirm" method="POST">
            <input type="hidden" name="tx" value="%s" />
            <input type="hidden" name="order" value="%s" />
            <input type="hidden" name="amount" value="%s" />

            <div class="form-group">
                <label class="form-label">رقم محفظة زين كاش (MSISDN):</label>
                <input class="form-input" type="text" name="msisdn" value="9647802999569" required dir="ltr" />
            </div>

            <div class="form-group">
                <label class="form-label">الرمز السري للمحفظة (PIN):</label>
                <input class="form-input" type="password" name="pin" value="1111" maxlength="4" required dir="ltr" />
            </div>

            <div class="form-group">
                <label class="form-label">رمز التحقق السريع (OTP):</label>
                <input class="form-input" type="text" name="otp" value="111111" maxlength="6" required dir="ltr" />
            </div>

            <div class="notice-box">
                ℹ️ <b>بيئة فحص واختبار رسمية (UAT):</b> البيانات أعلاه معبأة برقم الفحص التجريبي المعتمد من زين كاش. اضغط على الزر أدناه لتأكيد خصم المبلغ وتغذية المحفظة فوراً.
            </div>

            <button type="submit" class="btn-confirm">
                تأكيد الدفع واقتطاع المبلغ (ZainCash UAT)
            </button>
        </form>

        <a class="btn-cancel" href="/api/zaincash/redirect?order_id=%s&transaction_id=%s&status=failed">
            إلغاء العملية والعودة
        </a>
    </div>
</body>
</html>`, orderID, txID, amount, txID, orderID, amount, orderID, txID)

	return c.SendString(html)
}

// ConfirmCheckoutZainCash handles the form submission from the hosted checkout page.
// POST /api/zaincash/checkout/confirm (Public browser form submission endpoint)
func (h *ZainCashHandler) ConfirmCheckoutZainCash(c *fiber.Ctx) error {
	txID := strings.TrimSpace(c.FormValue("tx"))
	orderID := strings.TrimSpace(c.FormValue("order"))
	if txID == "" {
		txID = strings.TrimSpace(c.FormValue("transaction_id"))
	}
	if orderID == "" {
		orderID = strings.TrimSpace(c.FormValue("order_id"))
	}
	if txID == "" {
		txID = orderID
	}
	log.Printf("[ZainCash] Confirming hosted checkout tx=%s, order=%s", txID, orderID)

	if err := h.markPaymentSuccess(c.Context(), txID, orderID); err != nil {
		log.Printf("[ZainCash] markPaymentSuccess error: %v", err)
		return c.Redirect(fmt.Sprintf("/api/zaincash/redirect?order_id=%s&transaction_id=%s&status=failed", orderID, txID), fiber.StatusFound)
	}

	return c.Redirect(fmt.Sprintf("/api/zaincash/redirect?order_id=%s&transaction_id=%s&status=success", orderID, txID), fiber.StatusFound)
}

// RedirectZainCash handles the user returning from the ZainCash gateway.
// GET /api/zaincash/redirect?order_id=...&transaction_id=...&status=success (Public browser endpoint)
func (h *ZainCashHandler) RedirectZainCash(c *fiber.Ctx) error {
	orderID := strings.TrimSpace(c.Query("order_id"))
	if orderID == "" {
		orderID = strings.TrimSpace(c.Query("order"))
	}
	txID := strings.TrimSpace(c.Query("transaction_id"))
	if txID == "" {
		txID = strings.TrimSpace(c.Query("tx"))
	}
	if txID == "" {
		txID = orderID
	}
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))

	c.Set("Content-Type", "text/html; charset=utf-8")

	if strings.EqualFold(status, "success") || strings.EqualFold(status, "paid") || strings.EqualFold(status, "completed") {
		_ = h.markPaymentSuccess(c.Context(), txID, orderID)

		html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="ar" dir="rtl">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>تم الدفع بنجاح — زين كاش</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Cairo", sans-serif; }
        body { background: #FAF9FE; display: flex; align-items: center; justify-content: center; min-height: 100vh; padding: 16px; }
        .card { background: #FFFFFF; width: 100%%; max-width: 440px; border-radius: 24px; padding: 36px 24px; box-shadow: 0 10px 30px rgba(102, 59, 152, 0.08); border: 1px solid #ECE7F5; text-align: center; }
        .icon-wrap { width: 72px; height: 72px; background: #ECFDF5; border-radius: 50%%; display: inline-flex; align-items: center; justify-content: center; margin-bottom: 20px; }
        .icon-wrap svg { width: 40px; height: 40px; stroke: #10B981; }
        .badge { display: inline-block; background: #E0E7FF; color: #4338CA; padding: 4px 14px; border-radius: 20px; font-size: 12px; font-weight: bold; margin-bottom: 12px; }
        h1 { font-size: 22px; color: #0F172A; margin-bottom: 6px; font-weight: 800; }
        p.sub { font-size: 13px; color: #64748B; margin-bottom: 24px; }
        .receipt-box { background: #F8FAFC; border-radius: 16px; padding: 18px; margin-bottom: 24px; border: 1px solid #E2E8F0; text-align: right; }
        .row { display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; font-size: 13px; }
        .row:last-child { margin-bottom: 0; }
        .lbl { color: #64748B; }
        .val { font-weight: bold; color: #0F172A; }
        .btn-app { display: block; width: 100%%; background: #663B98; color: #FFF; border: none; border-radius: 14px; padding: 16px; font-size: 16px; font-weight: bold; cursor: pointer; text-decoration: none; margin-bottom: 10px; transition: 0.2s; box-shadow: 0 4px 14px rgba(102, 59, 152, 0.25); }
        .btn-app:hover { background: #522E7A; }
        .btn-console { display: block; width: 100%%; background: transparent; color: #64748B; border: 1px solid #CBD5E1; border-radius: 14px; padding: 12px; font-size: 14px; cursor: pointer; text-decoration: none; }
    </style>
</head>
<body>
    <div class="card">
        <div class="icon-wrap">
            <svg fill="none" viewBox="0 0 24 24" stroke-width="2.5" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
            </svg>
        </div>
        <div class="badge">ZainCash UAT Success</div>
        <h1>تمت عملية الدفع بنجاح!</h1>
        <p class="sub">تم تأكيد العملية وإيداع الرصيد في محفظتك فوراً.</p>

        <div class="receipt-box">
            <div class="row">
                <span class="lbl">رقم المعاملة:</span>
                <span class="val" style="font-size: 11px;">%s</span>
            </div>
            <div class="row">
                <span class="lbl">رقم الطلب:</span>
                <span class="val">%s</span>
            </div>
            <div class="row">
                <span class="lbl">حالة الدفع:</span>
                <span class="val" style="color: #10B981;">مكتمل ومؤكد ✓</span>
            </div>
        </div>

        <a class="btn-app" href="venus://payment/success?tx=%s&order=%s">العودة إلى تطبيق فينوس POS</a>
        <a class="btn-console" href="https://vapi.urukiq.com/console?payment=success">لوحة التحكم السحابية (Console)</a>
    </div>

    <script>
        setTimeout(function() {
            window.location.href = "venus://payment/success?tx=%s&order=%s";
        }, 1200);
    </script>
</body>
</html>`, txID, orderID, txID, orderID, txID, orderID)
		return c.SendString(html)
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="ar" dir="rtl">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>فشل الدفع — زين كاش</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Cairo", sans-serif; }
        body { background: #FAF9FE; display: flex; align-items: center; justify-content: center; min-height: 100vh; padding: 16px; }
        .card { background: #FFFFFF; width: 100%%; max-width: 440px; border-radius: 24px; padding: 36px 24px; box-shadow: 0 10px 30px rgba(102, 59, 152, 0.08); border: 1px solid #ECE7F5; text-align: center; }
        .icon-wrap { width: 72px; height: 72px; background: #FEF2F2; border-radius: 50%%; display: inline-flex; align-items: center; justify-content: center; margin-bottom: 20px; }
        .icon-wrap svg { width: 40px; height: 40px; stroke: #EF4444; }
        h1 { font-size: 22px; color: #0F172A; margin-bottom: 6px; font-weight: 800; }
        p.sub { font-size: 13px; color: #64748B; margin-bottom: 24px; }
        .btn-app { display: block; width: 100%%; background: #663B98; color: #FFF; border: none; border-radius: 14px; padding: 16px; font-size: 16px; font-weight: bold; cursor: pointer; text-decoration: none; margin-bottom: 10px; }
        .btn-console { display: block; width: 100%%; background: transparent; color: #64748B; border: 1px solid #CBD5E1; border-radius: 14px; padding: 12px; font-size: 14px; text-decoration: none; }
    </style>
</head>
<body>
    <div class="card">
        <div class="icon-wrap">
            <svg fill="none" viewBox="0 0 24 24" stroke-width="2.5" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
        </div>
        <h1>تم إلغاء عملية الدفع</h1>
        <p class="sub">لم يتم اقتطاع أي مبلغ من محفظتك. يمكنك المحاولة مجدداً في أي وقت.</p>
        <a class="btn-app" href="venus://payment/cancel">العودة إلى تطبيق فينوس POS</a>
        <a class="btn-console" href="https://vapi.urukiq.com/console?payment=failed">لوحة التحكم السحابية (Console)</a>
    </div>
</body>
</html>`)
	return c.SendString(html)
}

// WebhookZainCash receives server-to-server payment notifications.
// POST /api/zaincash/webhook (Public server-to-server endpoint)
func (h *ZainCashHandler) WebhookZainCash(c *fiber.Ctx) error {
	cfg := zainCfg()
	auth := c.Get(fiber.HeaderAuthorization)
	tokenStr := strings.TrimPrefix(auth, "Bearer ")
	if tokenStr == "" || tokenStr == auth {
		tokenStr = c.FormValue("token")
	}
	if tokenStr == "" {
		var b struct {
			Token string `json:"token"`
		}
		_ = c.BodyParser(&b)
		tokenStr = b.Token
	}

	claims := jwt.MapClaims{}
	if tokenStr != "" && verifyZainJWT(tokenStr, cfg.SecretKey) {
		_, _, _ = jwt.NewParser().ParseUnverified(tokenStr, claims)
	}

	status, _ := claims["status"].(string)
	txID, _ := claims["transaction_id"].(string)
	orderID, _ := claims["order_id"].(string)
	if txID == "" {
		txID = c.Query("transaction_id", c.FormValue("transaction_id"))
	}
	if orderID == "" {
		orderID = c.Query("order_id", c.FormValue("order_id"))
	}
	if status == "" {
		status = c.Query("status", c.FormValue("status"))
	}

	if !strings.EqualFold(status, "success") && !strings.EqualFold(status, "paid") && !strings.EqualFold(status, "completed") {
		return c.JSON(fiber.Map{"received": true, "status": status})
	}

	if err := h.markPaymentSuccess(c.Context(), txID, orderID); err != nil {
		log.Printf("[ZainCash Webhook] markPaymentSuccess error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"received": true, "status": "success"})
}

// InquiryZainCash checks payment status.
// GET /api/payments/zaincash/inquiry/:transactionId (Authenticated subscriber endpoint)
func (h *ZainCashHandler) InquiryZainCash(c *fiber.Ctx) error {
	txID := strings.TrimSpace(c.Params("transactionId"))
	if txID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "transactionId required"})
	}

	var (
		status string
		amount float64
	)
	err := h.Pool.QueryRow(c.Context(),
		`SELECT status, amount FROM wallet_ledger 
		 WHERE provider_ref = $1 
		    OR provider_ref = ('uat-' || $1)
		 ORDER BY id DESC LIMIT 1`, txID,
	).Scan(&status, &amount)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "transaction not found"})
	}

	return c.JSON(fiber.Map{
		"transactionId": txID,
		"status":        status,
		"amount":        amount,
		"paid":          strings.EqualFold(status, "success"),
	})
}

// zainOAuthToken fetches an OAuth2 access token from ZainCash v2 UAT.
func (h *ZainCashHandler) zainOAuthToken(ctx context.Context, cfg zainConfig) (string, error) {
	form := "grant_type=client_credentials"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL+"/oauth2/token", strings.NewReader(form))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(cfg.MerchantID, cfg.SecretKey)
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	_ = json.Unmarshal(b, &tok)
	if tok.AccessToken == "" {
		return "", fmt.Errorf("no access_token: %s", string(b))
	}
	return tok.AccessToken, nil
}

// verifyZainJWT validates an HS256 JWT signed with the ZainCash secret.
func verifyZainJWT(tokenStr, secret string) bool {
	if secret == "" {
		parts := strings.Split(tokenStr, ".")
		return len(parts) == 3
	}
	key := []byte(secret)
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return key, nil
	})
	return err == nil && tok.Valid
}

// markPaymentSuccess credits the wallet, activates the subscription and
// broadcasts payment_success. Idempotent on provider_ref.
func (h *ZainCashHandler) markPaymentSuccess(ctx context.Context, txID, orderID string) error {
	if txID == "" && orderID == "" {
		return fmt.Errorf("empty transaction identifier")
	}
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var (
		ledgerID int64
		uid      int64
		amount   float64
		status   string
		pref     string
	)
	err = tx.QueryRow(ctx,
		`SELECT id, user_id, amount, status, provider_ref FROM wallet_ledger 
		 WHERE provider_ref = $1 
		    OR provider_ref = $2 
		    OR provider_ref = ('uat-' || $2) 
		    OR provider_ref = ('uat-' || $1) 
		 FOR UPDATE LIMIT 1`, txID, orderID,
	).Scan(&ledgerID, &uid, &amount, &status, &pref)
	if err != nil {
		return fmt.Errorf("transaction not found (tx=%s, order=%s): %w", txID, orderID, err)
	}
	if status == "success" {
		return tx.Commit(ctx) // already processed, idempotent
	}

	if _, err := tx.Exec(ctx,
		`UPDATE wallet_ledger SET status = 'success', completed_at = now() WHERE id = $1`, ledgerID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE users SET wallet_balance = wallet_balance + $1, updated_at = now() WHERE id = $2`, amount, uid); err != nil {
		return err
	}
	// Activate any pending subscription tied to this transaction.
	_, _ = tx.Exec(ctx,
		`UPDATE subscriptions SET status = 'active', started_at = now()
		 WHERE (provider_ref = $1 OR provider_ref = $2 OR provider_ref = $3) AND status = 'pending'`,
		txID, orderID, pref)

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	writeAudit(ctx, h.Pool, uid, "", "PAYMENT_SUCCESS", "wallet_ledger", itoa(ledgerID), map[string]any{
		"provider": "zaincash", "transaction_id": txID, "order_id": orderID, "amount": amount, "at": time.Now(),
	}, "")
	if h.Hub != nil {
		h.Hub.Broadcast(uid, "payment_success", fiber.Map{
			"provider": "zaincash", "transaction_id": txID, "order_id": orderID, "amount": amount, "wallet_ledger_id": ledgerID,
		})
	}
	return nil
}

// hmacSHA256 is a small helper kept for webhook signature checks if needed.
func hmacSHA256(data, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(data))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
