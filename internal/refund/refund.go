package refund

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"venus_backend/internal/ledger"
)

var (
	ErrInvalidRefundAmount = errors.New("refund amount must be greater than zero")
	ErrAlreadyRefunded     = errors.New("this transaction has already been refunded")
)

type RefundRequest struct {
	MerchantID     int64  `json:"merchant_id"`
	Amount         int64  `json:"amount"` // in Iraqi Dinars (int64)
	RefType        string `json:"ref_type"`
	RefID          string `json:"ref_id"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

type RefundResult struct {
	TxID           string    `json:"tx_id"`
	MerchantID     int64     `json:"merchant_id"`
	Amount         int64     `json:"amount"`
	BalanceAfter   int64     `json:"balance_after"`
	RefType        string    `json:"ref_type"`
	RefID          string    `json:"ref_id"`
	IsReplay       bool      `json:"is_replay"`
	CreatedAt      time.Time `json:"created_at"`
}

type Service struct {
	pool      *pgxpool.Pool
	ledgerSvc *ledger.Service
}

func NewService(pool *pgxpool.Pool, ledgerSvc *ledger.Service) *Service {
	return &Service{
		pool:      pool,
		ledgerSvc: ledgerSvc,
	}
}

// ProcessRefundTx executes a safe, atomic, idempotent refund counter-entry inside an existing pgx.Tx.
func (s *Service) ProcessRefundTx(ctx context.Context, tx pgx.Tx, req RefundRequest) (*RefundResult, error) {
	if req.Amount <= 0 {
		return nil, ErrInvalidRefundAmount
	}
	if req.MerchantID <= 0 {
		return nil, errors.New("invalid merchant_id for refund")
	}

	// 1. Idempotency Check: check if a reversal/refund entry already exists for this ref_id
	var existingTxID string
	var existingBalance int64
	var existingCreatedAt time.Time
	err := tx.QueryRow(ctx, `
		SELECT tx_id, balance_after, created_at
		FROM ledger_entries
		WHERE account_id = $1
		  AND ref_type = 'refund'
		  AND ref_id = $2
		ORDER BY id DESC
		LIMIT 1;
	`, req.MerchantID, req.RefID).Scan(&existingTxID, &existingBalance, &existingCreatedAt)

	if err == nil {
		// Found existing refund: return cached success without repeating balance credit
		return &RefundResult{
			TxID:         existingTxID,
			MerchantID:   req.MerchantID,
			Amount:       req.Amount,
			BalanceAfter: existingBalance,
			RefType:      req.RefType,
			RefID:        req.RefID,
			IsReplay:     true,
			CreatedAt:    existingCreatedAt,
		}, nil
	} else if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("check existing refund: %w", err)
	}

	// 2. Identify System Account to debit (adjustments or settlement)
	adjustmentsID, err := s.ledgerSvc.GetSystemAccountID(ctx, ledger.SystemEmailAdjustments)
	if err != nil {
		return nil, fmt.Errorf("lookup adjustments system account: %w", err)
	}

	// 3. Post balanced double-entry:
	// Credit: Merchant Wallet (+req.Amount)
	// Debit: Adjustments System Account (+req.Amount)
	txUUID := uuid.New()
	idemKey := req.IdempotencyKey
	if idemKey == "" {
		idemKey = fmt.Sprintf("refund_%s_%d", req.RefID, time.Now().UnixNano())
	}

	doubleEntryReq := ledger.DoubleEntryRequest{
		TxID:           txUUID,
		RefType:        "refund",
		RefID:          req.RefID,
		IdempotencyKey: idemKey,
		CreatedAt:      time.Now().UTC(),
		Legs: []ledger.Leg{
			{AccountID: adjustmentsID, Debit: req.Amount, Credit: 0},
			{AccountID: req.MerchantID, Debit: 0, Credit: req.Amount},
		},
	}

	entries, err := s.ledgerSvc.ExecuteDoubleEntry(ctx, tx, doubleEntryReq)
	if err != nil {
		return nil, fmt.Errorf("execute refund double entry: %w", err)
	}

	var newBalance int64
	for _, entry := range entries {
		if entry.AccountID == req.MerchantID {
			newBalance = entry.BalanceAfter
			break
		}
	}

	// 4. Audit Log
	details, _ := json.Marshal(map[string]interface{}{
		"amount":         req.Amount,
		"ref_type":       req.RefType,
		"ref_id":         req.RefID,
		"reason":         req.Reason,
		"tx_id":          txUUID.String(),
		"balance_after":  newBalance,
		"idempotency_key": idemKey,
	})
	auditQuery := `
		INSERT INTO audit_log (user_id, action, target_type, target_id, details, ip_address, created_at)
		VALUES ($1, 'FINANCIAL_REFUND', 'ledger', $2, $3, '127.0.0.1', now());
	`
	_, _ = tx.Exec(ctx, auditQuery, req.MerchantID, txUUID.String(), details)

	return &RefundResult{
		TxID:         txUUID.String(),
		MerchantID:   req.MerchantID,
		Amount:       req.Amount,
		BalanceAfter: newBalance,
		RefType:      req.RefType,
		RefID:        req.RefID,
		IsReplay:     false,
		CreatedAt:    doubleEntryReq.CreatedAt,
	}, nil
}

// ProcessRefund wraps ProcessRefundTx in a dedicated transaction.
func (s *Service) ProcessRefund(ctx context.Context, req RefundRequest) (*RefundResult, error) {
	if req.Amount <= 0 {
		return nil, ErrInvalidRefundAmount
	}
	if s.pool == nil {
		return nil, errors.New("no database connection pool")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin refund tx: %w", err)
	}
	defer tx.Rollback(ctx)

	res, err := s.ProcessRefundTx(ctx, tx, req)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit refund tx: %w", err)
	}

	return res, nil
}
