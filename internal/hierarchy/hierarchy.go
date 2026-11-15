package hierarchy

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"venus_backend/internal/ledger"
)

var (
	ErrCyclicHierarchy = errors.New("hierarchy: cyclical parent-agent assignment detected")
	ErrSelfParenting   = errors.New("hierarchy: an agent cannot be their own parent")
	ErrMaxDepthExceeded = errors.New("hierarchy: agent hierarchy exceeds maximum permitted depth")
)

const MaxHierarchyDepth = 25

type AgentCommissionRule struct {
	ID              int64     `json:"id"`
	AgentID         int64     `json:"agent_id"`
	ProductID       string    `json:"product_id"`
	CommissionType  string    `json:"commission_type"` // "fixed" or "percentage"
	CommissionValue float64   `json:"commission_value"`
	CreatedAt       time.Time `json:"created_at"`
}

type CommissionResult struct {
	ParentAgentID    int64   `json:"parent_agent_id"`
	CommissionAmount int64   `json:"commission_amount"` // IQD
	CommissionType   string  `json:"commission_type"`
	CommissionValue  float64 `json:"commission_value"`
	ProductID        string  `json:"product_id"`
	SaleAmount       int64   `json:"sale_amount"`
}

type Service struct {
	Pool      *pgxpool.Pool
	LedgerSvc *ledger.Service
}

func NewService(pool *pgxpool.Pool, ledgerSvc *ledger.Service) *Service {
	return &Service{
		Pool:      pool,
		LedgerSvc: ledgerSvc,
	}
}

// CheckCyclePreventsInfiniteLoop verifies that assigning proposedParentID to childID
// does not create a cycle in the agent hierarchy.
func (s *Service) CheckCyclePreventsInfiniteLoop(ctx context.Context, childID, proposedParentID int64) error {
	if childID == proposedParentID {
		return ErrSelfParenting
	}
	if proposedParentID == 0 {
		return nil
	}

	current := proposedParentID
	visited := make(map[int64]bool)
	visited[childID] = true

	for depth := 0; depth < MaxHierarchyDepth; depth++ {
		if current == childID {
			return ErrCyclicHierarchy
		}
		if visited[current] {
			return ErrCyclicHierarchy
		}
		visited[current] = true

		var nextParent *int64
		err := s.Pool.QueryRow(ctx,
			`SELECT parent_agent_id FROM users WHERE id = $1`, current,
		).Scan(&nextParent)
		if err != nil || nextParent == nil || *nextParent == 0 {
			// Reached top of hierarchy safely
			return nil
		}
		current = *nextParent
	}

	return ErrMaxDepthExceeded
}

// CalculateCommissionAmount calculates the monetary commission in Iraqi Dinar.
func CalculateCommissionAmount(commType string, commValue float64, saleAmount int64) int64 {
	if commValue <= 0 || saleAmount <= 0 {
		return 0
	}

	if commType == "percentage" {
		amt := (float64(saleAmount) * commValue) / 100.0
		return int64(math.Round(amt))
	}

	// Default: fixed amount in IQD
	return int64(math.Round(commValue))
}

// ProcessSaleCommission checks for a parent agent and records the commission in the same DB transaction.
func (s *Service) ProcessSaleCommission(ctx context.Context, tx pgx.Tx, subPointID int64, productID string, saleAmount int64, refID string) (*CommissionResult, error) {
	if s.Pool == nil || subPointID <= 0 || saleAmount <= 0 {
		return nil, nil
	}

	// 1. Look up parent agent
	var parentAgentID *int64
	err := tx.QueryRow(ctx,
		`SELECT parent_agent_id FROM users WHERE id = $1`, subPointID,
	).Scan(&parentAgentID)
	if err != nil || parentAgentID == nil || *parentAgentID == 0 {
		return nil, nil // Independent subscriber / top-level agent
	}

	// 2. Fetch specific or wildcard commission rule for this agent
	var commType string
	var commValue float64
	err = tx.QueryRow(ctx,
		`SELECT commission_type, commission_value
		 FROM agent_commissions
		 WHERE agent_id = $1 AND (product_id = $2 OR product_id = '*')
		 ORDER BY CASE WHEN product_id = '*' THEN 2 ELSE 1 END
		 LIMIT 1`,
		*parentAgentID, productID,
	).Scan(&commType, &commValue)
	if err != nil {
		// Default fallback if no custom rule: 1% commission
		commType = "percentage"
		commValue = 1.0
	}

	commAmount := CalculateCommissionAmount(commType, commValue, saleAmount)
	if commAmount <= 0 {
		return nil, nil
	}

	// 3. Record commission via Double-Entry Ledger (Debit: Settlement, Credit: Parent Agent)
	if s.LedgerSvc != nil {
		settlementID, sErr := s.LedgerSvc.GetSystemAccountID(ctx, ledger.SystemEmailSettlement)
		if sErr == nil {
			req := ledger.DoubleEntryRequest{
				Legs: []ledger.Leg{
					{AccountID: settlementID, Debit: commAmount, Credit: 0},
					{AccountID: *parentAgentID, Debit: 0, Credit: commAmount},
				},
				RefType: "agent_commission",
				RefID:   fmt.Sprintf("sub_%d_sale_%s", subPointID, refID),
			}
			_, lErr := s.LedgerSvc.ExecuteDoubleEntry(ctx, tx, req)
			if lErr != nil {
				return nil, fmt.Errorf("record commission ledger legs: %w", lErr)
			}
		}
	} else {
		// Fallback direct balance update if ledger service not injected
		_, err = tx.Exec(ctx,
			`UPDATE users
			 SET wallet_balance = wallet_balance + $1
			 WHERE id = $2`,
			commAmount, *parentAgentID,
		)
		if err != nil {
			return nil, fmt.Errorf("credit parent agent wallet: %w", err)
		}
	}

	return &CommissionResult{
		ParentAgentID:    *parentAgentID,
		CommissionAmount: commAmount,
		CommissionType:   commType,
		CommissionValue:  commValue,
		ProductID:        productID,
		SaleAmount:       saleAmount,
	}, nil
}

// ReverseCommission reverses a previously awarded commission on order refund in the same transaction.
func (s *Service) ReverseCommission(ctx context.Context, tx pgx.Tx, parentAgentID int64, commissionAmount int64, refID string) error {
	if parentAgentID <= 0 || commissionAmount <= 0 {
		return nil
	}

	// 1. Reverse Double-Entry Ledger legs (Debit: Parent Agent, Credit: Settlement)
	if s.LedgerSvc != nil {
		settlementID, sErr := s.LedgerSvc.GetSystemAccountID(ctx, ledger.SystemEmailSettlement)
		if sErr == nil {
			req := ledger.DoubleEntryRequest{
				Legs: []ledger.Leg{
					{AccountID: parentAgentID, Debit: commissionAmount, Credit: 0},
					{AccountID: settlementID, Debit: 0, Credit: commissionAmount},
				},
				RefType: "agent_commission_reversal",
				RefID:   refID,
			}
			_, lErr := s.LedgerSvc.ExecuteDoubleEntry(ctx, tx, req)
			if lErr != nil {
				return fmt.Errorf("record commission reversal ledger legs: %w", lErr)
			}
			return nil
		}
	}

	_, err := tx.Exec(ctx,
		`UPDATE users
		 SET wallet_balance = wallet_balance - $1
		 WHERE id = $2`,
		commissionAmount, parentAgentID,
	)
	if err != nil {
		return fmt.Errorf("debit parent agent wallet on refund: %w", err)
	}

	return nil
}
