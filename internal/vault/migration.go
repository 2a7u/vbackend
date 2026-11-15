package vault

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LegacyMigrationReport summarizes the results of the plaintext PIN migration.
type LegacyMigrationReport struct {
	TotalLegacyRows      int64    `json:"total_legacy_rows"`
	Processed            int64    `json:"processed"`
	EncryptedAndVerified int64    `json:"encrypted_and_verified"`
	DuplicatesFound      int64    `json:"duplicates_found"`
	Errors               []string `json:"errors"`
	DryRun               bool     `json:"dry_run"`
}

// MigrateLegacyPlaintextPINs migrates unencrypted legacy vouchers into the AES-256-GCM vault.
// It is idempotent and resumable (filtering on pin_encrypted = '').
// It computes HMAC-SHA256, catches legacy duplicates, verifies decrypt-and-compare on every row,
// and supports dry-run mode.
func MigrateLegacyPlaintextPINs(ctx context.Context, pool *pgxpool.Pool, mgr *VaultManager, dryRun bool) (*LegacyMigrationReport, error) {
	if pool == nil {
		return nil, fmt.Errorf("database pool is required for migration")
	}
	if mgr == nil {
		mgr = Default()
	}

	report := &LegacyMigrationReport{
		DryRun: dryRun,
		Errors: []string{},
	}

	// 1. Fetch unencrypted rows
	rows, err := pool.Query(ctx,
		`SELECT id, pin, serial_number
		 FROM vouchers_vault
		 WHERE pin_encrypted = '' AND pin <> ''
		 ORDER BY id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query legacy vouchers: %w", err)
	}
	defer rows.Close()

	type LegacyRow struct {
		ID     int64
		PIN    string
		Serial string
	}
	var legacyRows []LegacyRow
	for rows.Next() {
		var r LegacyRow
		if err := rows.Scan(&r.ID, &r.PIN, &r.Serial); err == nil {
			legacyRows = append(legacyRows, r)
		}
	}
	rows.Close()

	report.TotalLegacyRows = int64(len(legacyRows))

	for _, lr := range legacyRows {
		report.Processed++
		cleanPIN := strings.TrimSpace(lr.PIN)
		if cleanPIN == "" {
			continue
		}

		hmac := mgr.ComputeHMAC(cleanPIN)

		// Check if this HMAC already exists in vouchers_vault
		var duplicateCount int
		_ = pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM vouchers_vault WHERE pin_hmac = $1 AND id <> $2`,
			hmac, lr.ID,
		).Scan(&duplicateCount)

		if duplicateCount > 0 {
			report.DuplicatesFound++
			if !dryRun {
				_, _ = pool.Exec(ctx,
					`INSERT INTO legacy_migration_duplicates (legacy_voucher_id, serial_number, pin_hmac, reason)
					 VALUES ($1, $2, $3, $4)`,
					lr.ID, lr.Serial, hmac, "duplicate_hmac_found_in_vault",
				)
				// Quarantine duplicate row with unique tombstone HMAC so it does not block the total unique index
				_, _ = pool.Exec(ctx,
					`UPDATE vouchers_vault
					 SET pin = '', pin_encrypted = 'DUPLICATE_QUARANTINED', nonce = 'QUARANTINE', pin_hmac = 'DUP_' || id || '_' || $1, is_sold = TRUE
					 WHERE id = $2`,
					hmac, lr.ID,
				)
			}
			continue
		}

		// Encrypt
		ctHex, nonceHex, keyVer, encErr := mgr.EncryptPIN(cleanPIN)
		if encErr != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("row %d encrypt error: %v", lr.ID, encErr))
			continue
		}

		// Verify immediately by decrypt-and-compare
		decrypted, decErr := mgr.DecryptPIN(ctHex, nonceHex, keyVer)
		if decErr != nil || decrypted != cleanPIN {
			errMsg := fmt.Sprintf("row %d decrypt-and-compare failed: %v", lr.ID, decErr)
			report.Errors = append(report.Errors, errMsg)
			return report, fmt.Errorf("CRITICAL SECURITY VIOLATION: %s", errMsg)
		}

		if !dryRun {
			// Update the row with encryption artifacts and masked PIN
			masked := cleanPIN
			if len(cleanPIN) > 4 {
				masked = strings.Repeat("*", len(cleanPIN)-4) + cleanPIN[len(cleanPIN)-4:]
			}

			_, updateErr := pool.Exec(ctx,
				`UPDATE vouchers_vault
				 SET pin_encrypted = $1, nonce = $2, key_version = $3, pin_hmac = $4, pin = $5
				 WHERE id = $6`,
				ctHex, nonceHex, keyVer, hmac, masked, lr.ID,
			)
			if updateErr != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("row %d update error: %v", lr.ID, updateErr))
				continue
			}
		}

		report.EncryptedAndVerified++
	}

	return report, nil
}

// VerificationReport details the exact verification status of vouchers_vault.
type VerificationReport struct {
	TotalRows              int64 `json:"total_rows"`
	UnencryptedRows        int64 `json:"unencrypted_rows"`
	MissingHMACRows        int64 `json:"missing_hmac_rows"`
	RemainingPlaintextRows int64 `json:"remaining_plaintext_rows"`
	Is100PercentEncrypted  bool  `json:"is_100_percent_encrypted"`
	ZeroPlaintextConfirmed bool  `json:"zero_plaintext_confirmed"`
}

// NullifyPlaintextPINs wipes residual legacy plaintext PINs after migration verification.
// Run ONLY after MigrateLegacyPlaintextPINs has completed and been verified.
func NullifyPlaintextPINs(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	if pool == nil {
		return 0, fmt.Errorf("database pool is required")
	}

	res, err := pool.Exec(ctx,
		`UPDATE vouchers_vault
		 SET pin = ''
		 WHERE pin_encrypted <> '' AND pin_hmac <> '';`,
	)
	if err != nil {
		return 0, fmt.Errorf("nullify plaintext pins: %w", err)
	}
	return res.RowsAffected(), nil
}

// VerifyZeroPlaintext runs the exact verification query proving 100% of rows have pin_encrypted
// and pin_hmac, and fails if any non-empty plaintext PIN remains.
func VerifyZeroPlaintext(ctx context.Context, pool *pgxpool.Pool) (*VerificationReport, error) {
	if pool == nil {
		return nil, fmt.Errorf("database pool is required")
	}

	var rep VerificationReport
	err := pool.QueryRow(ctx,
		`SELECT 
			COUNT(*),
			COUNT(*) FILTER (WHERE pin_encrypted = '' OR pin_encrypted IS NULL),
			COUNT(*) FILTER (WHERE pin_hmac = '' OR pin_hmac IS NULL),
			COUNT(*) FILTER (WHERE pin <> '' AND pin IS NOT NULL)
		 FROM vouchers_vault`,
	).Scan(&rep.TotalRows, &rep.UnencryptedRows, &rep.MissingHMACRows, &rep.RemainingPlaintextRows)
	if err != nil {
		return nil, fmt.Errorf("execute plaintext verification query: %w", err)
	}

	rep.Is100PercentEncrypted = (rep.UnencryptedRows == 0 && rep.MissingHMACRows == 0)
	rep.ZeroPlaintextConfirmed = (rep.RemainingPlaintextRows == 0)

	if rep.RemainingPlaintextRows > 0 {
		return &rep, fmt.Errorf("CRITICAL SECURITY VIOLATION: detected %d rows with un-wiped plaintext PINs in vouchers_vault", rep.RemainingPlaintextRows)
	}
	if rep.UnencryptedRows > 0 || rep.MissingHMACRows > 0 {
		return &rep, fmt.Errorf("INCOMPLETE ENCRYPTION: %d unencrypted rows and %d missing HMAC rows remain in vouchers_vault", rep.UnencryptedRows, rep.MissingHMACRows)
	}

	return &rep, nil
}
