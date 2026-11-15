package seal

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSealSignatureVerification(t *testing.T) {
	svc := NewService(nil, "my-super-secret-key-12345", "https://vapi.urukiq.com")

	receiptID := "sale_999888"
	amount := int64(15000)
	issuedAt := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

	sig1 := svc.GenerateSignature(receiptID, amount, issuedAt)
	sig2 := svc.GenerateSignature(receiptID, amount, issuedAt)
	if sig1 != sig2 {
		t.Fatal("expected deterministic HMAC signature")
	}

	// Tampered amount must produce different signature
	tamperedSig := svc.GenerateSignature(receiptID, 16000, issuedAt)
	if sig1 == tamperedSig {
		t.Fatal("tampered amount should produce different signature")
	}

	token, err := svc.CreateToken(context.Background(), 10, "voucher", receiptID, amount)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(token.Token, "v-") {
		t.Fatalf("expected token prefix v-, got %s", token.Token)
	}

	url := svc.FormatVerificationURL(token.Token)
	if !strings.Contains(url, "/verify/v-") {
		t.Fatalf("unexpected verification URL: %s", url)
	}
}
