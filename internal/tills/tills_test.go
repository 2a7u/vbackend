package tills

import (
	"context"
	"testing"
)

func TestCashierPermissionsMasking(t *testing.T) {
	adminPerms := GetPermissions("admin")
	if !adminPerms.CanViewCosts || !adminPerms.CanViewProfits {
		t.Fatal("admin should be able to view costs and profits")
	}

	cashierPerms := GetPermissions("cashier")
	if cashierPerms.CanViewCosts || cashierPerms.CanViewProfits {
		t.Fatal("cashier must NOT be able to view costs or profits")
	}
	if !cashierPerms.CanCloseShift {
		t.Fatal("cashier should be able to close shift")
	}
}

func TestOpenShiftNilPool(t *testing.T) {
	svc := NewService(nil)
	shift, err := svc.OpenShift(context.Background(), 1, 2, "device_abc", 50000)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if shift.Status != "open" {
		t.Fatalf("expected open status, got %s", shift.Status)
	}
	if shift.StartingCash != 50000 {
		t.Fatalf("expected starting cash 50000, got %d", shift.StartingCash)
	}
}
