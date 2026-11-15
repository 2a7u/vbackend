package deviceradar_test

import (
	"context"
	"testing"
	"time"

	"venus_backend/internal/deviceradar"
)

func TestLiveDevicePresence(t *testing.T) {
	svc := deviceradar.NewService(nil, nil)

	ctx := context.Background()
	svc.RecordHeartbeat(ctx, 101, "store1@uruk.pos", "محل النور", "Galaxy S22", "1.5.0", 35, "pos_checkout")
	svc.RecordHeartbeat(ctx, 102, "store2@uruk.pos", "محل الرافدين", "iPhone 14", "1.5.0", 22, "inventory")

	online := svc.GetOnlineDevices()
	if len(online) != 2 {
		t.Fatalf("expected 2 online devices, got %d", len(online))
	}

	// Remove one
	svc.RemoveDevice(101)
	onlineAfter := svc.GetOnlineDevices()
	if len(onlineAfter) != 1 {
		t.Fatalf("expected 1 online device, got %d", len(onlineAfter))
	}
	if onlineAfter[0].UserID != 102 {
		t.Fatalf("expected user 102 to remain online")
	}
}

func TestRemoteCommandWhitelistValidation(t *testing.T) {
	svc := deviceradar.NewService(nil, nil)
	ctx := context.Background()

	// Whitelisted commands must not be rejected upfront by validation
	_, err := svc.QueueCommand(ctx, 101, "unauthorized_malicious_exec", nil, 5*time.Minute)
	if err == nil {
		t.Fatalf("expected unauthorized command to be strictly rejected")
	}
}
