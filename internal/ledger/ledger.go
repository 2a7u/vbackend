package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"
	CurrencyIQD = "IQD"

	SystemEmailSettlement  = "settlement@venus.internal"
	SystemEmailRevenue     = "revenue@venus.internal"
	SystemEmailVouchers    = "vouchers@venus.internal"
	SystemEmailAdjustments = "adjustments@venus.internal"
)

var (
	ErrUnbalancedTransaction = errors.New("unbalanced double entry: total debit must equal total credit and be > 0")
	ErrInsufficientFunds     = errors.New("الرصيد غير كافٍ وسقف الائتمان لا يسمح بإتمام العملية")
	ErrWalletFrozen          = errors.New("المحفظة مجمدة لأسباب أمنية، يرجى مراجعة إدارة النظام")
	ErrAccountNotFound       = errors.New("الحساب المالي غير موجود")
	ErrInvalidAmount         = errors.New("المبلغ المالي غير صالح")
)

type Broadcaster interface {
	Broadcast(userID int64, event string, payload any)
}

type Leg struct {
	AccountID int64
	Debit     int64
	Credit    int64
}

type DoubleEntryRequest struct {
	TxID           uuid.UUID
	RefType        string
	RefID          string
	IdempotencyKey string
	Legs           []Leg
	CreatedAt      time.Time
}

type LedgerEntry struct {
	ID             int64     `json:"id"`
	TxID           string    `json:"tx_id"`
	AccountID      int64     `json:"account_id"`
	Debit          int64     `json:"debit"`
	Credit         int64     `json:"credit"`
	BalanceAfter   int64     `json:"balance_after"`
	Currency       string    `json:"currency"`
	RefType        string    `json:"ref_type"`
	RefID          string    `json:"ref_id"`
	IdempotencyKey *string   `json:"idempotency_key,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	PrevHash       string    `json:"prev_hash"`
	Hash           string    `json:"hash"`
	AccountEmail   string    `json:"account_email,omitempty"`
	AccountName    string    `json:"account_name,omitempty"`
}

type Service struct {
	Pool *pgxpool.Pool
	Hub  Broadcaster

	mu              sync.RWMutex
	systemAccountID map[string]int64
}

func NewService(pool *pgxpool.Pool, hub Broadcaster) *Service {
	return &Service{
		Pool:            pool,
		Hub:             hub,
		systemAccountID: make(map[string]int64),
	}
}

// GetSystemAccountID retrieves or caches the user ID for a system email.
func (s *Service) GetSystemAccountID(ctx context.Context, email string) (int64, error) {
	s.mu.RLock()
	id, ok := s.systemAccountID[email]
	s.mu.RUnlock()
	if ok && id > 0 {
		return id, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var foundID int64
	err := s.Pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&foundID)
	if err == pgx.ErrNoRows {
		// Auto-provision if missing
		err = s.Pool.QueryRow(ctx,
			`INSERT INTO users (email, password_hash, full_name, role, is_approved)
			 VALUES ($1, 'SYSTEM_ACCOUNT_LOCKED', $2, 'system', true)
			 ON CONFLICT (email) DO UPDATE SET role = 'system'
			 RETURNING id`,
			email, "حساب نظام - "+email,
		).Scan(&foundID)
	}
	if err != nil {
		return 0, fmt.Errorf("resolve system account %s: %w", email, err)
	}

	s.systemAccountID[email] = foundID
	return foundID, nil
}

// GetCachedSystemAccountID retrieves the account ID from memory if already cached.
func (s *Service) GetCachedSystemAccountID(email string) (int64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.systemAccountID[email]
	return id, ok && id > 0
}

// GetSystemAccountIDTx retrieves or provisions a system account within the provided transaction
// to avoid deadlocking connection pools when all connections are busy.
func (s *Service) GetSystemAccountIDTx(ctx context.Context, tx pgx.Tx, email string) (int64, error) {
	if id, ok := s.GetCachedSystemAccountID(email); ok {
		return id, nil
	}

	var foundID int64
	err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&foundID)
	if err == pgx.ErrNoRows {
		err = tx.QueryRow(ctx,
			`INSERT INTO users (email, password_hash, full_name, role, is_approved)
			 VALUES ($1, 'SYSTEM_ACCOUNT_LOCKED', $2, 'system', true)
			 ON CONFLICT (email) DO UPDATE SET role = 'system'
			 RETURNING id`,
			email, "حساب نظام - "+email,
		).Scan(&foundID)
	}
	if err != nil {
		return 0, fmt.Errorf("resolve system account %s: %w", email, err)
	}

	s.mu.Lock()
	s.systemAccountID[email] = foundID
	s.mu.Unlock()

	return foundID, nil
}

// CalculateEntryHash returns the cryptographic SHA-256 hash for a ledger entry.
func CalculateEntryHash(prevHash string, txID uuid.UUID, accountID, debit, credit, balanceAfter int64, createdAt time.Time) string {
	payload := fmt.Sprintf("%s|%s|%d|%d|%d|%d|%s",
		prevHash,
		txID.String(),
		accountID,
		debit,
		credit,
		balanceAfter,
		createdAt.UTC().Format(time.RFC3339Nano),
	)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// ExecuteDoubleEntry executes a balanced double-entry transaction.
// It locks participating accounts via SELECT ... FOR UPDATE, enforces balance
// and credit limit constraints, chains SHA-256 hashes, and records ledger_entries.
func (s *Service) ExecuteDoubleEntry(ctx context.Context, tx pgx.Tx, req DoubleEntryRequest) ([]LedgerEntry, error) {
	if len(req.Legs) < 2 {
		return nil, errors.New("double-entry requires at least 2 legs")
	}

	var totalDebit, totalCredit int64
	for _, leg := range req.Legs {
		if leg.Debit < 0 || leg.Credit < 0 {
			return nil, ErrInvalidAmount
		}
		if (leg.Debit > 0 && leg.Credit > 0) || (leg.Debit == 0 && leg.Credit == 0) {
			return nil, errors.New("each leg must have strictly either debit > 0 or credit > 0")
		}
		totalDebit += leg.Debit
		totalCredit += leg.Credit
	}
	if totalDebit != totalCredit || totalDebit <= 0 {
		return nil, ErrUnbalancedTransaction
	}

	if req.TxID == uuid.Nil {
		req.TxID = uuid.New()
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now().UTC()
	}

	var entries []LedgerEntry

	for _, leg := range req.Legs {
		var currentBalance float64
		var creditLimit int64
		var walletStatus string
		var role string

		// Strict row-level lock on the account
		err := tx.QueryRow(ctx,
			`SELECT COALESCE(wallet_balance, 0), COALESCE(credit_limit, 0), COALESCE(wallet_status, 'active'), COALESCE(role, 'subscriber')
			 FROM users
			 WHERE id = $1
			 FOR UPDATE`,
			leg.AccountID,
		).Scan(&currentBalance, &creditLimit, &walletStatus, &role)
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("%w: user #%d", ErrAccountNotFound, leg.AccountID)
		}
		if err != nil {
			return nil, fmt.Errorf("lock account #%d: %w", leg.AccountID, err)
		}

		currentBalanceIQD := int64(currentBalance)

		// Debit checks: status and overdraft limit (system clearing accounts are exempt from subscriber limits)
		if leg.Debit > 0 && role != "system" {
			if walletStatus == "frozen" {
				return nil, fmt.Errorf("%w (حساب #%d)", ErrWalletFrozen, leg.AccountID)
			}
			maxSpendable := currentBalanceIQD + creditLimit
			if maxSpendable < leg.Debit {
				return nil, fmt.Errorf("%w: الرصيد المتوفر %d + سقف الائتمان %d لا يكفي لخصم %d (حساب #%d)",
					ErrInsufficientFunds, currentBalanceIQD, creditLimit, leg.Debit, leg.AccountID)
			}
		}

		balanceAfter := currentBalanceIQD + leg.Credit - leg.Debit

		// Get previous hash for hash chain continuity
		var prevHash string
		err = tx.QueryRow(ctx,
			`SELECT hash FROM ledger_entries
			 WHERE account_id = $1
			 ORDER BY id DESC
			 LIMIT 1`,
			leg.AccountID,
		).Scan(&prevHash)
		if err == pgx.ErrNoRows {
			prevHash = GenesisHash
		} else if err != nil {
			return nil, fmt.Errorf("fetch prev hash for account #%d: %w", leg.AccountID, err)
		}

		entryHash := CalculateEntryHash(prevHash, req.TxID, leg.AccountID, leg.Debit, leg.Credit, balanceAfter, req.CreatedAt)

		var idKey *string
		if req.IdempotencyKey != "" {
			idKey = &req.IdempotencyKey
		}

		var entryID int64
		err = tx.QueryRow(ctx,
			`INSERT INTO ledger_entries (
				tx_id, account_id, debit, credit, balance_after, currency,
				ref_type, ref_id, idempotency_key, created_at, prev_hash, hash
			 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			 RETURNING id`,
			req.TxID, leg.AccountID, leg.Debit, leg.Credit, balanceAfter, CurrencyIQD,
			req.RefType, req.RefID, idKey, req.CreatedAt, prevHash, entryHash,
		).Scan(&entryID)
		if err != nil {
			return nil, fmt.Errorf("insert ledger entry for account #%d: %w", leg.AccountID, err)
		}

		// Update users.wallet_balance to match ledger balance_after
		_, err = tx.Exec(ctx,
			`UPDATE users
			 SET wallet_balance = $1, updated_at = now()
			 WHERE id = $2`,
			balanceAfter, leg.AccountID,
		)
		if err != nil {
			return nil, fmt.Errorf("update wallet_balance for account #%d: %w", leg.AccountID, err)
		}

		entries = append(entries, LedgerEntry{
			ID:             entryID,
			TxID:           req.TxID.String(),
			AccountID:      leg.AccountID,
			Debit:          leg.Debit,
			Credit:         leg.Credit,
			BalanceAfter:   balanceAfter,
			Currency:       CurrencyIQD,
			RefType:        req.RefType,
			RefID:          req.RefID,
			IdempotencyKey: idKey,
			CreatedAt:      req.CreatedAt,
			PrevHash:       prevHash,
			Hash:           entryHash,
		})
	}

	return entries, nil
}

// RecordTopup records a wallet top-up (e.g. ZainCash or manual):
// Debit: System Settlement Account
// Credit: User Account
func (s *Service) RecordTopup(ctx context.Context, tx pgx.Tx, userAccountID int64, amountIQD int64, refType, refID, idempotencyKey string) ([]LedgerEntry, error) {
	if amountIQD <= 0 {
		return nil, ErrInvalidAmount
	}
	settlementID, err := s.GetSystemAccountIDTx(ctx, tx, SystemEmailSettlement)
	if err != nil {
		return nil, err
	}

	return s.ExecuteDoubleEntry(ctx, tx, DoubleEntryRequest{
		TxID:           uuid.New(),
		RefType:        refType,
		RefID:          refID,
		IdempotencyKey: idempotencyKey,
		Legs: []Leg{
			{AccountID: settlementID, Debit: amountIQD, Credit: 0},
			{AccountID: userAccountID, Debit: 0, Credit: amountIQD},
		},
	})
}

// RecordDebit records a user purchase (voucher, subscription, etc.):
// Debit: User Account
// Credit: System Revenue or Vouchers Account
func (s *Service) RecordPurchase(ctx context.Context, tx pgx.Tx, userAccountID int64, systemEmail string, amountIQD int64, refType, refID, idempotencyKey string) ([]LedgerEntry, error) {
	if amountIQD <= 0 {
		return nil, ErrInvalidAmount
	}
	systemID, err := s.GetSystemAccountIDTx(ctx, tx, systemEmail)
	if err != nil {
		return nil, err
	}

	return s.ExecuteDoubleEntry(ctx, tx, DoubleEntryRequest{
		TxID:           uuid.New(),
		RefType:        refType,
		RefID:          refID,
		IdempotencyKey: idempotencyKey,
		Legs: []Leg{
			{AccountID: userAccountID, Debit: amountIQD, Credit: 0},
			{AccountID: systemID, Debit: 0, Credit: amountIQD},
		},
	})
}

// RecordAdminAdjustment records a manual balance correction by an admin:
// If amountIQD > 0: Debit System Adjustments, Credit User Account
// If amountIQD < 0: Debit User Account, Credit System Adjustments
func (s *Service) RecordAdminAdjustment(ctx context.Context, tx pgx.Tx, userAccountID int64, amountIQD int64, adminID int64, note, idempotencyKey string) ([]LedgerEntry, error) {
	if amountIQD == 0 {
		return nil, ErrInvalidAmount
	}
	adjustmentsID, err := s.GetSystemAccountIDTx(ctx, tx, SystemEmailAdjustments)
	if err != nil {
		return nil, err
	}

	var legs []Leg
	if amountIQD > 0 {
		legs = []Leg{
			{AccountID: adjustmentsID, Debit: amountIQD, Credit: 0},
			{AccountID: userAccountID, Debit: 0, Credit: amountIQD},
		}
	} else {
		absAmount := -amountIQD
		legs = []Leg{
			{AccountID: userAccountID, Debit: absAmount, Credit: 0},
			{AccountID: adjustmentsID, Debit: 0, Credit: absAmount},
		}
	}

	refID := fmt.Sprintf("admin_%d_%d", adminID, time.Now().UnixNano())
	if note != "" {
		refID = fmt.Sprintf("admin_%d_%s", adminID, note)
	}

	return s.ExecuteDoubleEntry(ctx, tx, DoubleEntryRequest{
		TxID:           uuid.New(),
		RefType:        "admin_adjustment",
		RefID:          refID,
		IdempotencyKey: idempotencyKey,
		Legs:           legs,
	})
}

// ExecuteReversal reverses an existing financial transaction by tx_id.
// It generates mirrored legs (Debit <-> Credit) ensuring double entry invariance.
func (s *Service) ExecuteReversal(ctx context.Context, tx pgx.Tx, originalTxID uuid.UUID, reason string, operatorID int64) ([]LedgerEntry, error) {
	rows, err := tx.Query(ctx,
		`SELECT account_id, debit, credit
		 FROM ledger_entries
		 WHERE tx_id = $1
		 ORDER BY id ASC`,
		originalTxID,
	)
	if err != nil {
		return nil, fmt.Errorf("read original transaction legs: %w", err)
	}
	defer rows.Close()

	var mirroredLegs []Leg
	for rows.Next() {
		var accountID, debit, credit int64
		if err := rows.Scan(&accountID, &debit, &credit); err != nil {
			return nil, err
		}
		// Invert leg: original debit becomes credit; original credit becomes debit
		mirroredLegs = append(mirroredLegs, Leg{
			AccountID: accountID,
			Debit:     credit,
			Credit:    debit,
		})
	}
	if len(mirroredLegs) < 2 {
		return nil, fmt.Errorf("transaction %s has no reversible legs", originalTxID)
	}

	refID := fmt.Sprintf("reversal_%s_by_%d", originalTxID.String(), operatorID)
	if reason != "" {
		refID += "_" + reason
	}

	return s.ExecuteDoubleEntry(ctx, tx, DoubleEntryRequest{
		TxID:      uuid.New(),
		RefType:   "reversal",
		RefID:     refID,
		Legs:      mirroredLegs,
		CreatedAt: time.Now().UTC(),
	})
}
