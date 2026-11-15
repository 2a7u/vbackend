package limits

import (
	"testing"
	"time"
)

func TestTokenBucketRateLimiting(t *testing.T) {
	svc := NewService(nil)

	// Bucket of capacity 3, refill rate 1 token / sec
	key := "test-client"

	// First 3 requests should succeed
	for i := 0; i < 3; i++ {
		allowed, retryAfter := svc.Allow(key, 3, 1)
		if !allowed {
			t.Fatalf("expected request %d to be allowed", i+1)
		}
		if retryAfter != 0 {
			t.Fatalf("expected retryAfter=0 on allowed, got %d", retryAfter)
		}
	}

	// 4th request must be rate limited
	allowed, retryAfter := svc.Allow(key, 3, 1)
	if allowed {
		t.Fatalf("expected 4th request to be denied by rate limiter")
	}
	if retryAfter < 1 {
		t.Fatalf("expected retryAfter >= 1, got %d", retryAfter)
	}

	// Sleep for refill
	time.Sleep(1100 * time.Millisecond)

	// Should allow 1 request now
	allowed2, _ := svc.Allow(key, 3, 1)
	if !allowed2 {
		t.Fatalf("expected request after refill to be allowed")
	}
}
