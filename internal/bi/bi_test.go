package bi_test

import (
	"context"
	"testing"
	"venus_backend/internal/bi"
)

func TestServerHealthDiagnostics(t *testing.T) {
	svc := bi.NewService(nil)
	health := svc.GetServerHealth(context.Background())

	if health.GoroutinesCount <= 0 {
		t.Fatalf("expected positive goroutines count, got %d", health.GoroutinesCount)
	}
	if health.AllocatedRAMMB <= 0 {
		t.Fatalf("expected positive allocated RAM, got %f", health.AllocatedRAMMB)
	}
	if health.ServerTimeBaghdad == "" {
		t.Fatalf("expected formatted Baghdad time, got empty string")
	}
}
