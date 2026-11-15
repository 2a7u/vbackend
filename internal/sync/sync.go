package sync

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Tombstone struct {
	ID        int64     `json:"id"`
	TableName string    `json:"table_name"`
	RecordID  string    `json:"record_id"`
	DeletedAt time.Time `json:"deleted_at"`
}

type SyncDelta struct {
	Cursor     string                   `json:"cursor"`
	ServerTime time.Time                `json:"server_time"`
	Sims       []map[string]interface{} `json:"sims"`
	Sales      []map[string]interface{} `json:"sales"`
	Debts      []map[string]interface{} `json:"debts"`
	Expenses   []map[string]interface{} `json:"expenses"`
	Tombstones []Tombstone              `json:"tombstones"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// ParseCursor converts cursor string into a time.Time.
func ParseCursor(cursorStr string) time.Time {
	if cursorStr == "" || cursorStr == "0" {
		return time.Time{}
	}
	// Try RFC3339Nano
	if t, err := time.Parse(time.RFC3339Nano, cursorStr); err == nil {
		return t.UTC()
	}
	// Try RFC3339
	if t, err := time.Parse(time.RFC3339, cursorStr); err == nil {
		return t.UTC()
	}
	// Try Unix Milliseconds
	if ms, err := strconv.ParseInt(cursorStr, 10, 64); err == nil {
		if ms > 1e11 { // ms
			return time.UnixMilli(ms).UTC()
		}
		return time.Unix(ms, 0).UTC()
	}
	return time.Time{}
}

// RecordTombstone records a deleted record for offline delta replication.
func (s *Service) RecordTombstone(ctx context.Context, merchantID int64, tableName, recordID string) error {
	if s.pool == nil {
		return nil
	}
	query := `
		INSERT INTO sync_tombstones (merchant_id, table_name, record_id, deleted_at)
		VALUES ($1, $2, $3, now());
	`
	_, err := s.pool.Exec(ctx, query, merchantID, tableName, recordID)
	return err
}

// GetDelta returns entities updated/deleted since the specified cursor for the merchant.
func (s *Service) GetDelta(ctx context.Context, merchantID int64, cursorStr string) (*SyncDelta, error) {
	since := ParseCursor(cursorStr)
	now := time.Now().UTC()
	newCursor := now.Format(time.RFC3339Nano)

	delta := &SyncDelta{
		Cursor:     newCursor,
		ServerTime: now,
		Sims:       make([]map[string]interface{}, 0),
		Sales:      make([]map[string]interface{}, 0),
		Debts:      make([]map[string]interface{}, 0),
		Expenses:   make([]map[string]interface{}, 0),
		Tombstones: make([]Tombstone, 0),
	}

	if s.pool == nil {
		return delta, nil
	}

	// 1. Sims
	simsQuery := `
		SELECT id, barcode, phone_number, serial_number, operator, sim_type, buy_price, sell_price, is_sold, created_at, updated_at
		FROM sims
		WHERE user_id = $1 AND updated_at > $2
		ORDER BY updated_at ASC
		LIMIT 500;
	`
	simRows, err := s.pool.Query(ctx, simsQuery, merchantID, since)
	if err == nil {
		defer simRows.Close()
		for simRows.Next() {
			var id int64
			var barcode, phone, serial, operator, simType string
			var buyPrice, sellPrice float64
			var isSold bool
			var createdAt, updatedAt time.Time
			if err := simRows.Scan(&id, &barcode, &phone, &serial, &operator, &simType, &buyPrice, &sellPrice, &isSold, &createdAt, &updatedAt); err == nil {
				delta.Sims = append(delta.Sims, map[string]interface{}{
					"id":            id,
					"barcode":       barcode,
					"phone_number":  phone,
					"serial_number": serial,
					"operator":      operator,
					"sim_type":      simType,
					"buy_price":     buyPrice,
					"sell_price":    sellPrice,
					"is_sold":       isSold,
					"created_at":    createdAt,
					"updated_at":    updatedAt,
				})
			}
		}
		simRows.Close()
	}

	// 2. Sales
	salesQuery := `
		SELECT id, sim_id, barcode, phone_number, operator, sale_price, cost_price, profit, customer_name, customer_phone, is_credit, created_at, updated_at
		FROM sales
		WHERE user_id = $1 AND updated_at > $2
		ORDER BY updated_at ASC
		LIMIT 500;
	`
	saleRows, err := s.pool.Query(ctx, salesQuery, merchantID, since)
	if err == nil {
		defer saleRows.Close()
		for saleRows.Next() {
			var id, simID int64
			var barcode, phone, operator, custName, custPhone string
			var salePrice, costPrice, profit float64
			var isCredit bool
			var createdAt, updatedAt time.Time
			if err := saleRows.Scan(&id, &simID, &barcode, &phone, &operator, &salePrice, &costPrice, &profit, &custName, &custPhone, &isCredit, &createdAt, &updatedAt); err == nil {
				delta.Sales = append(delta.Sales, map[string]interface{}{
					"id":             id,
					"sim_id":         simID,
					"barcode":        barcode,
					"phone_number":   phone,
					"operator":       operator,
					"sale_price":     salePrice,
					"cost_price":     costPrice,
					"profit":         profit,
					"customer_name":  custName,
					"customer_phone": custPhone,
					"is_credit":      isCredit,
					"created_at":     createdAt,
					"updated_at":     updatedAt,
				})
			}
		}
		saleRows.Close()
	}

	// 3. Debts
	debtsQuery := `
		SELECT id, customer_name, customer_phone, total_amount, paid_amount, remaining_amount, is_paid, due_date, created_at, updated_at
		FROM debts
		WHERE user_id = $1 AND updated_at > $2
		ORDER BY updated_at ASC
		LIMIT 500;
	`
	debtRows, err := s.pool.Query(ctx, debtsQuery, merchantID, since)
	if err == nil {
		defer debtRows.Close()
		for debtRows.Next() {
			var id int64
			var custName, custPhone string
			var total, paid, remaining float64
			var isPaid bool
			var dueDate *time.Time
			var createdAt, updatedAt time.Time
			if err := debtRows.Scan(&id, &custName, &custPhone, &total, &paid, &remaining, &isPaid, &dueDate, &createdAt, &updatedAt); err == nil {
				delta.Debts = append(delta.Debts, map[string]interface{}{
					"id":               id,
					"customer_name":    custName,
					"customer_phone":   custPhone,
					"total_amount":     total,
					"paid_amount":      paid,
					"remaining_amount": remaining,
					"is_paid":          isPaid,
					"due_date":         dueDate,
					"created_at":       createdAt,
					"updated_at":       updatedAt,
				})
			}
		}
		debtRows.Close()
	}

	// 4. Expenses
	expensesQuery := `
		SELECT id, category, amount, description, created_at, updated_at
		FROM expenses
		WHERE user_id = $1 AND updated_at > $2
		ORDER BY updated_at ASC
		LIMIT 500;
	`
	expenseRows, err := s.pool.Query(ctx, expensesQuery, merchantID, since)
	if err == nil {
		defer expenseRows.Close()
		for expenseRows.Next() {
			var id int64
			var category, desc string
			var amount float64
			var createdAt, updatedAt time.Time
			if err := expenseRows.Scan(&id, &category, &amount, &desc, &createdAt, &updatedAt); err == nil {
				delta.Expenses = append(delta.Expenses, map[string]interface{}{
					"id":          id,
					"category":    category,
					"amount":      amount,
					"description": desc,
					"created_at":  createdAt,
					"updated_at":  updatedAt,
				})
			}
		}
		expenseRows.Close()
	}

	// 5. Tombstones
	tombQuery := `
		SELECT id, table_name, record_id, deleted_at
		FROM sync_tombstones
		WHERE merchant_id = $1 AND deleted_at > $2
		ORDER BY deleted_at ASC
		LIMIT 500;
	`
	tombRows, err := s.pool.Query(ctx, tombQuery, merchantID, since)
	if err == nil {
		defer tombRows.Close()
		for tombRows.Next() {
			var t Tombstone
			if err := tombRows.Scan(&t.ID, &t.TableName, &t.RecordID, &t.DeletedAt); err == nil {
				delta.Tombstones = append(delta.Tombstones, t)
			}
		}
		tombRows.Close()
	}

	return delta, nil
}
