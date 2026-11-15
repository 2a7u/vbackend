package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type IntegrityViolation struct {
	AccountID     int64     `json:"account_id"`
	EntryID       int64     `json:"entry_id"`
	TxID          string    `json:"tx_id"`
	ViolationType string    `json:"violation_type"`
	ExpectedValue string    `json:"expected_value"`
	ActualValue   string    `json:"actual_value"`
	DetectedAt    time.Time `json:"detected_at"`
}

type IntegritySummary struct {
	TotalEntriesChecked int                  `json:"total_entries_checked"`
	AccountsChecked     int                  `json:"accounts_checked"`
	ValidAccountsCount  int                  `json:"valid_accounts_count"`
	ViolationsCount     int                  `json:"violations_count"`
	Violations          []IntegrityViolation `json:"violations"`
	VerifiedAt          time.Time            `json:"verified_at"`
	IsClean             bool                 `json:"is_clean"`
}

// ChainRecord represents an entry in an account's cryptographic hash chain.
type ChainRecord struct {
	ID           int64
	TxID         uuid.UUID
	AccountID    int64
	Debit        int64
	Credit       int64
	BalanceAfter int64
	CreatedAt    time.Time
	PrevHash     string
	Hash         string
}

// VerifyAccountRecords verifies hash linkage, SHA-256 integrity, and balance continuity across an entry sequence.
func VerifyAccountRecords(accountID int64, records []ChainRecord) []IntegrityViolation {
	var violations []IntegrityViolation
	var prevHash = GenesisHash
	var prevBalance int64 = 0
	isFirst := true

	for _, rec := range records {
		// 1. Verify prev_hash linkage
		if isFirst {
			if rec.PrevHash != GenesisHash {
				violations = append(violations, IntegrityViolation{
					AccountID:     accountID,
					EntryID:       rec.ID,
					TxID:          rec.TxID.String(),
					ViolationType: "INVALID_GENESIS_PREV_HASH",
					ExpectedValue: GenesisHash,
					ActualValue:   rec.PrevHash,
					DetectedAt:    time.Now().UTC(),
				})
			}
			isFirst = false
		} else {
			if rec.PrevHash != prevHash {
				violations = append(violations, IntegrityViolation{
					AccountID:     accountID,
					EntryID:       rec.ID,
					TxID:          rec.TxID.String(),
					ViolationType: "BROKEN_HASH_CHAIN",
					ExpectedValue: prevHash,
					ActualValue:   rec.PrevHash,
					DetectedAt:    time.Now().UTC(),
				})
			}
		}

		// 2. Recompute cryptographic SHA-256 hash
		expectedHash := CalculateEntryHash(rec.PrevHash, rec.TxID, accountID, rec.Debit, rec.Credit, rec.BalanceAfter, rec.CreatedAt)
		if rec.Hash != expectedHash {
			violations = append(violations, IntegrityViolation{
				AccountID:     accountID,
				EntryID:       rec.ID,
				TxID:          rec.TxID.String(),
				ViolationType: "HASH_TAMPERED",
				ExpectedValue: expectedHash,
				ActualValue:   rec.Hash,
				DetectedAt:    time.Now().UTC(),
			})
		}

		// 3. Mathematical balance continuity
		expectedBalance := prevBalance + rec.Credit - rec.Debit
		if rec.BalanceAfter != expectedBalance {
			violations = append(violations, IntegrityViolation{
				AccountID:     accountID,
				EntryID:       rec.ID,
				TxID:          rec.TxID.String(),
				ViolationType: "BALANCE_CALCULATION_ERROR",
				ExpectedValue: fmt.Sprintf("%d", expectedBalance),
				ActualValue:   fmt.Sprintf("%d", rec.BalanceAfter),
				DetectedAt:    time.Now().UTC(),
			})
		}

		prevHash = rec.Hash
		prevBalance = rec.BalanceAfter
	}

	return violations
}

// VerifyAccountChain verifies the cryptographic hash chain and mathematical balance
// continuity for a single account from genesis to the latest entry.
func VerifyAccountChain(ctx context.Context, pool *pgxpool.Pool, accountID int64) ([]IntegrityViolation, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, tx_id, debit, credit, balance_after, created_at, prev_hash, hash
		 FROM ledger_entries
		 WHERE account_id = $1
		 ORDER BY id ASC`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("query ledger entries for account #%d: %w", accountID, err)
	}
	defer rows.Close()

	var records []ChainRecord
	for rows.Next() {
		var r ChainRecord
		r.AccountID = accountID
		if err := rows.Scan(&r.ID, &r.TxID, &r.Debit, &r.Credit, &r.BalanceAfter, &r.CreatedAt, &r.PrevHash, &r.Hash); err != nil {
			return nil, err
		}
		records = append(records, r)
	}

	return VerifyAccountRecords(accountID, records), nil
}

// VerifyGlobalIntegrity runs a complete cryptographic and double-entry audit across all accounts.
// Any violations found are recorded in the integrity_alerts table.
func VerifyGlobalIntegrity(ctx context.Context, pool *pgxpool.Pool, hub Broadcaster) (*IntegritySummary, error) {
	summary := &IntegritySummary{
		VerifiedAt: time.Now().UTC(),
		IsClean:    true,
	}

	// 1. Get all distinct accounts with entries
	rows, err := pool.Query(ctx, `SELECT DISTINCT account_id FROM ledger_entries`)
	if err != nil {
		return nil, fmt.Errorf("list ledger accounts: %w", err)
	}
	var accountIDs []int64
	for rows.Next() {
		var aid int64
		if err := rows.Scan(&aid); err == nil {
			accountIDs = append(accountIDs, aid)
		}
	}
	rows.Close()

	summary.AccountsChecked = len(accountIDs)

	for _, aid := range accountIDs {
		accountViolations, err := VerifyAccountChain(ctx, pool, aid)
		if err != nil {
			log.Printf("integrity check error for account #%d: %v", aid, err)
			continue
		}
		if len(accountViolations) > 0 {
			summary.Violations = append(summary.Violations, accountViolations...)
			summary.ViolationsCount += len(accountViolations)
			summary.IsClean = false

			// Record in integrity_alerts table
			for _, v := range accountViolations {
				detailsJSON, _ := json.Marshal(v)
				_, _ = pool.Exec(ctx,
					`INSERT INTO integrity_alerts (alert_type, account_id, entry_id, expected_value, actual_value, details)
					 VALUES ($1, $2, $3, $4, $5, $6)`,
					v.ViolationType, v.AccountID, v.EntryID, v.ExpectedValue, v.ActualValue, detailsJSON,
				)
			}
		} else {
			summary.ValidAccountsCount++
		}
	}

	// 2. Verify double-entry balance for each tx_id
	unbalancedRows, err := pool.Query(ctx,
		`SELECT tx_id, SUM(debit) AS total_debit, SUM(credit) AS total_credit
		 FROM ledger_entries
		 GROUP BY tx_id
		 HAVING SUM(debit) <> SUM(credit)`,
	)
	if err == nil {
		defer unbalancedRows.Close()
		for unbalancedRows.Next() {
			var txID uuid.UUID
			var totalDebit, totalCredit int64
			if err := unbalancedRows.Scan(&txID, &totalDebit, &totalCredit); err == nil {
				v := IntegrityViolation{
					TxID:          txID.String(),
					ViolationType: "UNBALANCED_TRANSACTION",
					ExpectedValue: fmt.Sprintf("debit=%d credit=%d", totalDebit, totalDebit),
					ActualValue:   fmt.Sprintf("debit=%d credit=%d", totalDebit, totalCredit),
					DetectedAt:    time.Now().UTC(),
				}
				summary.Violations = append(summary.Violations, v)
				summary.ViolationsCount++
				summary.IsClean = false

				detailsJSON, _ := json.Marshal(v)
				_, _ = pool.Exec(ctx,
					`INSERT INTO integrity_alerts (alert_type, tx_id, expected_value, actual_value, details)
					 VALUES ($1, $2, $3, $4, $5)`,
					v.ViolationType, txID, v.ExpectedValue, v.ActualValue, detailsJSON,
				)
			}
		}
	}

	var count int
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM ledger_entries`).Scan(&count)
	summary.TotalEntriesChecked = count

	if !summary.IsClean && hub != nil {
		hub.Broadcast(0, "integrity_alert", summary)
	}

	return summary, nil
}
