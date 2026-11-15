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

// NullifyPlaintextPINs wipes residual legacy plaintext PINs after migration verification.
// Run ONLY after MigrateLegacyPlaintextPINs has completed and been verified.
func NullifyPlaintextPINs(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	if pool == nil {
		return 0, fmt.Errorf("database pool is required")
	}

	res, err := pool.Exec(ctx,
		`UPDATE vouchers_vault
		 SET pin = ''
		 WHERE pin_encrypted <> '' AND pin <> '' AND pin NOT LIKE '***%'`,
	)
	if err != nil {
		return 0, fmt.Errorf("nullify plaintext pins: %w", err)
	}
	return res.RowsAffected(), nil
}
