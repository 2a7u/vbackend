package refund

import (
	"context"
	"testing"
)

func TestRefundValidation(t *testing.T) {
	svc := NewService(nil, nil)
	_, err := svc.ProcessRefundTx(context.Background(), nil, RefundRequest{
		MerchantID: 1,
		Amount:     0, // invalid <= 0
		RefType:    "sale",
		RefID:      "123",
	})
	if err != ErrInvalidRefundAmount {
		t.Fatalf("expected ErrInvalidRefundAmount, got %v", err)
	}

	_, err = svc.ProcessRefundTx(context.Background(), nil, RefundRequest{
		MerchantID: 0, // invalid merchant id
		Amount:     5000,
		RefType:    "sale",
		RefID:      "123",
	})
	if err == nil {
		t.Fatal("expected error for invalid merchant id")
	}
}
