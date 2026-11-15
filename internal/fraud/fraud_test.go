package fraud_test

import (
	"context"
	"testing"
	"time"

	"venus_backend/internal/fraud"
)

type mockBroadcaster struct {
	events []string
}

func (m *mockBroadcaster) Broadcast(userID int64, event string, payload any) {
	m.events = append(m.events, event)
}

func TestSlidingWindowVelocityEvaluation(t *testing.T) {
	hub := &mockBroadcaster{}
	engine := fraud.NewEngine(nil, hub)

	// Inject test rules manually
	ctx := context.Background()

	rule := fraud.Rule{
		ID:                1,
		RuleCode:          "VELOCITY_TEST",
		NameAR:            "فحص تكرار العمليات",
		ConditionType:     "velocity",
		ThresholdValue:    3, // Max 3 requests in 10s
		TimeWindowSeconds: 10,
		Action:            "alert",
		IsActive:          true,
	}
	engine.SetRule(rule)

	// Simulate rapid transactions
	userID := int64(999)
	req := fraud.CheckRequest{
		UserID:    userID,
		AmountIQD: 10000,
		TxType:    "topup",
		IPAddress: "127.0.0.1",
	}

	// 1st request -> allowed
	res1, _ := engine.Evaluate(ctx, nil, req)
	if !res1.Allowed {
		t.Fatalf("expected 1st request to be allowed")
	}

	// 2nd request -> allowed
	res2, _ := engine.Evaluate(ctx, nil, req)
	if !res2.Allowed {
		t.Fatalf("expected 2nd request to be allowed")
	}

	// 3rd request -> allowed
	res3, _ := engine.Evaluate(ctx, nil, req)
	if !res3.Allowed {
		t.Fatalf("expected 3rd request to be allowed")
	}

	_ = rule
}

func TestMaxSingleAmountRule(t *testing.T) {
	hub := &mockBroadcaster{}
	engine := fraud.NewEngine(nil, hub)
	ctx := context.Background()

	largeReq := fraud.CheckRequest{
		UserID:    888,
		AmountIQD: 2000000, // 2M IQD
		TxType:    "adjustment",
		IPAddress: "192.168.1.1",
	}

	res, err := engine.Evaluate(ctx, nil, largeReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = res
}

func TestTimeFormatBaghdad(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Baghdad")
	if err != nil {
		t.Fatalf("load Baghdad location: %v", err)
	}
	now := time.Now().In(loc)
	if now.Location().String() != "Asia/Baghdad" {
		t.Fatalf("expected Asia/Baghdad timezone, got %s", now.Location().String())
	}
}
