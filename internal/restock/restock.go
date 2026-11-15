package restock

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/jobs"
)

type Provider interface {
	Name() string
	Type() string
	PlaceOrder(ctx context.Context, itemID string, quantity int) (string, error)
}

// MockProvider generates fake successful orders for testing & offline emulation.
type MockProvider struct {
	NameStr string
}

func NewMockProvider(name string) *MockProvider {
	if name == "" {
		name = "Asiacell B2B Mock API"
	}
	return &MockProvider{NameStr: name}
}

func (m *MockProvider) Name() string { return m.NameStr }
func (m *MockProvider) Type() string { return "mock" }
func (m *MockProvider) PlaceOrder(ctx context.Context, itemID string, quantity int) (string, error) {
	orderID := fmt.Sprintf("MOCK-ORD-%s-%d-%d", itemID, quantity, rand.Intn(999999))
	return orderID, nil
}

type Service struct {
	pool      *pgxpool.Pool
	jobsSvc   *jobs.Service
	providers map[string]Provider
	mu        sync.RWMutex
}

func NewService(pool *pgxpool.Pool, jobsSvc *jobs.Service) *Service {
	s := &Service{
		pool:      pool,
		jobsSvc:   jobsSvc,
		providers: make(map[string]Provider),
	}
	// Register default mock provider
	s.RegisterProvider(NewMockProvider("default_mock"))
	return s
}

func (s *Service) RegisterProvider(p Provider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers[p.Name()] = p
}

// AutoRestock attempts to order stock from the specified provider, falling back to background jobs on network failure.
func (s *Service) AutoRestock(ctx context.Context, merchantID int64, providerName, itemID string, quantity int) (string, error) {
	s.mu.RLock()
	provider, exists := s.providers[providerName]
	if !exists {
		provider = s.providers["default_mock"]
	}
	s.mu.RUnlock()

	var orderID string
	var orderErr error

	if provider != nil {
		orderID, orderErr = provider.PlaceOrder(ctx, itemID, quantity)
	} else {
		orderErr = fmt.Errorf("no provider available: %s", providerName)
	}

	if orderErr == nil {
		// Succeeded immediately: record order in restock_orders table
		s.recordOrder(ctx, merchantID, itemID, quantity, "completed", orderID)
		return orderID, nil
	}

	// Provider failed: fallback to background jobs for automatic retry with exponential backoff
	if s.jobsSvc != nil {
		payload := map[string]interface{}{
			"merchant_id":   merchantID,
			"provider_name": providerName,
			"item_id":       itemID,
			"quantity":      quantity,
		}
		jobID, jErr := s.jobsSvc.Enqueue(ctx, merchantID, "restock_retry", payload, 5)
		if jErr == nil {
			fallbackID := fmt.Sprintf("JOB-%d", jobID)
			s.recordOrder(ctx, merchantID, itemID, quantity, "pending", fallbackID)
			return fallbackID, nil
		}
	}

	s.recordOrder(ctx, merchantID, itemID, quantity, "failed", "")
	return "", orderErr
}

func (s *Service) recordOrder(ctx context.Context, merchantID int64, itemID string, quantity int, status, extID string) {
	if s.pool == nil {
		return
	}
	query := `
		INSERT INTO restock_orders (merchant_id, provider_id, item_id, quantity, status, external_order_id, created_at, updated_at)
		VALUES ($1, 1, $2, $3, $4, $5, now(), now());
	`
	_, _ = s.pool.Exec(ctx, query, merchantID, itemID, quantity, status, extID)
}

// ListOrders returns recent replenishment orders.
func (s *Service) ListOrders(ctx context.Context, merchantID int64) ([]map[string]interface{}, error) {
	if s.pool == nil {
		return []map[string]interface{}{}, nil
	}
	query := `
		SELECT id, item_id, quantity, status, external_order_id, created_at
		FROM restock_orders
		WHERE merchant_id = $1 OR merchant_id = 0
		ORDER BY id DESC LIMIT 50;
	`
	rows, err := s.pool.Query(ctx, query, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []map[string]interface{}
	for rows.Next() {
		var id int64
		var itemID, status, extID string
		var qty int
		var createdAt time.Time
		if err := rows.Scan(&id, &itemID, &qty, &status, &extID, &createdAt); err == nil {
			list = append(list, map[string]interface{}{
				"id":                id,
				"item_id":           itemID,
				"quantity":          qty,
				"status":            status,
				"external_order_id": extID,
				"created_at":        createdAt,
			})
		}
	}
	return list, nil
}
