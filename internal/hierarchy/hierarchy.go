package hierarchy

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"venus_backend/internal/ledger"
)

var (
	ErrCyclicHierarchy  = errors.New("hierarchy: cyclical parent-agent assignment detected")
	ErrSelfParenting    = errors.New("hierarchy: an agent cannot be their own parent")
	ErrMaxDepthExceeded = errors.New("hierarchy: agent hierarchy exceeds maximum permitted depth")
)

const MaxHierarchyDepth = 25

type AgentCommissionRule struct {
	ID                    int64     `json:"id"`
	AgentID               int64     `json:"agent_id"`
	ProductID             string    `json:"product_id"`
	CommissionType        string    `json:"commission_type"` // "fixed" or "percentage"
	CommissionBasisPoints int       `json:"commission_basis_points"` // 100 = 1.00%, 250 = 2.50%
	FixedAmount           int64     `json:"fixed_amount"`           // in IQD
	CapToMargin           bool      `json:"cap_to_margin"`
	CreatedAt             time.Time `json:"created_at"`
}

type CommissionResult struct {
	ParentAgentID         int64  `json:"parent_agent_id"`
	CommissionAmount      int64  `json:"commission_amount"` // IQD
	CommissionType        string `json:"commission_type"`
	CommissionBasisPoints int    `json:"commission_basis_points,omitempty"`
	FixedAmount           int64  `json:"fixed_amount,omitempty"`
	ProductID             string `json:"product_id"`
	SaleAmount            int64  `json:"sale_amount"`
	CostPrice             int64  `json:"cost_price,omitempty"`
	MarginCapped          bool   `json:"margin_capped"`
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

// maxDepth returns the configured depth limit or defaults to 25.
func (s *Service) maxDepth() int {
	if env := os.Getenv("MAX_HIERARCHY_DEPTH"); env != "" {
		if d, err := strconv.Atoi(env); err == nil && d > 0 {
			return d
		}
	}
	return MaxHierarchyDepth
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

	limit := s.maxDepth()
	for depth := 0; depth < limit; depth++ {
		if current == childID {
			return ErrCyclicHierarchy
		}
		if visited[current] {
			return ErrCyclicHierarchy
		}
		visited[current] = true

		if s.Pool == nil {
			return nil
		}

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

// CalculateCommissionAmount calculates the monetary commission in Iraqi Dinar using exact integer arithmetic.
// Rounding rule: half-up rounding (basis points * saleAmount + 5000) / 10000.
// If capToMargin is true, the commission is strictly capped at max(0, saleAmount - costPrice).
func CalculateCommissionAmount(commType string, basisPoints int, fixedAmount int64, saleAmount int64, costPrice int64, capToMargin bool) (int64, bool) {
	if saleAmount <= 0 {
		return 0, false
	}

	var rawCommission int64
	if commType == "percentage" {
		if basisPoints <= 0 {
			return 0, false
		}
		// Basis points (100 = 1.00%, 10,000 = 100.00%).
		// Half-up rounding without floats: (saleAmount * basisPoints + 5000) / 10000
		rawCommission = (saleAmount*int64(basisPoints) + 5000) / 10000
	} else {
		// "fixed" amount in IQD
		if fixedAmount <= 0 {
			return 0, false
		}
		rawCommission = fixedAmount
	}

	if !capToMargin {
		return rawCommission, false
	}

	margin := saleAmount - costPrice
	if margin < 0 {
		margin = 0
	}

	if rawCommission > margin {
		return margin, true // Capped to sale margin
	}
	return rawCommission, false
}

// ProcessSaleCommission checks for a parent agent and records the commission in the same DB transaction.
func (s *Service) ProcessSaleCommission(ctx context.Context, tx pgx.Tx, subPointID int64, productID string, saleAmount int64, costPrice int64, refID string) (*CommissionResult, error) {
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
	var basisPoints int
	var fixedAmount int64
	var capToMargin bool
	var legacyValue float64

	err = tx.QueryRow(ctx,
		`SELECT commission_type, commission_basis_points, fixed_amount, cap_to_margin, commission_value
		 FROM agent_commissions
		 WHERE agent_id = $1 AND (product_id = $2 OR product_id = '*')
		 ORDER BY CASE WHEN product_id = '*' THEN 2 ELSE 1 END
		 LIMIT 1`,
		*parentAgentID, productID,
	).Scan(&commType, &basisPoints, &fixedAmount, &capToMargin, &legacyValue)
	if err != nil {
		// Default fallback if no custom rule: 1% (100 basis points), cap_to_margin = true
		commType = "percentage"
		basisPoints = 100
		fixedAmount = 0
		capToMargin = true
	} else if basisPoints == 0 && fixedAmount == 0 && legacyValue > 0 {
		// Legacy migration fallback
		if commType == "percentage" {
			basisPoints = int(math.Round(legacyValue * 100))
		} else {
			fixedAmount = int64(math.Round(legacyValue))
		}
	}

	commAmount, wasCapped := CalculateCommissionAmount(commType, basisPoints, fixedAmount, saleAmount, costPrice, capToMargin)
	if commAmount <= 0 {
		return nil, nil
	}

	// 3. Record commission via Double-Entry Ledger (Debit: Settlement, Credit: Parent Agent)
	if s.LedgerSvc != nil {
		settlementID, sErr := s.LedgerSvc.GetSystemAccountIDTx(ctx, tx, ledger.SystemEmailSettlement)
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
		ParentAgentID:         *parentAgentID,
		CommissionAmount:      commAmount,
		CommissionType:        commType,
		CommissionBasisPoints: basisPoints,
		FixedAmount:           fixedAmount,
		ProductID:             productID,
		SaleAmount:            saleAmount,
		CostPrice:             costPrice,
		MarginCapped:          wasCapped,
	}, nil
}

// ReverseCommission reverses a previously awarded commission on order refund in the same transaction.
// It is strictly idempotent: DB-level unique index `idx_ledger_entries_unique_reversal`
// guarantees that multiple concurrent reversal requests cannot double-debit the parent agent.
func (s *Service) ReverseCommission(ctx context.Context, tx pgx.Tx, parentAgentID int64, commissionAmount int64, refID string) error {
	if parentAgentID <= 0 || commissionAmount <= 0 {
		return nil
	}

	// 1. Reverse Double-Entry Ledger legs (Debit: Parent Agent, Credit: Settlement)
	if s.LedgerSvc != nil {
		settlementID, sErr := s.LedgerSvc.GetSystemAccountIDTx(ctx, tx, ledger.SystemEmailSettlement)
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
				// DB-enforced idempotency guarantee: unique violation on idx_ledger_entries_unique_reversal is a safe no-op success
				var pgErr *pgconn.PgError
				if errors.As(lErr, &pgErr) && (pgErr.Code == "23505" || strings.Contains(pgErr.ConstraintName, "unique_reversal")) {
					return nil
				}
				if strings.Contains(lErr.Error(), "idx_ledger_entries_unique_reversal") || strings.Contains(lErr.Error(), "duplicate key") {
					return nil
				}
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
