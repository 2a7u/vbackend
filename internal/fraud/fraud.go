package fraud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Broadcaster interface {
	Broadcast(userID int64, event string, payload any)
}

type Rule struct {
	ID                int64     `json:"id"`
	RuleCode          string    `json:"rule_code"`
	NameAR            string    `json:"name_ar"`
	Description       string    `json:"description"`
	ConditionType     string    `json:"condition_type"` // 'velocity', 'max_single_amount', 'daily_volume'
	ThresholdValue    int64     `json:"threshold_value"`
	TimeWindowSeconds int       `json:"time_window_seconds"`
	Action            string    `json:"action"` // 'alert', 'freeze_wallet', 'require_otp'
	IsActive          bool      `json:"is_active"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type Event struct {
	ID          int64          `json:"id"`
	UserID      int64          `json:"user_id"`
	UserEmail   string         `json:"user_email,omitempty"`
	StoreName   string         `json:"store_name,omitempty"`
	RuleCode    string         `json:"rule_code"`
	RuleNameAR  string         `json:"rule_name_ar,omitempty"`
	Severity    string         `json:"severity"`
	ActionTaken string         `json:"action_taken"`
	Details     map[string]any `json:"details"`
	Status      string         `json:"status"` // 'pending', 'reviewed', 'dismissed'
	TriggeredAt time.Time      `json:"triggered_at"`
	ReviewedAt  *time.Time     `json:"reviewed_at,omitempty"`
	ReviewedBy  *int64         `json:"reviewed_by,omitempty"`
}

type txTimestamp struct {
	Time   time.Time
	Amount int64
}

type Engine struct {
	Pool       *pgxpool.Pool
	Hub        Broadcaster
	WebhookURL string

	mu      sync.RWMutex
	history map[int64][]txTimestamp // user_id -> sliding window history
	rules   map[string]Rule
}

func NewEngine(pool *pgxpool.Pool, hub Broadcaster) *Engine {
	webhook := os.Getenv("FRAUD_WEBHOOK_URL")
	e := &Engine{
		Pool:       pool,
		Hub:        hub,
		WebhookURL: webhook,
		history:    make(map[int64][]txTimestamp),
		rules:      make(map[string]Rule),
	}
	if pool != nil {
		_ = e.RefreshRules(context.Background())
	}
	return e
}

// SetRule adds or overrides a rule in memory (useful for unit testing).
func (e *Engine) SetRule(r Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules[r.RuleCode] = r
}

// RefreshRules reloads active fraud rules from the database.
func (e *Engine) RefreshRules(ctx context.Context) error {
	if e.Pool == nil {
		return nil
	}
	rows, err := e.Pool.Query(ctx,
		`SELECT id, rule_code, name_ar, description, condition_type, threshold_value,
		        time_window_seconds, action, is_active, created_at, updated_at
		 FROM fraud_rules`,
	)
	if err != nil {
		return fmt.Errorf("load fraud rules: %w", err)
	}
	defer rows.Close()

	newRules := make(map[string]Rule)
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.ID, &r.RuleCode, &r.NameAR, &r.Description, &r.ConditionType,
			&r.ThresholdValue, &r.TimeWindowSeconds, &r.Action, &r.IsActive, &r.CreatedAt, &r.UpdatedAt); err == nil {
			newRules[r.RuleCode] = r
		}
	}

	e.mu.Lock()
	e.rules = newRules
	e.mu.Unlock()
	return nil
}

type CheckRequest struct {
	UserID    int64
	AmountIQD int64
	TxType    string // 'topup', 'voucher_dispense', 'sale', 'adjustment'
	IPAddress string
}

type EvaluationResult struct {
	Allowed     bool
	ActionTaken string
	TriggeredBy *Rule
	Reason      string
}

// Evaluate evaluates a transaction against active fraud rules.
// If a rule dictates 'freeze_wallet', the user's wallet is locked in the database immediately.
func (e *Engine) Evaluate(ctx context.Context, tx pgx.Tx, req CheckRequest) (*EvaluationResult, error) {
	now := time.Now().UTC()

	e.mu.Lock()
	// Record in sliding window
	e.history[req.UserID] = append(e.history[req.UserID], txTimestamp{Time: now, Amount: req.AmountIQD})
	// Clean items older than 24h
	cutoff24h := now.Add(-24 * time.Hour)
	cleaned := make([]txTimestamp, 0, len(e.history[req.UserID]))
	for _, item := range e.history[req.UserID] {
		if item.Time.After(cutoff24h) {
			cleaned = append(cleaned, item)
		}
	}
	e.history[req.UserID] = cleaned
	userHistory := append([]txTimestamp(nil), cleaned...)
	rulesSnapshot := make([]Rule, 0, len(e.rules))
	for _, r := range e.rules {
		if r.IsActive {
			rulesSnapshot = append(rulesSnapshot, r)
		}
	}
	e.mu.Unlock()

	for _, rule := range rulesSnapshot {
		switch rule.ConditionType {
		case "max_single_amount":
			if req.AmountIQD > rule.ThresholdValue && rule.ThresholdValue > 0 {
				return e.handleViolation(ctx, tx, req, rule, fmt.Sprintf("قيمة العملية %d تتجاوز الحد المسموح %d دينار", req.AmountIQD, rule.ThresholdValue))
			}

		case "velocity":
			windowStart := now.Add(-time.Duration(rule.TimeWindowSeconds) * time.Second)
			var count int64
			for _, item := range userHistory {
				if item.Time.After(windowStart) {
					count++
				}
			}
			if count > rule.ThresholdValue && rule.ThresholdValue > 0 {
				return e.handleViolation(ctx, tx, req, rule, fmt.Sprintf("تكرار العمليات (%d خلال %d ثانية) يتجاوز الحد (%d)", count, rule.TimeWindowSeconds, rule.ThresholdValue))
			}

		case "daily_volume":
			windowStart := now.Add(-24 * time.Hour)
			var dailySum int64
			for _, item := range userHistory {
				if item.Time.After(windowStart) {
					dailySum += item.Amount
				}
			}
			if dailySum > rule.ThresholdValue && rule.ThresholdValue > 0 {
				return e.handleViolation(ctx, tx, req, rule, fmt.Sprintf("حجم التداول اليومي %d دينار يتجاوز السقف %d دينار", dailySum, rule.ThresholdValue))
			}
		}
	}

	return &EvaluationResult{Allowed: true}, nil
}

func (e *Engine) handleViolation(ctx context.Context, tx pgx.Tx, req CheckRequest, rule Rule, reason string) (*EvaluationResult, error) {
	details := map[string]any{
		"amount":     req.AmountIQD,
		"tx_type":    req.TxType,
		"ip":         req.IPAddress,
		"reason":     reason,
		"rule_name":  rule.NameAR,
		"threshold":  rule.ThresholdValue,
		"window_sec": rule.TimeWindowSeconds,
	}
	detailsJSON, _ := json.Marshal(details)

	severity := "high"
	if rule.Action == "freeze_wallet" {
		severity = "critical"
	}

	// Record event
	var eventID int64
	execer := tx.Exec
	if tx == nil {
		err := e.Pool.QueryRow(ctx,
			`INSERT INTO fraud_events (user_id, rule_code, severity, action_taken, details)
			 VALUES ($1, $2, $3, $4, $5)
			 RETURNING id`,
			req.UserID, rule.RuleCode, severity, rule.Action, detailsJSON,
		).Scan(&eventID)
		if err != nil {
			return nil, fmt.Errorf("record fraud event: %w", err)
		}
	} else {
		err := tx.QueryRow(ctx,
			`INSERT INTO fraud_events (user_id, rule_code, severity, action_taken, details)
			 VALUES ($1, $2, $3, $4, $5)
			 RETURNING id`,
			req.UserID, rule.RuleCode, severity, rule.Action, detailsJSON,
		).Scan(&eventID)
		if err != nil {
			return nil, fmt.Errorf("record fraud event: %w", err)
		}
	}

	if rule.Action == "freeze_wallet" {
		if tx != nil {
			_, _ = tx.Exec(ctx, `UPDATE users SET wallet_status = 'frozen', updated_at = now() WHERE id = $1`, req.UserID)
		} else {
			_, _ = e.Pool.Exec(ctx, `UPDATE users SET wallet_status = 'frozen', updated_at = now() WHERE id = $1`, req.UserID)
		}

		if e.Hub != nil {
			e.Hub.Broadcast(req.UserID, "wallet_frozen", fiberMap("سبب التجميد", reason))
		}
	}

	alertPayload := map[string]any{
		"event_id":     eventID,
		"user_id":      req.UserID,
		"rule_code":    rule.RuleCode,
		"rule_name_ar": rule.NameAR,
		"severity":     severity,
		"action_taken": rule.Action,
		"reason":       reason,
		"amount":       req.AmountIQD,
		"time":         time.Now().UTC(),
	}

	if e.Hub != nil {
		e.Hub.Broadcast(0, "fraud_alert", alertPayload)
	}

	// Dispatch to external webhook if configured (Requirement 1d)
	if e.WebhookURL != "" {
		go func(webhook string, payload map[string]any) {
			b, err := json.Marshal(payload)
			if err != nil {
				return
			}
			client := &http.Client{Timeout: 5 * time.Second}
			httpReq, err := http.NewRequest("POST", webhook, bytes.NewBuffer(b))
			if err == nil {
				httpReq.Header.Set("Content-Type", "application/json")
				httpReq.Header.Set("User-Agent", "VENUS-POS-Fintech-FraudEngine/1.0")
				resp, err := client.Do(httpReq)
				if err == nil && resp != nil && resp.Body != nil {
					_ = resp.Body.Close()
				}
			}
		}(e.WebhookURL, alertPayload)
	}

	_ = execer
	return &EvaluationResult{
		Allowed:     rule.Action != "freeze_wallet",
		ActionTaken: rule.Action,
		TriggeredBy: &rule,
		Reason:      reason,
	}, nil
}

func fiberMap(k, v string) map[string]string {
	return map[string]string{k: v}
}

// UnfreezeWallet unlocks a user's wallet and sets wallet_status = 'active'.
func (e *Engine) UnfreezeWallet(ctx context.Context, userID int64, adminID int64, reason string) error {
	_, err := e.Pool.Exec(ctx,
		`UPDATE users
		 SET wallet_status = 'active', updated_at = now()
		 WHERE id = $1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("unfreeze wallet: %w", err)
	}

	// Update any open fraud events for this user to reviewed
	_, _ = e.Pool.Exec(ctx,
		`UPDATE fraud_events
		 SET status = 'reviewed', reviewed_at = now(), reviewed_by = $1
		 WHERE user_id = $2 AND status = 'pending'`,
		adminID, userID,
	)

	if e.Hub != nil {
		e.Hub.Broadcast(userID, "wallet_unfrozen", map[string]any{
			"user_id": userID,
			"reason":  reason,
			"time":    time.Now().UTC(),
		})
	}
	return nil
}
