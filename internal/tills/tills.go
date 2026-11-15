package tills

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrShiftAlreadyOpen = errors.New("cashier already has an active open shift")
	ErrShiftNotFound    = errors.New("active shift not found")
	ErrDeviceMismatch   = errors.New("device binding mismatch: cashier locked to a different device")
)

type CashierPermissions struct {
	Role            string `json:"role"`
	CanViewCosts    bool   `json:"can_view_costs"`
	CanViewProfits  bool   `json:"can_view_profits"`
	CanCloseShift   bool   `json:"can_close_shift"`
	CanApplyDiscount bool  `json:"can_apply_discount"`
}

type Shift struct {
	ID           int64      `json:"id"`
	MerchantID   int64      `json:"merchant_id"`
	CashierID    int64      `json:"cashier_id"`
	CashierName  string     `json:"cashier_name,omitempty"`
	DeviceID     string     `json:"device_id"`
	OpenedAt     time.Time  `json:"opened_at"`
	ClosedAt     *time.Time `json:"closed_at,omitempty"`
	StartingCash int64      `json:"starting_cash"`
	ExpectedCash int64      `json:"expected_cash"`
	ActualCash   int64      `json:"actual_cash"`
	Discrepancy  int64      `json:"discrepancy"`
	Status       string     `json:"status"` // open, closed
	Note         string     `json:"note"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// GetPermissions returns role-based viewing permissions (masks cost & profit for cashiers).
func GetPermissions(role string) CashierPermissions {
	switch role {
	case "admin", "merchant_owner":
		return CashierPermissions{
			Role:            role,
			CanViewCosts:    true,
			CanViewProfits:  true,
			CanCloseShift:   true,
			CanApplyDiscount: true,
		}
	default: // "cashier", "subscriber_staff"
		return CashierPermissions{
			Role:            role,
			CanViewCosts:    false,
			CanViewProfits:  false,
			CanCloseShift:   true,
			CanApplyDiscount: false,
		}
	}
}

// OpenShift starts a new till shift with device binding verification.
func (s *Service) OpenShift(ctx context.Context, merchantID, cashierID int64, deviceID string, startingCash int64) (*Shift, error) {
	if s.pool == nil {
		return &Shift{
			MerchantID:   merchantID,
			CashierID:    cashierID,
			DeviceID:     deviceID,
			OpenedAt:     time.Now().UTC(),
			StartingCash: startingCash,
			Status:       "open",
		}, nil
	}

	// 1. Verify device lock if user is locked to a specific device
	var boundDevice string
	var isLocked bool
	err := s.pool.QueryRow(ctx, "SELECT COALESCE(device_id, ''), COALESCE(is_device_locked, false) FROM users WHERE id = $1;", cashierID).Scan(&boundDevice, &isLocked)
	if err == nil && isLocked && boundDevice != "" && deviceID != "" && boundDevice != deviceID {
		return nil, ErrDeviceMismatch
	}

	// 2. Check for existing open shift
	var existingID int64
	err = s.pool.QueryRow(ctx, "SELECT id FROM tills_shifts WHERE cashier_id = $1 AND status = 'open' LIMIT 1;", cashierID).Scan(&existingID)
	if err == nil && existingID > 0 {
		return nil, ErrShiftAlreadyOpen
	}

	query := `
		INSERT INTO tills_shifts (merchant_id, cashier_id, device_id, opened_at, starting_cash, status)
		VALUES ($1, $2, $3, now(), $4, 'open')
		RETURNING id, opened_at;
	`
	var id int64
	var openedAt time.Time
	err = s.pool.QueryRow(ctx, query, merchantID, cashierID, deviceID, startingCash).Scan(&id, &openedAt)
	if err != nil {
		return nil, fmt.Errorf("open till shift: %w", err)
	}

	return &Shift{
		ID:           id,
		MerchantID:   merchantID,
		CashierID:    cashierID,
		DeviceID:     deviceID,
		OpenedAt:     openedAt,
		StartingCash: startingCash,
		Status:       "open",
	}, nil
}

// CloseShift reconciles and closes the active shift.
func (s *Service) CloseShift(ctx context.Context, shiftID int64, actualCash int64, note string) (*Shift, error) {
	if s.pool == nil {
		now := time.Now().UTC()
		return &Shift{
			ID:         shiftID,
			ClosedAt:   &now,
			ActualCash: actualCash,
			Status:     "closed",
			Note:       note,
		}, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var (
		merchantID, cashierID int64
		startingCash          int64
		openedAt              time.Time
		status                string
	)
	query := `
		SELECT merchant_id, cashier_id, starting_cash, opened_at, status
		FROM tills_shifts
		WHERE id = $1
		FOR UPDATE;
	`
	err = tx.QueryRow(ctx, query, shiftID).Scan(&merchantID, &cashierID, &startingCash, &openedAt, &status)
	if err == pgx.ErrNoRows {
		return nil, ErrShiftNotFound
	} else if err != nil {
		return nil, err
	}

	if status == "closed" {
		return nil, errors.New("shift is already closed")
	}

	// Calculate cash sales during this shift
	var totalSales float64
	_ = tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(sale_price), 0)
		FROM sales
		WHERE user_id = $1 AND created_at >= $2 AND is_credit = FALSE;
	`, cashierID, openedAt).Scan(&totalSales)

	expectedCash := startingCash + int64(totalSales)
	discrepancy := actualCash - expectedCash
	closedAt := time.Now().UTC()

	updateQuery := `
		UPDATE tills_shifts
		SET closed_at = $1, expected_cash = $2, actual_cash = $3, discrepancy = $4, status = 'closed', note = $5
		WHERE id = $6;
	`
	_, err = tx.Exec(ctx, updateQuery, closedAt, expectedCash, actualCash, discrepancy, note, shiftID)
	if err != nil {
		return nil, fmt.Errorf("update shift close: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &Shift{
		ID:           shiftID,
		MerchantID:   merchantID,
		CashierID:    cashierID,
		OpenedAt:     openedAt,
		ClosedAt:     &closedAt,
		StartingCash: startingCash,
		ExpectedCash: expectedCash,
		ActualCash:   actualCash,
		Discrepancy:  discrepancy,
		Status:       "closed",
		Note:         note,
	}, nil
}

// GetActiveShift returns the cashier's currently open shift.
func (s *Service) GetActiveShift(ctx context.Context, cashierID int64) (*Shift, error) {
	if s.pool == nil {
		return nil, nil
	}
	query := `
		SELECT id, merchant_id, cashier_id, device_id, opened_at, starting_cash, status
		FROM tills_shifts
		WHERE cashier_id = $1 AND status = 'open'
		ORDER BY id DESC LIMIT 1;
	`
	var sh Shift
	err := s.pool.QueryRow(ctx, query, cashierID).Scan(&sh.ID, &sh.MerchantID, &sh.CashierID, &sh.DeviceID, &sh.OpenedAt, &sh.StartingCash, &sh.Status)
	if err == pgx.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return &sh, nil
}
