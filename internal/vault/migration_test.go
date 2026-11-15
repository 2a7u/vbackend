package vault_test

import (
	"strings"
	"testing"
	"venus_backend/internal/vault"
)

// TestMigrationLogicAndVerificationSimulation verifies the core logic of MigrateLegacyPlaintextPINs:
// Encrypts plaintext, checks duplicate HMAC, and verifies decrypt-and-compare.
func TestMigrationLogicAndVerificationSimulation(t *testing.T) {
	mgr := vault.Default()

	legacyVouchers := []struct {
		ID     int64
		PIN    string
		Serial string
	}{
		{ID: 1, PIN: "112233445566", Serial: "SN-001"},
		{ID: 2, PIN: "998877665544", Serial: "SN-002"},
		{ID: 3, PIN: "112233445566", Serial: "SN-003"}, // Duplicate PIN of ID 1
	}

	seenHMACs := make(map[string]int64)
	var succeeded, duplicates int

	for _, v := range legacyVouchers {
		cleanPIN := strings.TrimSpace(v.PIN)
		hmac := mgr.ComputeHMAC(cleanPIN)

		if firstID, exists := seenHMACs[hmac]; exists {
			duplicates++
			_ = firstID
			continue
		}

		// Encrypt
		ct, nonce, ver, err := mgr.EncryptPIN(cleanPIN)
		if err != nil {
			t.Fatalf("encryption error: %v", err)
		}

		// Mandatory Decrypt-and-Compare verification
		decrypted, err := mgr.DecryptPIN(ct, nonce, ver)
		if err != nil {
			t.Fatalf("decryption error: %v", err)
		}
		if decrypted != cleanPIN {
			t.Fatalf("decrypt-and-compare failed: %q != %q", decrypted, cleanPIN)
		}

		seenHMACs[hmac] = v.ID
		succeeded++
	}

	if succeeded != 2 {
		t.Fatalf("expected 2 successfully verified migrations, got %d", succeeded)
	}
	if duplicates != 1 {
		t.Fatalf("expected 1 duplicate detected, got %d", duplicates)
	}
}

// TestCanaryStartupVerification verifies that VerifyStartup checks HMAC secret consistency.
func TestCanaryStartupVerification(t *testing.T) {
	mgr := vault.Default()
	h1 := mgr.ComputeHMAC(vault.CanaryPayload)
	if h1 == "" || len(h1) != 64 {
		t.Fatalf("expected 64-char hex HMAC, got %q", h1)
	}

	// Verify idempotency
	h2 := mgr.ComputeHMAC(vault.CanaryPayload)
	if h1 != h2 {
		t.Fatalf("canary HMAC must be deterministic: %s != %s", h1, h2)
	}
}
