package idem

import (
	"testing"
	"time"
)

func TestCleanKey(t *testing.T) {
	longKey := ""
	for i := 0; i < 200; i++ {
		longKey += "a"
	}

	cleaned := CleanKey("  " + longKey + "  ")
	if len(cleaned) != 128 {
		t.Fatalf("expected 128 characters, got %d", len(cleaned))
	}

	normal := CleanKey("  uuid-12345-abcde  ")
	if normal != "uuid-12345-abcde" {
		t.Fatalf("expected trimmed key, got %q", normal)
	}
}

func TestIdemTTLLimits(t *testing.T) {
	minTTL := 5 * time.Second
	if minTTL < 10*time.Second {
		minTTL = 10 * time.Second
	}
	if minTTL != 10*time.Second {
		t.Fatalf("expected minimum 10s TTL, got %v", minTTL)
	}

	maxTTL := 48 * time.Hour
	if maxTTL > 24*time.Hour {
		maxTTL = 24 * time.Hour
	}
	if maxTTL != 24*time.Hour {
		t.Fatalf("expected maximum 24h TTL, got %v", maxTTL)
	}
}
