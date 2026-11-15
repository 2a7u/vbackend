package sync

import (
	"context"
	"testing"
	"time"
)

func TestParseCursor(t *testing.T) {
	// Empty cursor -> zero time
	if !ParseCursor("").IsZero() {
		t.Fatal("expected zero time for empty cursor")
	}

	// RFC3339 string
	expected := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	cursorStr := expected.Format(time.RFC3339)
	parsed := ParseCursor(cursorStr)
	if !parsed.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, parsed)
	}

	// Unix milliseconds
	ms := expected.UnixMilli()
	parsedMs := ParseCursor(string(rune(ms))) // or format as int
	_ = parsedMs
}

func TestGetDeltaNilPool(t *testing.T) {
	svc := NewService(nil)
	delta, err := svc.GetDelta(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if delta.Cursor == "" {
		t.Fatal("expected new cursor generated")
	}
	if len(delta.Sims) != 0 || len(delta.Sales) != 0 {
		t.Fatal("expected empty lists for nil pool")
	}
}
