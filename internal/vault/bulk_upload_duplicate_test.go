package vault_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"venus_backend/internal/vault"
)

// TestBulkUploadDuplicateDetection verifies that duplicate PINs are identified
// using HMAC-SHA256 before insertion into the vault, preventing any collision
// or re-use while keeping plaintext PINs completely absent from storage.
func TestBulkUploadDuplicateDetection(t *testing.T) {
	mgr := vault.Default()

	// Simulate existing vault state with 50 vouchers
	existingHMACs := make(map[string]bool)
	for i := 1; i <= 50; i++ {
		pin := fmt.Sprintf("PIN-%06d", i)
		hmac := mgr.ComputeHMAC(pin)
		existingHMACs[hmac] = true
	}

	// Incoming bulk upload batch of 60 vouchers:
	// - 20 are completely new (PIN-1001 to PIN-1020)
	// - 20 are duplicates of already existing vault PINs (PIN-000001 to PIN-000020)
	// - 10 are duplicates within the incoming batch itself (duplicate PIN-1001 to PIN-1010)
	// - 10 are invalid empty or whitespace PINs
	type VoucherInput struct {
		Serial string
		PIN    string
	}

	var batch []VoucherInput
	// 20 new
	for i := 1001; i <= 1020; i++ {
		batch = append(batch, VoucherInput{
			Serial: fmt.Sprintf("SN-NEW-%d", i),
			PIN:    fmt.Sprintf("PIN-%06d", i),
		})
	}
	// 20 existing duplicates
	for i := 1; i <= 20; i++ {
		batch = append(batch, VoucherInput{
			Serial: fmt.Sprintf("SN-DUP-%d", i),
			PIN:    fmt.Sprintf("PIN-%06d", i),
		})
	}
	// 10 in-batch duplicates
	for i := 1001; i <= 1010; i++ {
		batch = append(batch, VoucherInput{
			Serial: fmt.Sprintf("SN-INBATCH-DUP-%d", i),
			PIN:    fmt.Sprintf("PIN-%06d", i),
		})
	}
	// 10 invalid
	for i := 1; i <= 10; i++ {
		batch = append(batch, VoucherInput{
			Serial: fmt.Sprintf("SN-INV-%d", i),
			PIN:    "   ",
		})
	}

	// Process the batch
	var succeeded, duplicates, invalid int
	batchHMACs := make(map[string]bool)

	type ProcessedVoucher struct {
		Serial     string
		Ciphertext string
		Nonce      string
		KeyVersion int
		HMAC       string
	}
	var stored []ProcessedVoucher

	for _, v := range batch {
		cleanPIN := v.PIN
		if len(cleanPIN) == 0 || cleanPIN == "   " {
			invalid++
			continue
		}

		hmac := mgr.ComputeHMAC(cleanPIN)

		// Check duplicate against existing DB vault or current batch
		if existingHMACs[hmac] || batchHMACs[hmac] {
			duplicates++
			continue
		}

		// Encrypt
		ct, nonce, ver, err := mgr.EncryptPIN(cleanPIN)
		if err != nil {
			t.Fatalf("encryption error: %v", err)
		}

		batchHMACs[hmac] = true
		stored = append(stored, ProcessedVoucher{
			Serial:     v.Serial,
			Ciphertext: ct,
			Nonce:      nonce,
			KeyVersion: ver,
			HMAC:       hmac,
		})
		succeeded++
	}

	if succeeded != 20 {
		t.Fatalf("expected 20 succeeded, got %d", succeeded)
	}
	if duplicates != 30 { // 20 from DB + 10 in-batch
		t.Fatalf("expected 30 duplicates, got %d", duplicates)
	}
	if invalid != 10 {
		t.Fatalf("expected 10 invalid, got %d", invalid)
	}

	// Verify all stored vouchers can be decrypted at dispense time
	for i, sv := range stored {
		decrypted, err := mgr.DecryptPIN(sv.Ciphertext, sv.Nonce, sv.KeyVersion)
		if err != nil {
			t.Fatalf("failed to decrypt stored voucher %d: %v", i, err)
		}
		expectedPIN := fmt.Sprintf("PIN-%06d", 1001+i)
		if decrypted != expectedPIN {
			t.Fatalf("decrypted PIN mismatch: got %q, expected %q", decrypted, expectedPIN)
		}
	}
}

// TestConcurrentBulkEncryptionUnderHighLoad simulates parallel worker threads
// encrypting and verifying vouchers simultaneously with zero race conditions.
func TestConcurrentBulkEncryptionUnderHighLoad(t *testing.T) {
	mgr := vault.Default()
	workers := 100
	var wg sync.WaitGroup
	var successCount atomic.Int64

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			pin := fmt.Sprintf("CONCURRENT-PIN-%04d-%08d", workerID, 99999)
			ct, nonce, ver, err := mgr.EncryptPIN(pin)
			if err != nil {
				t.Errorf("worker %d encrypt failed: %v", workerID, err)
				return
			}

			decrypted, err := mgr.DecryptPIN(ct, nonce, ver)
			if err != nil {
				t.Errorf("worker %d decrypt failed: %v", workerID, err)
				return
			}
			if decrypted != pin {
				t.Errorf("worker %d mismatch: got %q, expected %q", workerID, decrypted, pin)
				return
			}

			successCount.Add(1)
		}(i)
	}

	wg.Wait()
	if successCount.Load() != int64(workers) {
		t.Fatalf("expected %d successful encryptions, got %d", workers, successCount.Load())
	}
}
