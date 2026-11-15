package restock

import (
	"context"
	"strings"
	"testing"
)

func TestMockProviderRestock(t *testing.T) {
	svc := NewService(nil, nil)

	orderID, err := svc.AutoRestock(context.Background(), 10, "default_mock", "asia_5000", 50)
	if err != nil {
		t.Fatalf("unexpected restock error: %v", err)
	}
	if !strings.HasPrefix(orderID, "MOCK-ORD-asia_5000-50") {
		t.Fatalf("unexpected order ID format: %s", orderID)
	}
}
