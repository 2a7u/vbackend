package ledger

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

const OpeningLedgerMetaKey = "opening_ledger_v1_seeded"

// SeedOpeningBalancesFromLegacy performs an idempotent opening-ledger migration:
// For every pre-existing user with wallet_balance > 0, it creates balanced double-entry
// ledger records (debiting Settlement and crediting User), ensuring 100% hash chain
// integrity from day one without mutating or dropping legacy columns.
func (s *Service) SeedOpeningBalancesFromLegacy(ctx context.Context) error {
	var alreadyApplied bool
	err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM system_meta WHERE key = $1)`,
		OpeningLedgerMetaKey,
	).Scan(&alreadyApplied)
	if err != nil {
		return fmt.Errorf("check opening ledger migration marker: %w", err)
	}
	if alreadyApplied {
		return nil
	}

	settlementID, err := s.GetSystemAccountID(ctx, SystemEmailSettlement)
	if err != nil {
		return fmt.Errorf("get settlement account id: %w", err)
	}

	rows, err := s.Pool.Query(ctx,
		`SELECT id, wallet_balance
		 FROM users
		 WHERE wallet_balance > 0 AND role = 'subscriber'`,
	)
	if err != nil {
		return fmt.Errorf("query users with balance: %w", err)
	}
	defer rows.Close()

	type balanceSeed struct {
		UserID  int64
		Balance int64
	}
	var toSeed []balanceSeed
	for rows.Next() {
		var uid int64
		var bal float64
		if err := rows.Scan(&uid, &bal); err == nil && int64(bal) > 0 {
			toSeed = append(toSeed, balanceSeed{UserID: uid, Balance: int64(bal)})
		}
	}
	rows.Close()

	seededCount := 0
	for _, item := range toSeed {
		tx, txErr := s.Pool.Begin(ctx)
		if txErr != nil {
			return fmt.Errorf("begin seed tx: %w", txErr)
		}

		var hasOpening bool
		_ = tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM ledger_entries WHERE account_id = $1 AND ref_type = 'opening_balance')`,
			item.UserID,
		).Scan(&hasOpening)

		if !hasOpening {
			now := time.Now().UTC()
			txID := uuid.New()

			// 1. User Leg (Credit)
			userPrevHash := GenesisHash
			_ = tx.QueryRow(ctx, `SELECT hash FROM ledger_entries WHERE account_id = $1 ORDER BY id DESC LIMIT 1`, item.UserID).Scan(&userPrevHash)
			userHash := CalculateEntryHash(userPrevHash, txID, item.UserID, 0, item.Balance, item.Balance, now)

			_, err = tx.Exec(ctx,
				`INSERT INTO ledger_entries (
					tx_id, account_id, debit, credit, balance_after, currency,
					ref_type, ref_id, created_at, prev_hash, hash
				 ) VALUES ($1, $2, 0, $3, $4, 'IQD', 'opening_balance', 'migration_v1', $5, $6, $7)`,
				txID, item.UserID, item.Balance, item.Balance, now, userPrevHash, userHash,
			)
			if err != nil {
				tx.Rollback(ctx)
				return fmt.Errorf("insert user opening leg for #%d: %w", item.UserID, err)
			}

			// 2. Settlement Leg (Debit)
			var settlementCurrentBal float64
			_ = tx.QueryRow(ctx, `SELECT COALESCE(wallet_balance, 0) FROM users WHERE id = $1 FOR UPDATE`, settlementID).Scan(&settlementCurrentBal)
			settlementAfter := int64(settlementCurrentBal) - item.Balance

			settlementPrevHash := GenesisHash
			_ = tx.QueryRow(ctx, `SELECT hash FROM ledger_entries WHERE account_id = $1 ORDER BY id DESC LIMIT 1`, settlementID).Scan(&settlementPrevHash)
			settlementHash := CalculateEntryHash(settlementPrevHash, txID, settlementID, item.Balance, 0, settlementAfter, now)

			_, err = tx.Exec(ctx,
				`INSERT INTO ledger_entries (
					tx_id, account_id, debit, credit, balance_after, currency,
					ref_type, ref_id, created_at, prev_hash, hash
				 ) VALUES ($1, $2, $3, 0, $4, 'IQD', 'opening_balance', 'migration_v1', $5, $6, $7)`,
				txID, settlementID, item.Balance, settlementAfter, now, settlementPrevHash, settlementHash,
			)
			if err != nil {
				tx.Rollback(ctx)
				return fmt.Errorf("insert settlement opening leg for #%d: %w", item.UserID, err)
			}

			_, _ = tx.Exec(ctx, `UPDATE users SET wallet_balance = $1, updated_at = now() WHERE id = $2`, settlementAfter, settlementID)

			if cErr := tx.Commit(ctx); cErr != nil {
				return fmt.Errorf("commit opening seed: %w", cErr)
			}
			seededCount++
		} else {
			tx.Rollback(ctx)
		}
	}

	_, _ = s.Pool.Exec(ctx,
		`INSERT INTO system_meta (key) VALUES ($1) ON CONFLICT (key) DO NOTHING`,
		OpeningLedgerMetaKey,
	)

	log.Printf("opening ledger migration completed: seeded %d existing accounts into double-entry ledger", seededCount)
	return nil
}
