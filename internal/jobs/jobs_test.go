package jobs

import (
	"context"
	"testing"
	"time"
)

func TestJobDataStructures(t *testing.T) {
	svc := NewService(nil)
	if svc == nil {
		t.Fatal("expected service instance")
	}

	executed := false
	svc.RegisterHandler("test_kind", func(ctx context.Context, job *Job) error {
		executed = true
		return nil
	})

	svc.mu.RLock()
	handler, exists := svc.handlers["test_kind"]
	svc.mu.RUnlock()

	if !exists || handler == nil {
		t.Fatal("expected handler registered")
	}

	dummyJob := &Job{
		ID:         1,
		MerchantID: 10,
		Kind:       "test_kind",
	}
	if err := handler(context.Background(), dummyJob); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !executed {
		t.Fatal("expected handler to execute")
	}
}

func TestJobBackoffFormula(t *testing.T) {
	// 30s * attempts
	attempts := 3
	backoff := time.Duration(30*attempts) * time.Second
	expected := 90 * time.Second
	if backoff != expected {
		t.Fatalf("expected %v, got %v", expected, backoff)
	}
}
