package vault

import (
	"context"
	"fmt"
	"strings"
	"time"

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

// LegacyRow represents an unencrypted legacy card in vouchers_vault.
type LegacyRow struct {
	ID     int64
	PIN    string
	Serial string
	IsSold bool
	Status string
	SoldAt *time.Time
}

// MigrateLegacyPlaintextPINs migrates unencrypted legacy vouchers into the AES-256-GCM vault with default batch size.
func MigrateLegacyPlaintextPINs(ctx context.Context, pool *pgxpool.Pool, mgr *VaultManager, dryRun bool) (*LegacyMigrationReport, error) {
	return MigrateLegacyPlaintextPINsBatched(ctx, pool, mgr, dryRun, 500)
}

// MigrateLegacyPlaintextPINsBatched migrates unencrypted legacy vouchers into the AES-256-GCM vault in batches.
// It is idempotent and resumable (filtering on pin_encrypted = '').
// It computes HMAC-SHA256, enforces canonical selection (sold row wins, or earliest if neither/both sold),
// isolates duplicates with status='quarantined' and is_sold=FALSE, and maintains 100% dry-run vs live parity across batches.
func MigrateLegacyPlaintextPINsBatched(ctx context.Context, pool *pgxpool.Pool, mgr *VaultManager, dryRun bool, batchSize int) (*LegacyMigrationReport, error) {
	if pool == nil {
		return nil, fmt.Errorf("database pool is required for migration")
	}
	if mgr == nil {
		mgr = Default()
	}
	if batchSize <= 0 {
		batchSize = 500
	}

	report := &LegacyMigrationReport{
		DryRun: dryRun,
		Errors: []string{},
	}

	// 1. Fetch unencrypted legacy rows
	rows, err := pool.Query(ctx,
		`SELECT id, pin, serial_number, is_sold, status, sold_at
		 FROM vouchers_vault
		 WHERE pin_encrypted = '' AND pin <> ''
		 ORDER BY id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query legacy vouchers: %w", err)
	}
	defer rows.Close()

	var legacyRows []LegacyRow
	for rows.Next() {
		var r LegacyRow
		if err := rows.Scan(&r.ID, &r.PIN, &r.Serial, &r.IsSold, &r.Status, &r.SoldAt); err == nil {
			legacyRows = append(legacyRows, r)
		}
	}
	rows.Close()

	report.TotalLegacyRows = int64(len(legacyRows))
	if len(legacyRows) == 0 {
		return report, nil
	}

	// 2. Pre-scan and group by HMAC across the entire unencrypted dataset.
	// This guarantees identical canonical selection and duplicate detection regardless of batch boundaries.
	type hmacGroup struct {
		hmac string
		rows []LegacyRow
	}
	groups := make(map[string]*hmacGroup)
	var groupOrder []string

	for _, lr := range legacyRows {
		cleanPIN := strings.TrimSpace(lr.PIN)
		if cleanPIN == "" {
			continue
		}
		hmac := mgr.ComputeHMAC(cleanPIN)
		if g, exists := groups[hmac]; exists {
			g.rows = append(g.rows, lr)
		} else {
			groups[hmac] = &hmacGroup{
				hmac: hmac,
				rows: []LegacyRow{lr},
			}
			groupOrder = append(groupOrder, hmac)
		}
	}

	// canonicalMap: id -> true if canonical, false if duplicate
	canonicalMap := make(map[int64]bool, len(legacyRows))

	for _, hmac := range groupOrder {
		g := groups[hmac]

		// Check if an existing row in vouchers_vault already holds this HMAC
		var existingCount int
		_ = pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM vouchers_vault WHERE pin_hmac = $1`,
			hmac,
		).Scan(&existingCount)

		if existingCount > 0 {
			// A canonical row already exists in the vault for this HMAC; ALL new legacy rows are duplicates
			for _, r := range g.rows {
				canonicalMap[r.ID] = false
			}
			continue
		}

		// Within this legacy HMAC group, select the single canonical row:
		// Rule: The row already sold is canonical.
		// If multiple sold, or neither sold, earliest id ASC is canonical.
		var canonicalID int64 = -1

		// First pass: look for sold row
		for _, r := range g.rows {
			isSold := (r.Status == "sold" || r.IsSold || r.SoldAt != nil)
			if isSold {
				canonicalID = r.ID
				break // Earliest sold row wins
			}
		}

		// Second pass: if none were sold, earliest id wins
		if canonicalID == -1 {
			canonicalID = g.rows[0].ID
		}

		for _, r := range g.rows {
			if r.ID == canonicalID {
				canonicalMap[r.ID] = true
			} else {
				canonicalMap[r.ID] = false
			}
		}
	}

	// 3. Process rows in configurable batches
	for i := 0; i < len(legacyRows); i += batchSize {
		end := i + batchSize
		if end > len(legacyRows) {
			end = len(legacyRows)
		}
		batch := legacyRows[i:end]

		for _, lr := range batch {
			report.Processed++
			cleanPIN := strings.TrimSpace(lr.PIN)
			if cleanPIN == "" {
				continue
			}

			hmac := mgr.ComputeHMAC(cleanPIN)
			isCanonical := canonicalMap[lr.ID]

			if !isCanonical {
				// DUPLICATE DETECTED: quarantine row
				report.DuplicatesFound++
				if !dryRun {
					dupCt, dupNonce, dupKeyVer, dupEncErr := mgr.EncryptPIN(cleanPIN)
					if dupEncErr != nil {
						report.Errors = append(report.Errors, fmt.Sprintf("duplicate encrypt error for row %d: %v", lr.ID, dupEncErr))
					} else {
						_, insErr := pool.Exec(ctx,
							`INSERT INTO legacy_migration_duplicates (legacy_voucher_id, serial_number, pin_hmac, reason, pin_encrypted, nonce)
							 VALUES ($1, $2, $3, $4, $5, $6)`,
							lr.ID, lr.Serial, hmac, "duplicate_hmac_found_in_vault", dupCt, dupNonce,
						)
						if insErr != nil {
							report.Errors = append(report.Errors, fmt.Sprintf("insert legacy_migration_duplicates for row %d: %v", lr.ID, insErr))
						}
						// Quarantine duplicate row with dedicated status='quarantined', is_sold=FALSE,
						// AES-256-GCM encrypted PIN, wiped plaintext, and unique tombstone HMAC.
						_, updErr := pool.Exec(ctx,
							`UPDATE vouchers_vault
							 SET status = 'quarantined', is_sold = FALSE, pin = '', pin_encrypted = $1, nonce = $2, key_version = $3, pin_hmac = 'DUP_' || id || '_' || $4
							 WHERE id = $5`,
							dupCt, dupNonce, dupKeyVer, hmac, lr.ID,
						)
						if updErr != nil {
							report.Errors = append(report.Errors, fmt.Sprintf("update quarantine for row %d: %v", lr.ID, updErr))
						}
					}
				}
				continue
			}

			// CANONICAL ROW: Encrypt and verify
			ctHex, nonceHex, keyVer, encErr := mgr.EncryptPIN(cleanPIN)
			if encErr != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("row %d encrypt error: %v", lr.ID, encErr))
				continue
			}

			// Decrypt-and-compare verification
			decrypted, decErr := mgr.DecryptPIN(ctHex, nonceHex, keyVer)
			if decErr != nil || decrypted != cleanPIN {
				errMsg := fmt.Sprintf("row %d decrypt-and-compare failed: %v", lr.ID, decErr)
				report.Errors = append(report.Errors, errMsg)
				return report, fmt.Errorf("CRITICAL SECURITY VIOLATION: %s", errMsg)
			}

			if !dryRun {
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
