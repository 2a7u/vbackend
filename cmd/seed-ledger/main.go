package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"venus_backend/internal/db"
	"venus_backend/internal/ledger"
	"venus_backend/internal/reconcile"
)

type WalletInfo struct {
	ID            int64
	Email         string
	Name          string
	Role          string
	WalletBalance int64
}

func main() {
	dryRunFlag := flag.Bool("dry-run", false, "Preview legacy balances and proposed ledger entries without writing")
	applyFlag := flag.Bool("apply", false, "Apply opening ledger balances inside a single atomic transaction")
	flag.Parse()

	if !*dryRunFlag && !*applyFlag {
		fmt.Println("================================================================================")
		fmt.Println(" VENUS FINTECH - Opening Ledger Seeding Tool")
		fmt.Println("================================================================================")
		fmt.Println("Usage:")
		fmt.Println("  go run ./cmd/seed-ledger --dry-run   (Inspect wallets and preview migration)")
		fmt.Println("  go run ./cmd/seed-ledger --apply     (Commit opening ledger entries & verify 0 diff)")
		fmt.Println("================================================================================")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	dbURL := os.Getenv("DATABASE_URL")
	if strings.TrimSpace(dbURL) == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/venus_db?sslmode=disable"
	}

	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer pool.Close()

	// 1. Refuse to run twice check
	var seededVal string
	err = pool.QueryRow(ctx, `SELECT value FROM system_settings WHERE key = 'opening_ledger_seeded'`).Scan(&seededVal)
	if err == nil && seededVal != "" {
		fmt.Printf("\n[REFUSAL] Opening ledger migration was ALREADY applied at %s.\n", seededVal)
		fmt.Println("Refusing to run twice to preserve cryptographic ledger integrity and prevent double-crediting.")
		os.Exit(0)
	}

	var existingOpeningCount int64
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM ledger_entries WHERE ref_type = 'opening_balance'`).Scan(&existingOpeningCount)
	if existingOpeningCount > 0 {
		fmt.Printf("\n[REFUSAL] %d opening balance ledger entries already exist in ledger_entries.\n", existingOpeningCount)
		fmt.Println("Refusing to run twice to preserve cryptographic ledger integrity.")
		os.Exit(0)
	}

	// 2. Query legacy balances
	rows, err := pool.Query(ctx,
		`SELECT id, email, COALESCE(store_name, full_name, ''), role, ROUND(wallet_balance)::BIGINT
		 FROM users
		 WHERE role <> 'system' AND wallet_balance > 0
		 ORDER BY id ASC`,
	)
	if err != nil {
		log.Fatalf("Query legacy wallets failed: %v", err)
	}
	defer rows.Close()

	var wallets []WalletInfo
	var totalLegacyBalance int64
	for rows.Next() {
		var w WalletInfo
		if err := rows.Scan(&w.ID, &w.Email, &w.Name, &w.Role, &w.WalletBalance); err == nil {
			wallets = append(wallets, w)
			totalLegacyBalance += w.WalletBalance
		}
	}
	rows.Close()

	fmt.Println("================================================================================")
	fmt.Printf(" OPENING LEDGER MIGRATION REPORT (%d active wallets with balance > 0)\n", len(wallets))
	fmt.Println("================================================================================")
	fmt.Printf("%-6s | %-24s | %-20s | %-10s | %-15s | %-15s\n", "User ID", "Email", "Name/Store", "Role", "Legacy Bal (IQD)", "Ledger Credit")
	fmt.Println("--------------------------------------------------------------------------------")
	for _, w := range wallets {
		fmt.Printf("%-6d | %-24s | %-20s | %-10s | %-15d | %-15d\n",
			w.ID, truncate(w.Email, 24), truncate(w.Name, 20), w.Role, w.WalletBalance, w.WalletBalance)
	}
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Printf("TOTAL LEGACY LIABILITY TO BE MIGRATED: %d IQD\n", totalLegacyBalance)
	fmt.Println("================================================================================")

	if *dryRunFlag {
		fmt.Println("\n[DRY-RUN COMPLETE] No database modifications were written.")
		fmt.Println("Run with `--apply` to commit these opening entries to the Double-Entry Ledger.")
		return
	}

	// 3. Apply migration inside a single atomic transaction
	fmt.Println("\nApplying opening ledger migration...")
	ledgerSvc := ledger.NewService(pool, nil)
	reconcileSvc := reconcile.NewService(pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatalf("Begin transaction failed: %v", err)
	}
	defer tx.Rollback(ctx)

	settlementID, err := ledgerSvc.GetSystemAccountID(ctx, ledger.SystemEmailSettlement)
	if err != nil {
		log.Fatalf("Lookup settlement account failed: %v", err)
	}

	var migratedCount int
	for _, w := range wallets {
		req := ledger.DoubleEntryRequest{
			Legs: []ledger.Leg{
				{AccountID: settlementID, Debit: w.WalletBalance, Credit: 0},
				{AccountID: w.ID, Debit: 0, Credit: w.WalletBalance},
			},
			RefType: "opening_balance",
			RefID:   fmt.Sprintf("legacy_uid_%d", w.ID),
		}
		_, err := ledgerSvc.ExecuteDoubleEntry(ctx, tx, req)
		if err != nil {
			log.Fatalf("ExecuteDoubleEntry failed for user %d: %v", w.ID, err)
		}
		migratedCount++
	}

	// Record migration marker
	_, err = tx.Exec(ctx,
		`INSERT INTO system_settings (key, value)
		 VALUES ('opening_ledger_seeded', $1)
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		log.Fatalf("Save opening_ledger_seeded setting failed: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("Commit transaction failed: %v", err)
	}

	fmt.Printf("Successfully committed %d opening ledger transactions.\n", migratedCount)

	// 4. Run immediate automated reconciliation to verify difference == 0
	fmt.Println("Running post-migration reconciliation audit...")
	rep, err := reconcileSvc.RunDailyReconciliation(ctx, time.Now().UTC(), "cli_seed_ledger")
	if err != nil {
		log.Fatalf("Reconciliation check error: %v", err)
	}

	fmt.Println("================================================================================")
	fmt.Printf("RECONCILIATION AUDIT: Report Date %s\n", rep.ReportDate)
	fmt.Printf("Total System Balances : %d IQD\n", rep.TotalSystemBalance)
	fmt.Printf("Total Ledger Credits  : %d IQD\n", rep.TotalLedgerCredits)
	fmt.Printf("Total Ledger Debits   : %d IQD\n", rep.TotalLedgerDebits)
	fmt.Printf("Reconciliation Diff   : %d IQD\n", rep.Discrepancy)
	fmt.Printf("Audit Status          : %s\n", rep.Status)
	fmt.Println("================================================================================")

	if rep.Discrepancy != 0 {
		log.Fatalf("FATAL AUDIT FAILURE: Reconciliation discrepancy is non-zero (%d IQD). Expected exact 0 diff!", rep.Discrepancy)
	}

	fmt.Println("SUCCESS: Opening ledger migrated with 0 IQD discrepancy. Books are perfectly balanced!")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
