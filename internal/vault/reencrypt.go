package vault

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ReencryptionReport details the batch re-encryption execution.
type ReencryptionReport struct {
	TargetVersion int      `json:"target_version"`
	TotalVouchers int64    `json:"total_vouchers"`
	Reencrypted   int64    `json:"reencrypted"`
	Failed        int64    `json:"failed"`
	Errors        []string `json:"errors"`
}

// ReencryptVaultVouchers migrates vouchers encrypted under older key versions to targetKeyVersion.
// It is batch-oriented, resumable, and verifies every row before updating.
func ReencryptVaultVouchers(ctx context.Context, pool *pgxpool.Pool, mgr *VaultManager, targetKeyVersion int, batchSize int) (*ReencryptionReport, error) {
	if pool == nil {
		return nil, fmt.Errorf("database pool is required")
	}
	if mgr == nil {
		mgr = Default()
	}
	if batchSize <= 0 {
		batchSize = 100
	}

	report := &ReencryptionReport{
		TargetVersion: targetKeyVersion,
		Errors:        []string{},
	}

	// 1. Fetch vouchers with key_version < targetKeyVersion
	rows, err := pool.Query(ctx,
		`SELECT id, pin_encrypted, nonce, key_version, serial_number
		 FROM vouchers_vault
		 WHERE key_version < $1 AND pin_encrypted <> ''
		 ORDER BY id ASC
		 LIMIT $2`,
		targetKeyVersion, batchSize,
	)
	if err != nil {
		return nil, fmt.Errorf("query vouchers for re-encryption: %w", err)
	}
	defer rows.Close()

	type VoucherRow struct {
		ID     int64
		CtHex  string
		Nonce  string
		KeyVer int
		Serial string
	}
	var batch []VoucherRow
	for rows.Next() {
		var vr VoucherRow
		if err := rows.Scan(&vr.ID, &vr.CtHex, &vr.Nonce, &vr.KeyVer, &vr.Serial); err == nil {
			batch = append(batch, vr)
		}
	}
	rows.Close()

	report.TotalVouchers = int64(len(batch))

	for _, vr := range batch {
		// 1. Decrypt with old key
		decrypted, decErr := mgr.DecryptPIN(vr.CtHex, vr.Nonce, vr.KeyVer)
		if decErr != nil {
			report.Failed++
			report.Errors = append(report.Errors, fmt.Sprintf("voucher %d decrypt failed: %v", vr.ID, decErr))
			continue
		}

		// 2. Re-encrypt with target key version
		newCt, newNonce, newVer, encErr := mgr.EncryptPINWithVersion(decrypted, targetKeyVersion)
		if encErr != nil {
			report.Failed++
			report.Errors = append(report.Errors, fmt.Sprintf("voucher %d re-encrypt failed: %v", vr.ID, encErr))
			continue
		}

		// 3. Verify new ciphertext
		verifyDec, verErr := mgr.DecryptPIN(newCt, newNonce, newVer)
		if verErr != nil || verifyDec != decrypted {
			report.Failed++
			report.Errors = append(report.Errors, fmt.Sprintf("voucher %d verify failed: %v", vr.ID, verErr))
			continue
		}

		// 4. Update row
		_, updateErr := pool.Exec(ctx,
			`UPDATE vouchers_vault
			 SET pin_encrypted = $1, nonce = $2, key_version = $3
			 WHERE id = $4`,
			newCt, newNonce, newVer, vr.ID,
		)
		if updateErr != nil {
			report.Failed++
			report.Errors = append(report.Errors, fmt.Sprintf("voucher %d db update failed: %v", vr.ID, updateErr))
			continue
		}

		report.Reencrypted++
	}

	return report, nil
}
