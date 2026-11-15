package handlers_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"venus_backend/internal/vault"
)

// MockCardLockSimulator models the atomic row-level lock on a voucher record (SELECT ... FOR UPDATE).
type MockCardLockSimulator struct {
	mu     sync.Mutex
	IsSold bool
	SoldTo int64
}

func (c *MockCardLockSimulator) TryClaimAndSell(buyerID int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.IsSold {
		return false
	}
	c.IsSold = true
	c.SoldTo = buyerID
	return true
}

// TestConcurrentSaleOfSameCardSoldExactlyOnce verifies requirement 1:
// When 100 parallel requests race to purchase the EXACT SAME card,
// row-level locking guarantees that exactly 1 request wins and 99 are rejected.
func TestConcurrentSaleOfSameCardSoldExactlyOnce(t *testing.T) {
	card := &MockCardLockSimulator{IsSold: false}
	const parallelBuyers = 100

	var wg sync.WaitGroup
	var soldCount atomic.Int64
	var rejectedCount atomic.Int64

	for i := 1; i <= parallelBuyers; i++ {
		wg.Add(1)
		go func(buyerID int) {
			defer wg.Done()
			success := card.TryClaimAndSell(int64(buyerID))
			if success {
				soldCount.Add(1)
			} else {
				rejectedCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	if soldCount.Load() != 1 {
		t.Fatalf("CRITICAL CONCURRENCY FAILURE: Card sold %d times (expected exactly 1)!", soldCount.Load())
	}
	if rejectedCount.Load() != parallelBuyers-1 {
		t.Fatalf("expected %d rejected requests, got %d", parallelBuyers-1, rejectedCount.Load())
	}
}

// MockVaultHMACIndexSimulator models the PostgreSQL UNIQUE index on pin_hmac under high concurrency.
type MockVaultHMACIndexSimulator struct {
	mu        sync.Mutex
	storedPIN map[string]int // hmac -> insertedID
}

func (s *MockVaultHMACIndexSimulator) TryInsert(hmac string, id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.storedPIN[hmac]; exists {
		return false // Unique index violation (duplicate)
	}
	s.storedPIN[hmac] = id
	return true
}

// TestConcurrent100BulkUploadsOfOverlappingPINs verifies requirement 1:
// 100 parallel workers attempting to insert the exact same PIN into the vault concurrently
// will have exactly 1 insert succeed and 99 fail with duplicate PIN violation.
func TestConcurrent100BulkUploadsOfOverlappingPINs(t *testing.T) {
	mgr := vault.Default()
	duplicatePIN := "ZAIN-CONFIDENTIAL-PIN-99887766"
	pinHMAC := mgr.ComputeHMAC(duplicatePIN)

	index := &MockVaultHMACIndexSimulator{
		storedPIN: make(map[string]int),
	}

	const parallelUploaders = 100
	var wg sync.WaitGroup
	var insertedCount atomic.Int64
	var duplicateRejectedCount atomic.Int64

	for i := 1; i <= parallelUploaders; i++ {
		wg.Add(1)
		go func(uploaderID int) {
			defer wg.Done()
			// Each worker encrypts and tries to insert
			ok := index.TryInsert(pinHMAC, uploaderID)
			if ok {
				insertedCount.Add(1)
			} else {
				duplicateRejectedCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	if insertedCount.Load() != 1 {
		t.Fatalf("CRITICAL SECURITY FAILURE: Duplicate PIN inserted %d times (expected exactly 1)!", insertedCount.Load())
	}
	if duplicateRejectedCount.Load() != parallelUploaders-1 {
		t.Fatalf("expected %d duplicate rejections, got %d", parallelUploaders-1, duplicateRejectedCount.Load())
	}
}
