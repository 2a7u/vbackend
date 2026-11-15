package ledger_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"venus_backend/internal/ledger"
)

// TestIntegrityCheckerDetectsTamperedBalanceAndHash verifies requirement 5:
// When a row in ledger_entries is tampered with (e.g. bypassing DB triggers via direct disk
// or administrator tampering), the cryptographic hash chain and mathematical continuity checks
// detect the anomaly and raise a critical alert with exact expected vs actual values.
func TestIntegrityCheckerDetectsTamperedBalanceAndHash(t *testing.T) {
	accountID := int64(1001)
	baseTime := time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC)

	// Build a valid chain of 5 ledger records
	var records []ledger.ChainRecord
	prevHash := ledger.GenesisHash
	var currentBalance int64 = 0

	for i := int64(1); i <= 5; i++ {
		credit := i * 50000 // 50k, 100k, 150k, 200k, 250k
		debit := int64(0)
		balanceAfter := currentBalance + credit - debit
		txID := uuid.New()
		createdAt := baseTime.Add(time.Duration(i) * time.Hour)

		h := ledger.CalculateEntryHash(prevHash, txID, accountID, debit, credit, balanceAfter, createdAt)
		records = append(records, ledger.ChainRecord{
			ID:           i,
			TxID:         txID,
			AccountID:    accountID,
			Debit:        debit,
			Credit:       credit,
			BalanceAfter: balanceAfter,
			CreatedAt:    createdAt,
			PrevHash:     prevHash,
			Hash:         h,
		})
		prevHash = h
		currentBalance = balanceAfter
	}

	// 1. Verify clean chain reports ZERO violations
	cleanViolations := ledger.VerifyAccountRecords(accountID, records)
	if len(cleanViolations) != 0 {
		t.Fatalf("expected 0 violations for pristine chain, got %d", len(cleanViolations))
	}

	// 2. Tamper row #3: Attacker artificially inflates balance without recomputing SHA-256
	tamperedRecords := make([]ledger.ChainRecord, len(records))
	copy(tamperedRecords, records)

	originalBal := tamperedRecords[2].BalanceAfter
	tamperedRecords[2].BalanceAfter += 100000 // +100,000 IQD unauthorized balance

	violations := ledger.VerifyAccountRecords(accountID, tamperedRecords)
	if len(violations) == 0 {
		t.Fatalf("CRITICAL SECURITY FAILURE: Integrity checker failed to detect tampered row!")
	}

	var foundHashTampered, foundBalanceMismatch bool
	for _, v := range violations {
		if v.EntryID == 3 && v.ViolationType == "HASH_TAMPERED" {
			foundHashTampered = true
		}
		if v.EntryID == 3 && v.ViolationType == "BALANCE_CALCULATION_ERROR" {
			foundBalanceMismatch = true
			if v.ActualValue != fmt.Sprintf("%d", originalBal+100000) {
				t.Fatalf("expected actual value %d, got %s", originalBal+100000, v.ActualValue)
			}
		}
	}

	if !foundHashTampered {
		t.Fatalf("expected HASH_TAMPERED alert on entry #3")
	}
	if !foundBalanceMismatch {
		t.Fatalf("expected BALANCE_CALCULATION_ERROR alert on entry #3")
	}
}

// TestIntegrityCheckerDetectsBrokenChainLinkage verifies that modifying prev_hash
// breaks the chain and triggers BROKEN_HASH_CHAIN alert.
func TestIntegrityCheckerDetectsBrokenChainLinkage(t *testing.T) {
	accountID := int64(2002)
	baseTime := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)

	var records []ledger.ChainRecord
	prevHash := ledger.GenesisHash
	var currentBalance int64 = 0

	for i := int64(1); i <= 3; i++ {
		credit := int64(20000)
		debit := int64(0)
		balanceAfter := currentBalance + credit
		txID := uuid.New()
		createdAt := baseTime.Add(time.Duration(i) * time.Hour)
		h := ledger.CalculateEntryHash(prevHash, txID, accountID, debit, credit, balanceAfter, createdAt)

		records = append(records, ledger.ChainRecord{
			ID:           i,
			TxID:         txID,
			AccountID:    accountID,
			Debit:        debit,
			Credit:       credit,
			BalanceAfter: balanceAfter,
			CreatedAt:    createdAt,
			PrevHash:     prevHash,
			Hash:         h,
		})
		prevHash = h
		currentBalance = balanceAfter
	}

	// Tamper entry #2 prev_hash
	records[1].PrevHash = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

	violations := ledger.VerifyAccountRecords(accountID, records)
	if len(violations) == 0 {
		t.Fatalf("expected broken chain alert, got 0 violations")
	}

	var foundBrokenChain bool
	for _, v := range violations {
		if v.EntryID == 2 && v.ViolationType == "BROKEN_HASH_CHAIN" {
			foundBrokenChain = true
		}
	}

	if !foundBrokenChain {
		t.Fatalf("expected BROKEN_HASH_CHAIN on entry #2")
	}
}

// TestIntegrityCheckerDetectsInvalidGenesisPrevHash verifies that entry #1 must link to GenesisHash.
func TestIntegrityCheckerDetectsInvalidGenesisPrevHash(t *testing.T) {
	accountID := int64(3003)
	txID := uuid.New()
	createdAt := time.Now().UTC()

	records := []ledger.ChainRecord{
		{
			ID:           1,
			TxID:         txID,
			AccountID:    accountID,
			Debit:        0,
			Credit:       10000,
			BalanceAfter: 10000,
			CreatedAt:    createdAt,
			PrevHash:     "invalid_genesis_hash",
			Hash:         ledger.CalculateEntryHash("invalid_genesis_hash", txID, accountID, 0, 10000, 10000, createdAt),
		},
	}

	violations := ledger.VerifyAccountRecords(accountID, records)
	if len(violations) == 0 {
		t.Fatalf("expected INVALID_GENESIS_PREV_HASH alert, got none")
	}

	if violations[0].ViolationType != "INVALID_GENESIS_PREV_HASH" {
		t.Fatalf("expected INVALID_GENESIS_PREV_HASH, got %s", violations[0].ViolationType)
	}
}
