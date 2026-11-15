package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"venus_backend/internal/vault"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "Simulate migration without modifying data")
	verifySample := flag.Int("verify-sample", 100, "Number of encrypted rows to decrypt and compare before wipe (default 100)")
	dbURLFlag := flag.String("db-url", "", "PostgreSQL connection string (defaults to DATABASE_URL env)")
	flag.Parse()

	dbURL := *dbURLFlag
	if strings.TrimSpace(dbURL) == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if strings.TrimSpace(dbURL) == "" {
		dbURL = os.Getenv("TEST_DATABASE_URL")
	}
	if strings.TrimSpace(dbURL) == "" {
		log.Fatalf("FATAL: DATABASE_URL is not set. Provide --db-url or set DATABASE_URL environment variable.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("FATAL: Failed to connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("FATAL: Database ping failed: %v", err)
	}

	// Initialize and verify vault manager keys
	vaultMgr := vault.Default()
	if vaultMgr.CurrentVersion() <= 0 {
		log.Fatalf("FATAL: Vault encryption keys are uninitialized or invalid.")
	}

	fmt.Println("=================================================================")
	fmt.Println("  VENUS FINTECH VAULT: PRODUCTION PIN MIGRATION TOOL")
	fmt.Printf("  Mode:             %s\n", map[bool]string{true: "DRY-RUN (SIMULATION ONLY - NO DATA WRITES)", false: "LIVE PRODUCTION MIGRATION"}[*dryRun])
	fmt.Printf("  Vault Key Version:%d\n", vaultMgr.CurrentVersion())
	fmt.Printf("  Timestamp (UTC):  %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Println("=================================================================")

	// Step 1: Pre-migration telemetry and counts
	var totalVouchers, alreadyMigrated, unmigratedPlaintext, blankRows int64

	_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM vouchers_vault").Scan(&totalVouchers)
	_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM vouchers_vault WHERE pin_encrypted <> '' AND pin_hmac <> ''").Scan(&alreadyMigrated)
	_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM vouchers_vault WHERE (pin_encrypted = '' OR pin_encrypted IS NULL) AND pin <> ''").Scan(&unmigratedPlaintext)
	_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM vouchers_vault WHERE (pin_encrypted = '' OR pin_encrypted IS NULL) AND (pin = '' OR pin IS NULL)").Scan(&blankRows)

	fmt.Println("\n[STEP 1/4] PRE-MIGRATION INVENTORY AUDIT:")
	fmt.Printf("  * Total vouchers in vault:          %d\n", totalVouchers)
	fmt.Printf("  * Already encrypted & hashed:       %d\n", alreadyMigrated)
	fmt.Printf("  * Legacy unencrypted plaintext rows:%d\n", unmigratedPlaintext)
	fmt.Printf("  * Empty/blank rows:                 %d\n", blankRows)

	if unmigratedPlaintext == 0 {
		fmt.Println("\n[INFO] 0 unmigrated plaintext vouchers found. Database is already fully encrypted.")
		fmt.Println("Ready for Migration 000004 (Verification Gate) and Migration 000005 (Constraints).")
		return
	}

	// Step 2: Execute Migration Worker
	fmt.Println("\n[STEP 2/4] EXECUTING ENCRYPTION & FINGERPRINT WORKER...")
	startTime := time.Now()
	report, err := vault.MigrateLegacyPlaintextPINs(ctx, pool, vaultMgr, *dryRun)
	duration := time.Since(startTime)
	if err != nil {
		log.Fatalf("\n[ERROR] Migration worker encountered critical failure: %v", err)
	}

	fmt.Println("\n[STEP 3/4] MIGRATION EXECUTION REPORT:")
	fmt.Printf("  * Legacy rows processed:            %d\n", report.Processed)
	fmt.Printf("  * Successfully encrypted & verified:%d\n", report.EncryptedAndVerified)
	fmt.Printf("  * Duplicates detected & quarantined:%d\n", report.DuplicatesFound)
	fmt.Printf("  * Elapsed duration:                 %s\n", duration)

	if len(report.Errors) > 0 {
		fmt.Printf("\n[WARNING] %d non-fatal row errors logged during migration:\n", len(report.Errors))
		for idx, errMsg := range report.Errors {
			if idx >= 10 {
				fmt.Printf("  ... and %d more errors\n", len(report.Errors)-10)
				break
			}
			fmt.Printf("  - %s\n", errMsg)
		}
	}

	// Step 4: Verification Gate Check & 100-Row Decrypt-and-Compare
	fmt.Println("\n[STEP 4/4] POST-MIGRATION VERIFICATION GATE & SAMPLE DECRYPT CHECK:")
	var postUnencrypted, postMissingHMAC, postDuplicates int64
	_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM vouchers_vault WHERE pin_encrypted = '' OR pin_encrypted IS NULL").Scan(&postUnencrypted)
	_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM vouchers_vault WHERE pin_hmac = '' OR pin_hmac IS NULL").Scan(&postMissingHMAC)
	_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM legacy_migration_duplicates").Scan(&postDuplicates)

	fmt.Printf("  * Post-run unencrypted rows:        %d\n", postUnencrypted)
	fmt.Printf("  * Post-run missing HMAC rows:       %d\n", postMissingHMAC)
	fmt.Printf("  * Quarantined duplicate entries:    %d\n", postDuplicates)

	if *dryRun {
		fmt.Println("\n>>> DRY RUN COMPLETED SUCCESSFULLY <<<")
		fmt.Println("No records were modified in PostgreSQL.")
		fmt.Println("To apply changes to production, run without --dry-run:")
		fmt.Println("  go run ./cmd/migrate-vault")
		return
	}

	if postUnencrypted > 0 || postMissingHMAC > 0 {
		fmt.Println("\n>>> VERIFICATION GATE FAILED: DO NOT PROCEED TO MIGRATION 000004 <<<")
		fmt.Println("Resolve remaining unencrypted or missing HMAC rows before applying migration 000004.")
		os.Exit(1)
	}

	// Run Decrypt-and-Compare sample verification
	if *verifySample > 0 {
		fmt.Printf("\n[SAMPLE CHECK] Running AES-256-GCM decrypt-and-compare on sample of %d rows...\n", *verifySample)
		verifiedCount, verifyErr := vault.VerifySampleDecryption(ctx, pool, vaultMgr, *verifySample)
		if verifyErr != nil {
			fmt.Printf(">>> SAMPLE DECRYPT-AND-COMPARE FAILED: %v <<<\n", verifyErr)
			os.Exit(1)
		}
		fmt.Printf("  * Successfully decrypted and verified HMAC on %d sample rows (100%% match)\n", verifiedCount)
	}

	fmt.Println("\n>>> VERIFICATION GATE & SAMPLE CHECK PASSED: READY FOR MIGRATION 000004 & 000005 <<<")
	fmt.Println("All vouchers have been encrypted and fingerprinted.")
	fmt.Println("Safe to run:")
	fmt.Println("  go run ./cmd/migrate -up")
}
