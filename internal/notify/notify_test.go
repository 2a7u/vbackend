package notify

import (
	"context"
	"errors"
	"testing"
)

type mockFailChannel struct{}

func (m *mockFailChannel) Type() string { return "failing_channel" }
func (m *mockFailChannel) Send(ctx context.Context, recipient, title, message string, extra map[string]interface{}) error {
	return errors.New("simulated network failure")
}

type mockSuccessChannel struct {
	sent bool
}

func (m *mockSuccessChannel) Type() string { return "success_channel" }
func (m *mockSuccessChannel) Send(ctx context.Context, recipient, title, message string, extra map[string]interface{}) error {
	m.sent = true
	return nil
}

func TestNotifyFallbackOrder(t *testing.T) {
	svc := NewService(nil, nil)
	// Clear default channels
	svc.channels = nil

	failCh := &mockFailChannel{}
	successCh := &mockSuccessChannel{}

	svc.RegisterChannel(failCh)
	svc.RegisterChannel(successCh)

	err := svc.Dispatch(context.Background(), 1, EventTopup, "10", "شحن محفظة", "تم شحن المحفظة بنجاح", nil)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got %v", err)
	}
	if !successCh.sent {
		t.Fatal("expected second channel in fallback chain to receive notification")
	}
}
