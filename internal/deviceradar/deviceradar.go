package deviceradar

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Broadcaster interface {
	Broadcast(userID int64, event string, payload any)
}

type DeviceStatus struct {
	UserID        int64     `json:"user_id"`
	Email         string    `json:"email"`
	StoreName     string    `json:"store_name"`
	DeviceModel   string    `json:"device_model"`
	AppVersion    string    `json:"app_version"`
	LatencyMs     int64     `json:"latency_ms"`
	CurrentScreen string    `json:"current_screen"`
	LastSeen      time.Time `json:"last_seen"`
	IsOnline      bool      `json:"is_online"`
}

type RemoteCommand struct {
	ID        int64          `json:"id"`
	CommandID string         `json:"command_id"`
	UserID    int64          `json:"user_id"`
	Command   string         `json:"command"` // 'force_sync', 'clear_cache', 'pull_diagnostics'
	Payload   map[string]any `json:"payload"`
	Status    string         `json:"status"` // 'queued', 'sent', 'acked', 'failed', 'expired'
	Result    map[string]any `json:"result,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	ExpiresAt time.Time      `json:"expires_at"`
	AckedAt   *time.Time     `json:"acked_at,omitempty"`
}

type Service struct {
	Pool *pgxpool.Pool
	Hub  Broadcaster

	mu      sync.RWMutex
	devices map[int64]*DeviceStatus
}

func NewService(pool *pgxpool.Pool, hub Broadcaster) *Service {
	s := &Service{
		Pool:    pool,
		Hub:     hub,
		devices: make(map[int64]*DeviceStatus),
	}
	// Start presence cleaner
	go s.runPresenceCleaner()
	return s
}

func (s *Service) runPresenceCleaner() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		cutoff := time.Now().Add(-90 * time.Second)
		for uid, dev := range s.devices {
			if dev.LastSeen.Before(cutoff) {
				delete(s.devices, uid)
			}
		}
		s.mu.Unlock()
	}
}

// RecordHeartbeat updates the live device presence.
func (s *Service) RecordHeartbeat(ctx context.Context, userID int64, email, store, model, version string, latency int64, screen string) {
	now := time.Now().UTC()
	s.mu.Lock()
	s.devices[userID] = &DeviceStatus{
		UserID:        userID,
		Email:         email,
		StoreName:     store,
		DeviceModel:   model,
		AppVersion:    version,
		LatencyMs:     latency,
		CurrentScreen: screen,
		LastSeen:      now,
		IsOnline:      true,
	}
	s.mu.Unlock()

	// Update user record in DB
	if s.Pool != nil {
		_, _ = s.Pool.Exec(ctx,
			`UPDATE users
			 SET device_model = $1, app_version = $2, last_heartbeat = $3
			 WHERE id = $4`,
			model, version, now, userID,
		)
	}
}

// RemoveDevice removes user presence on WebSocket disconnect.
func (s *Service) RemoveDevice(userID int64) {
	s.mu.Lock()
	delete(s.devices, userID)
	s.mu.Unlock()
}

// GetOnlineDevices returns all currently connected live devices.
func (s *Service) GetOnlineDevices() []DeviceStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]DeviceStatus, 0, len(s.devices))
	for _, d := range s.devices {
		list = append(list, *d)
	}
	return list
}

// Whitelist of strictly safe commands
var allowedCommands = map[string]bool{
	"force_sync":       true,
	"clear_cache":      true,
	"pull_diagnostics": true,
}

// QueueCommand signs and queues a verified remote command for a subscriber.
func (s *Service) QueueCommand(ctx context.Context, userID int64, command string, payload map[string]any, timeout time.Duration) (*RemoteCommand, error) {
	if !allowedCommands[command] {
		return nil, fmt.Errorf("الأمر غير مسموح به أمنياً. القائمة البيضاء: force_sync, clear_cache, pull_diagnostics")
	}

	cmdUUID := uuid.New()
	now := time.Now().UTC()
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	expiresAt := now.Add(timeout)

	var id int64
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO remote_commands (command_id, user_id, command, payload, status, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, 'queued', $5, $6)
		 RETURNING id`,
		cmdUUID, userID, command, payload, now, expiresAt,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("queue remote command: %w", err)
	}

	cmd := &RemoteCommand{
		ID:        id,
		CommandID: cmdUUID.String(),
		UserID:    userID,
		Command:   command,
		Payload:   payload,
		Status:    "queued",
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}

	// Dispatch immediately if online via WebSocket
	if s.Hub != nil {
		s.Hub.Broadcast(userID, "remote_command", cmd)
	}

	return cmd, nil
}

// AckCommand acknowledges command receipt/execution by the client app.
func (s *Service) AckCommand(ctx context.Context, commandID string, userID int64, status string, result map[string]any) error {
	now := time.Now().UTC()
	if status != "acked" && status != "failed" {
		status = "acked"
	}
	_, err := s.Pool.Exec(ctx,
		`UPDATE remote_commands
		 SET status = $1, result = $2, acked_at = $3
		 WHERE command_id = $4 AND user_id = $5`,
		status, result, now, commandID, userID,
	)
	return err
}

// Automation Rule Engine (Idempotent Event -> Action)
type AutomationRule struct {
	ID                     int64          `json:"id"`
	RuleName               string         `json:"rule_name"`
	TriggerEvent           string         `json:"trigger_event"` // 'topup_volume_spike', 'expiring_in_24h'
	Conditions             map[string]any `json:"conditions"`
	Actions                map[string]any `json:"actions"`
	IsActive               bool           `json:"is_active"`
	MaxExecutionsPerTarget int            `json:"max_executions_per_target"`
}

func (s *Service) ExecuteAutomationIfEligible(ctx context.Context, rule AutomationRule, targetUID int64, idempotencyKey string) (bool, error) {
	if !rule.IsActive {
		return false, nil
	}

	// 1. Check idempotency: ensure not rewarded or executed twice for this target/period
	var alreadyExecuted bool
	err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM automation_executions WHERE idempotency_key = $1)`,
		idempotencyKey,
	).Scan(&alreadyExecuted)
	if err != nil {
		return false, err
	}
	if alreadyExecuted {
		return false, nil
	}

	// 2. Execute action
	actionType, _ := rule.Actions["action_type"].(string)
	switch actionType {
	case "grant_free_days":
		days, _ := rule.Actions["days"].(float64)
		if days > 0 {
			_, _ = s.Pool.Exec(ctx,
				`UPDATE users
				 SET subscription_ends_at = GREATEST(COALESCE(subscription_ends_at, now()), now()) + ($1 || ' days')::interval
				 WHERE id = $2`,
				int(days), targetUID,
			)
		}
	case "send_app_notification":
		msg, _ := rule.Actions["message"].(string)
		if msg != "" && s.Hub != nil {
			s.Hub.Broadcast(targetUID, "announcement", map[string]any{
				"title":   rule.RuleName,
				"message": msg,
			})
		}
	}

	// 3. Record execution
	_, err = s.Pool.Exec(ctx,
		`INSERT INTO automation_executions (rule_id, target_user_id, idempotency_key, executed_action, created_at)
		 VALUES ($1, $2, $3, $4, now())`,
		rule.ID, targetUID, idempotencyKey, rule.Actions,
	)
	return err == nil, err
}
