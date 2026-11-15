package api

import (
	"context"
	"strings"
	"testing"
)

func TestHashKeyAndGeneration(t *testing.T) {
	key := "vsk_live_1122334455667788"
	h1 := HashKey(key)
	h2 := HashKey(key)
	if h1 != h2 {
		t.Fatal("expected deterministic hash")
	}

	svc := NewService(nil)
	gen, err := svc.GenerateKey(context.Background(), 1, "ERP Partner", []string{"read", "sell"}, 120)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.HasPrefix(gen.SecretKey, "vsk_live_") {
		t.Fatalf("expected prefix vsk_live_, got %s", gen.SecretKey)
	}

	spec := svc.GetOpenAPISpec()
	if spec["openapi"] != "3.0.3" {
		t.Fatalf("expected openapi 3.0.3, got %v", spec["openapi"])
	}
}
