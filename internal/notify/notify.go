package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNoActiveChannels = errors.New("no active notification channels configured")
	ErrAllChannelsFailed = errors.New("all notification channels failed delivery")
)

type EventType string

const (
	EventTopup           EventType = "topup"
	EventPlanSub         EventType = "plan_subscription"
	EventStockLow        EventType = "stock_low"
	EventSecurityFreeze  EventType = "security_freeze"
	EventDailySummary    EventType = "daily_summary"
)

type ChannelConfig struct {
	ID          int64           `json:"id"`
	MerchantID  int64           `json:"merchant_id"`
	ChannelType string          `json:"channel_type"` // telegram, whatsapp, sms, push
	Config      json.RawMessage `json:"config"`
	Priority    int             `json:"priority"`
	IsActive    bool            `json:"is_active"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type DeliveryLog struct {
	ID          int64     `json:"id"`
	MerchantID  int64     `json:"merchant_id"`
	EventType   string    `json:"event_type"`
	ChannelUsed string    `json:"channel_used"`
	Recipient   string    `json:"recipient"`
	Status      string    `json:"status"` // sent, failed, fallback
	Error       string    `json:"error"`
	CreatedAt   time.Time `json:"created_at"`
}

// Channel defines the delivery adapter interface for each provider.
type Channel interface {
	Type() string
	Send(ctx context.Context, recipient, title, message string, extra map[string]interface{}) error
}

// TelegramChannel implements Channel for Telegram Bot API.
type TelegramChannel struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
	Client   *http.Client
}

func (t *TelegramChannel) Type() string { return "telegram" }
func (t *TelegramChannel) Send(ctx context.Context, recipient, title, message string, extra map[string]interface{}) error {
	chatID := t.ChatID
	if recipient != "" {
		chatID = recipient
	}
	if t.BotToken == "" || chatID == "" {
		return errors.New("missing telegram bot_token or chat_id")
	}

	text := fmt.Sprintf("<b>%s</b>\n\n%s", title, message)
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.BotToken)
	body, _ := json.Marshal(map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	})

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram api error: status %d", resp.StatusCode)
	}
	return nil
}

// WhatsAppChannel implements Channel for WhatsApp HTTP Gateways.
type WhatsAppChannel struct {
	EndpointURL string `json:"endpoint_url"`
	APIKey      string `json:"api_key"`
	Client      *http.Client
}

func (w *WhatsAppChannel) Type() string { return "whatsapp" }
func (w *WhatsAppChannel) Send(ctx context.Context, recipient, title, message string, extra map[string]interface{}) error {
	if w.EndpointURL == "" {
		return errors.New("missing whatsapp endpoint URL")
	}
	body, _ := json.Marshal(map[string]interface{}{
		"to":      recipient,
		"title":   title,
		"message": message,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", w.EndpointURL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if w.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+w.APIKey)
	}

	resp, err := w.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("whatsapp gateway error: status %d", resp.StatusCode)
	}
	return nil
}

// SMSChannel implements Channel for SMS providers.
type SMSChannel struct {
	ProviderURL string `json:"provider_url"`
	SenderID    string `json:"sender_id"`
	APIKey      string `json:"api_key"`
	Client      *http.Client
}

func (s *SMSChannel) Type() string { return "sms" }
func (s *SMSChannel) Send(ctx context.Context, recipient, title, message string, extra map[string]interface{}) error {
	if s.ProviderURL == "" {
		return errors.New("missing sms provider URL")
	}
	body, _ := json.Marshal(map[string]interface{}{
		"sender":  s.SenderID,
		"to":      recipient,
		"message": fmt.Sprintf("[%s] %s", title, message),
	})
	req, err := http.NewRequestWithContext(ctx, "POST", s.ProviderURL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.APIKey)
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sms provider error: status %d", resp.StatusCode)
	}
	return nil
}

// PushChannel implements Channel for Push notifications & WebSocket.
type PushChannel struct {
	Broadcaster func(userID int64, event string, payload any)
}

func (p *PushChannel) Type() string { return "push" }
func (p *PushChannel) Send(ctx context.Context, recipient, title, message string, extra map[string]interface{}) error {
	if p.Broadcaster != nil {
		var uid int64
		if recipient != "" {
			fmt.Sscanf(recipient, "%d", &uid)
		}
		p.Broadcaster(uid, "push_notification", map[string]interface{}{
			"title":   title,
			"message": message,
			"extra":   extra,
		})
	}
	return nil
}

type Service struct {
	pool       *pgxpool.Pool
	httpClient *http.Client
	channels   []Channel
	mu         sync.RWMutex
}

func NewService(pool *pgxpool.Pool, broadcaster func(userID int64, event string, payload any)) *Service {
	s := &Service{
		pool: pool,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	// Default in-process push channel
	s.channels = []Channel{
		&PushChannel{Broadcaster: broadcaster},
	}
	return s
}

// RegisterChannel registers or prepends an active channel.
func (s *Service) RegisterChannel(ch Channel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channels = append(s.channels, ch)
}

// Dispatch executes the notification across channels using priority fallback.
func (s *Service) Dispatch(ctx context.Context, merchantID int64, event EventType, recipient, title, message string, extra map[string]interface{}) error {
	s.mu.RLock()
	activeChannels := make([]Channel, len(s.channels))
	copy(activeChannels, s.channels)
	s.mu.RUnlock()

	if len(activeChannels) == 0 {
		return ErrNoActiveChannels
	}

	var lastErr error
	for i, ch := range activeChannels {
		err := ch.Send(ctx, recipient, title, message, extra)
		if err == nil {
			// Delivered successfully
			s.recordLog(ctx, merchantID, string(event), ch.Type(), recipient, "sent", "")
			return nil
		}

		lastErr = err
		// If more channels available, mark as fallback and continue
		if i < len(activeChannels)-1 {
			s.recordLog(ctx, merchantID, string(event), ch.Type(), recipient, "fallback", err.Error())
		} else {
			s.recordLog(ctx, merchantID, string(event), ch.Type(), recipient, "failed", err.Error())
		}
	}

	return fmt.Errorf("%w: %v", ErrAllChannelsFailed, lastErr)
}

func (s *Service) recordLog(ctx context.Context, merchantID int64, event, channel, recipient, status, errMsg string) {
	if s.pool == nil {
		return
	}
	query := `
		INSERT INTO notify_logs (merchant_id, event_type, channel_used, recipient, status, error, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, now());
	`
	_, _ = s.pool.Exec(ctx, query, merchantID, event, channel, recipient, status, errMsg)
}

// ListChannels returns configured channels.
func (s *Service) ListChannels(ctx context.Context, merchantID int64) ([]ChannelConfig, error) {
	if s.pool == nil {
		return []ChannelConfig{}, nil
	}
	query := `
		SELECT id, merchant_id, channel_type, config, priority, is_active, created_at, updated_at
		FROM notify_channels
		WHERE merchant_id = $1 OR merchant_id = 0
		ORDER BY priority ASC, id ASC;
	`
	rows, err := s.pool.Query(ctx, query, merchantID)
	if err != nil {
		return nil, fmt.Errorf("list notify channels: %w", err)
	}
	defer rows.Close()

	var list []ChannelConfig
	for rows.Next() {
		var c ChannelConfig
		if err := rows.Scan(&c.ID, &c.MerchantID, &c.ChannelType, &c.Config, &c.Priority, &c.IsActive, &c.CreatedAt, &c.UpdatedAt); err == nil {
			list = append(list, c)
		}
	}
	return list, nil
}

// SaveChannel saves or updates a channel config.
func (s *Service) SaveChannel(ctx context.Context, merchantID int64, channelType string, configMap map[string]interface{}, priority int, isActive bool) error {
	if s.pool == nil {
		return nil
	}
	cfgBytes, _ := json.Marshal(configMap)
	query := `
		INSERT INTO notify_channels (merchant_id, channel_type, config, priority, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, now(), now());
	`
	_, err := s.pool.Exec(ctx, query, merchantID, channelType, cfgBytes, priority, isActive)
	return err
}

// DeleteChannel deletes a notification channel.
func (s *Service) DeleteChannel(ctx context.Context, id int64) error {
	if s.pool == nil {
		return nil
	}
	_, err := s.pool.Exec(ctx, "DELETE FROM notify_channels WHERE id = $1;", id)
	return err
}
