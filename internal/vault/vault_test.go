package vault_test

import (
	"testing"
	"venus_backend/internal/vault"
)

func TestVaultAES256GCMEncryptionRoundtrip(t *testing.T) {
	vm := vault.NewVaultManagerFromEnv()
	rawPIN := "7492048591829401"

	encHex, nonceHex, keyVer, err := vm.EncryptPIN(rawPIN)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	if encHex == rawPIN {
		t.Fatalf("plaintext PIN was not encrypted!")
	}
	if len(nonceHex) != 24 { // 12 bytes = 24 hex chars
		t.Fatalf("expected 24-char hex nonce, got %d chars", len(nonceHex))
	}
	if keyVer != 1 {
		t.Fatalf("expected keyVer 1, got %d", keyVer)
	}

	decrypted, err := vm.DecryptPIN(encHex, nonceHex, keyVer)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if decrypted != rawPIN {
		t.Fatalf("decrypted PIN mismatch: got %q, want %q", decrypted, rawPIN)
	}
}

func TestVaultUniqueNoncePerEncryption(t *testing.T) {
	vm := vault.NewVaultManagerFromEnv()
	rawPIN := "1234567890123456"

	enc1, nonce1, _, err1 := vm.EncryptPIN(rawPIN)
	enc2, nonce2, _, err2 := vm.EncryptPIN(rawPIN)

	if err1 != nil || err2 != nil {
		t.Fatalf("encryptions failed: %v, %v", err1, err2)
	}

	if nonce1 == nonce2 {
		t.Fatalf("nonces must be unique! got duplicate nonce: %s", nonce1)
	}
	if enc1 == enc2 {
		t.Fatalf("ciphertexts must be different for different nonces! got duplicate: %s", enc1)
	}
}

func TestVaultHMACDuplicateDetection(t *testing.T) {
	vm := vault.NewVaultManagerFromEnv()
	pinA := "9988776655443322"
	pinB := "9988776655443322"
	pinC := "1122334455667788"

	hmacA := vm.ComputeHMAC(pinA)
	hmacB := vm.ComputeHMAC(pinB)
	hmacC := vm.ComputeHMAC(pinC)

	if hmacA == "" || len(hmacA) != 64 {
		t.Fatalf("invalid HMAC-SHA256 length: %d", len(hmacA))
	}

	if hmacA != hmacB {
		t.Fatalf("identical PINs must yield identical HMACs for duplicate checking")
	}

	if hmacA == hmacC {
		t.Fatalf("different PINs must yield different HMACs")
	}
}

func TestVaultKeyRotationSupport(t *testing.T) {
	vm := vault.NewVaultManagerFromEnv()
	pin1 := "PIN_UNDER_KEY_V1"
	pin2 := "PIN_UNDER_KEY_V2"

	enc1, nonce1, ver1, err := vm.EncryptPIN(pin1)
	if err != nil || ver1 != 1 {
		t.Fatalf("encrypt under v1 failed: %v", err)
	}

	// Rotate to key version 2
	vm.SetKey(2, "NEW_ROTATED_SUPER_SECRET_KEY_FOR_2027")

	enc2, nonce2, ver2, err := vm.EncryptPIN(pin2)
	if err != nil || ver2 != 2 {
		t.Fatalf("encrypt under v2 failed: %v (ver %d)", err, ver2)
	}

	// Verify both v1 and v2 records decrypt accurately
	dec1, err := vm.DecryptPIN(enc1, nonce1, 1)
	if err != nil || dec1 != pin1 {
		t.Fatalf("failed to decrypt v1 PIN: %v", err)
	}

	dec2, err := vm.DecryptPIN(enc2, nonce2, 2)
	if err != nil || dec2 != pin2 {
		t.Fatalf("failed to decrypt v2 PIN: %v", err)
	}
}

func TestVaultTamperDetectionRejectsCorruptedCiphertext(t *testing.T) {
	vm := vault.NewVaultManagerFromEnv()
	pin := "CONFIDENTIAL_TOP_SECRET"

	enc, nonce, ver, _ := vm.EncryptPIN(pin)

	// Corrupt first byte of ciphertext
	corrupted := "00" + enc[2:]
	if corrupted == enc {
		corrupted = "ff" + enc[2:]
	}

	_, err := vm.DecryptPIN(corrupted, nonce, ver)
	if err == nil {
		t.Fatalf("tampered ciphertext must be rejected by AES-GCM authentication tag!")
	}
}
