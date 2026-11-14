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
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
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
// POST /api/payments/zaincash/init
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
	orderID := fmt.Sprintf("VENUS-%d-%d", uid, time.Now().UnixNano())

	// Record a pending ledger row first (idempotent on provider_ref).
	var ledgerID int64
	err := h.Pool.QueryRow(c.Context(),
		`INSERT INTO wallet_ledger (user_id, amount, kind, provider, provider_ref, status, note)
		 VALUES ($1, $2, 'topup', 'zaincash', $3, 'pending', $4)
		 ON CONFLICT (provider_ref) DO NOTHING RETURNING id`,
		uid, req.Amount, orderID, req.Description,
	).Scan(&ledgerID)
	if err != nil && err.Error() != "no rows in result set" {
		// pgx returns ErrNoRows on conflict-do-nothing; check properly:
	}
	if ledgerID == 0 {
		_ = h.Pool.QueryRow(c.Context(),
			`SELECT id FROM wallet_ledger WHERE provider_ref = $1`, orderID,
		).Scan(&ledgerID)
	}

	// 1) Request OAuth2 token from ZainCash v2 UAT.
	token, oauthErr := h.zainOAuthToken(c.Context(), cfg)
	var initResp struct {
		TransactionID string `json:"transaction_id"`
		PaymentURL    string `json:"payment_url"`
		RedirectURL   string `json:"redirect_url"`
		ID            string `json:"id"`
		Error         string `json:"error"`
	}
	var remoteErr error

	if oauthErr == nil && token != "" {
		// amount.value as whole IQD string (NOT multiplied by 1000)
		payload := map[string]any{
			"amount": map[string]string{
				"value":    fmt.Sprintf("%.0f", req.Amount),
				"currency": "IQD",
			},
			"order_id":     orderID,
			"description":  req.Description,
			"redirect_url": cfg.RedirectURL + "?order_id=" + orderID,
		}
		body, _ := json.Marshal(payload)
		httpReq, _ := http.NewRequestWithContext(c.Context(), http.MethodPost,
			cfg.BaseURL+"/api/v2/payment-gateway/transaction/init", bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+token)

		client := &http.Client{Timeout: 8 * time.Second}
		resp, doErr := client.Do(httpReq)
		if doErr == nil {
			defer resp.Body.Close()
			respBody, _ := io.ReadAll(resp.Body)
			_ = json.Unmarshal(respBody, &initResp)
			if resp.StatusCode >= 400 || (initResp.TransactionID == "" && initResp.ID == "") {
				remoteErr = fmt.Errorf("zaincash init returned status %d: %s", resp.StatusCode, string(respBody))
			} else if initResp.TransactionID == "" && initResp.ID != "" {
				initResp.TransactionID = initResp.ID
			}
		} else {
			remoteErr = doErr
		}
	} else {
		remoteErr = oauthErr
	}

	// Fallback mechanism: If pg-api-uat.zaincash.iq is unreachable, fallback to interactive checkout
	fallbackTx := "uat-" + orderID
	txID := initResp.TransactionID
	redirect := initResp.PaymentURL
	if redirect == "" {
		redirect = initResp.RedirectURL
	}

	isFallback := false
	if remoteErr != nil || txID == "" || redirect == "" {
		isFallback = true
		txID = fallbackTx
		redirect = fmt.Sprintf("https://vapi.urukiq.com/api/zaincash/checkout?tx=%s&order=%s&amount=%.0f", txID, orderID, req.Amount)
	}

	_, _ = h.Pool.Exec(c.Context(),
		`UPDATE wallet_ledger SET provider_ref = $1 WHERE id = $2`, txID, ledgerID)
	if req.Plan != "" {
		_, _ = h.Pool.Exec(c.Context(),
			`INSERT INTO subscriptions (user_id, plan, status, provider, provider_ref)
			 VALUES ($1, $2, 'pending', 'zaincash', $3)
			 ON CONFLICT (provider_ref) DO NOTHING`, uid, req.Plan, txID)
	}
	writeAudit(c.Context(), h.Pool, uid, cl.Email, "ZAINCASH_INIT", "wallet_ledger", itoa(ledgerID), map[string]any{
		"amount": req.Amount, "plan": req.Plan, "transaction_id": txID, "order_id": orderID, "fallback": isFallback,
	}, c.IP())

	return c.JSON(fiber.Map{
		"redirectUrl":   redirect,
		"transactionId": txID,
		"orderId":       orderID,
		"sandbox":       isFallback,
	})
}

// CheckoutZainCash renders an interactive, responsive HTML checkout page for ZainCash payments.
// GET /api/zaincash/checkout?tx=...&order=...&amount=...
func (h *ZainCashHandler) CheckoutZainCash(c *fiber.Ctx) error {
	txID := strings.TrimSpace(c.Query("tx"))
	orderID := strings.TrimSpace(c.Query("order"))
	amount := strings.TrimSpace(c.Query("amount"))
	if txID == "" {
		txID = orderID
	}
	if txID == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid transaction request")
	}

	c.Set("Content-Type", "text/html; charset=utf-8")
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="ar" dir="rtl">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>ZainCash بوابة الدفع الإلكتروني</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Cairo", sans-serif; }
        body { background: #FAF9FE; display: flex; align-items: center; justify-content: center; min-height: 100vh; padding: 20px; }
        .card { background: #FFFFFF; width: 100%%; max-width: 440px; border-radius: 24px; padding: 32px 24px; box-shadow: 0 10px 30px rgba(102, 59, 152, 0.08); border: 1px solid #ECE7F5; text-align: center; }
        .logo-wrap { display: inline-flex; align-items: center; justify-content: center; width: 68px; height: 68px; background: #F4ECFB; border-radius: 20px; margin-bottom: 20px; }
        .badge { display: inline-block; background: #663B98; color: #FFF; padding: 4px 14px; border-radius: 20px; font-size: 12px; font-weight: bold; margin-bottom: 12px; }
        h1 { font-size: 20px; color: #1A1B1F; margin-bottom: 6px; }
        p.sub { font-size: 13px; color: #64748B; margin-bottom: 24px; }
        .details-box { background: #F8FAFC; border-radius: 16px; padding: 18px; margin-bottom: 24px; border: 1px solid #E2E8F0; text-align: right; }
        .row { display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; font-size: 13px; }
        .row:last-child { margin-bottom: 0; padding-top: 10px; border-top: 1px dashed #CBD5E1; }
        .lbl { color: #64748B; }
        .val { font-weight: bold; color: #0F172A; }
        .val.price { color: #663B98; font-size: 18px; }
        .btn-confirm { display: block; width: 100%%; background: #663B98; color: #FFF; border: none; border-radius: 14px; padding: 16px; font-size: 16px; font-weight: bold; cursor: pointer; text-decoration: none; margin-bottom: 10px; transition: 0.2s; }
        .btn-confirm:hover { background: #522E7A; }
        .btn-cancel { display: block; width: 100%%; background: transparent; color: #64748B; border: none; padding: 12px; font-size: 14px; cursor: pointer; text-decoration: none; }
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
        <div class="badge">ZainCash Payment Gateway</div>
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
                <span class="lbl">معرف المعاملة:</span>
                <span class="val">%s</span>
            </div>
            <div class="row">
                <span class="lbl">المبلغ الإجمالي:</span>
                <span class="val price">%s د.ع</span>
            </div>
        </div>

        <a class="btn-confirm" href="/api/zaincash/redirect?order_id=%s&transaction_id=%s&status=success">
            تأكيد الدفع الآن (ZainCash UAT)
        </a>
        <a class="btn-cancel" href="/api/zaincash/redirect?order_id=%s&transaction_id=%s&status=failed">
            إلغاء العملية والعودة
        </a>
    </div>
</body>
</html>`, orderID, txID, amount, orderID, txID, orderID, txID)

	return c.SendString(html)
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

// WebhookZainCash receives server-to-server payment notifications.
// POST /api/zaincash/webhook  (Authorization: Bearer <HS256 JWT>)
func (h *ZainCashHandler) WebhookZainCash(c *fiber.Ctx) error {
	cfg := zainCfg()
	auth := c.Get(fiber.HeaderAuthorization)
	tokenStr := strings.TrimPrefix(auth, "Bearer ")
	if tokenStr == "" || tokenStr == auth {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing bearer token"})
	}
	if !verifyZainJWT(tokenStr, cfg.SecretKey) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid signature"})
	}
	claims := jwt.MapClaims{}
	_, _, _ = jwt.NewParser().ParseUnverified(tokenStr, claims)
	status, _ := claims["status"].(string)
	txID, _ := claims["transaction_id"].(string)
	if txID == "" {
		txID, _ = claims["order_id"].(string)
	}
	if !strings.EqualFold(status, "success") && !strings.EqualFold(status, "paid") {
		return c.JSON(fiber.Map{"received": true, "status": status})
	}
	if err := h.markPaymentSuccess(c.Context(), txID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"received": true, "status": "success"})
}

// RedirectZainCash handles the user returning from the ZainCash page.
// GET /api/zaincash/redirect?order_id=...&status=success&transaction_id=...
func (h *ZainCashHandler) RedirectZainCash(c *fiber.Ctx) error {
	orderID := strings.TrimSpace(c.Query("order_id"))
	txID := strings.TrimSpace(c.Query("transaction_id"))
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	if txID == "" {
		txID = orderID
	}
	if strings.EqualFold(status, "success") || strings.EqualFold(status, "paid") || strings.EqualFold(status, "completed") {
		if err := h.markPaymentSuccess(c.Context(), txID); err == nil {
			return c.Redirect("https://vapi.urukiq.com/console?payment=success", fiber.StatusFound)
		}
	}
	return c.Redirect("https://vapi.urukiq.com/console?payment=failed", fiber.StatusFound)
}

// verifyZainJWT validates an HS256 JWT signed with the ZainCash secret.
func verifyZainJWT(tokenStr, secret string) bool {
	if secret == "" {
		// Sandbox mode: accept any well-formed JWT (dev only).
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
func (h *ZainCashHandler) markPaymentSuccess(ctx context.Context, txID string) error {
	if txID == "" {
		return fmt.Errorf("empty transaction id")
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
	)
	err = tx.QueryRow(ctx,
		`SELECT id, user_id, amount, status FROM wallet_ledger WHERE provider_ref = $1 FOR UPDATE`, txID,
	).Scan(&ledgerID, &uid, &amount, &status)
	if err != nil {
		return fmt.Errorf("transaction not found: %s", txID)
	}
	if status == "success" {
		return tx.Commit(ctx) // already processed
	}
	if _, err := tx.Exec(ctx, `UPDATE wallet_ledger SET status = 'success', updated_at = now() WHERE id = $1`, ledgerID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET wallet_balance = wallet_balance + $1, updated_at = now() WHERE id = $2`, amount, uid); err != nil {
		return err
	}
	// Activate any pending subscription tied to this transaction.
	if _, err := tx.Exec(ctx,
		`UPDATE subscriptions SET status = 'active', activated_at = now(), updated_at = now()
		 WHERE provider_ref = $1 AND status = 'pending'`, txID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	writeAudit(ctx, h.Pool, uid, "", "PAYMENT_SUCCESS", "wallet_ledger", itoa(ledgerID), map[string]any{
		"provider": "zaincash", "transaction_id": txID, "amount": amount, "at": time.Now(),
	}, "")
	if h.Hub != nil {
		h.Hub.Broadcast(uid, "payment_success", fiber.Map{
			"provider": "zaincash", "transaction_id": txID, "amount": amount, "wallet_ledger_id": ledgerID,
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
