package risk

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type RiskProfile struct {
	MerchantID       int64     `json:"merchant_id"`
	StoreName        string    `json:"store_name"`
	RiskScore        int       `json:"risk_score"` // 0-100
	VelocityCount    int       `json:"velocity_count"`
	FailedAuthCount  int       `json:"failed_auth_count"`
	SuspiciousFlags  []string  `json:"suspicious_flags"`
	IsAutoFrozen     bool      `json:"is_auto_frozen"`
	WalletStatus     string    `json:"wallet_status"`
	LastEvaluatedAt  time.Time `json:"last_evaluated_at"`
}

type RiskAssessment struct {
	MerchantID      int64    `json:"merchant_id"`
	Score           int      `json:"score"`
	Flags           []string `json:"flags"`
	ActionTaken     string   `json:"action_taken"` // "allow", "alert", "freeze"
	IsFrozen        bool     `json:"is_frozen"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// ComputeRisk evaluates rule factors and returns score + triggered flags.
func ComputeRisk(txHourBaghdad int, velocityCount int, failedAttempts int, isAbnormalAmount bool) (int, []string) {
	score := 0
	var flags []string

	// 1. Off-hours (2:00 AM - 5:00 AM Baghdad time)
	if txHourBaghdad >= 2 && txHourBaghdad <= 5 {
		score += 20
		flags = append(flags, "OFF_HOURS_ACTIVITY (2:00 AM - 5:00 AM)")
	}

	// 2. High velocity (> 15 transactions in 10 minutes)
	if velocityCount >= 15 {
		score += 30
		flags = append(flags, fmt.Sprintf("HIGH_VELOCITY_SPIKE (%d tx / 10m)", velocityCount))
	} else if velocityCount >= 8 {
		score += 15
		flags = append(flags, fmt.Sprintf("MODERATE_VELOCITY (%d tx / 10m)", velocityCount))
	}

	// 3. Failed activations or auth attempts
	if failedAttempts >= 5 {
		score += 30
		flags = append(flags, fmt.Sprintf("BRUTE_FORCE_ATTEMPTS (%d failures)", failedAttempts))
	} else if failedAttempts >= 3 {
		score += 15
		flags = append(flags, fmt.Sprintf("MULTIPLE_FAILURES (%d failures)", failedAttempts))
	}

	// 4. Abnormal amount
	if isAbnormalAmount {
		score += 25
		flags = append(flags, "ABNORMAL_TRANSACTION_SPIKE")
	}

	if score > 100 {
		score = 100
	}
	return score, flags
}

// EvaluateRisk performs live assessment of merchant operation.
func (s *Service) EvaluateRisk(ctx context.Context, merchantID int64, amount int64) (*RiskAssessment, error) {
	loc := time.FixedZone("Asia/Baghdad", 3*60*60)
	now := time.Now().In(loc)
	txHour := now.Hour()

	velocity := 0
	failures := 0
	isAbnormal := false

	if s.pool != nil {
		// Count sales in last 10 minutes
		_ = s.pool.QueryRow(ctx, `
			SELECT count(*) FROM sales
			WHERE user_id = $1 AND created_at >= (now() - interval '10 minutes');
		`, merchantID).Scan(&velocity)

		// Check failed attempts from limits_blocks
		_ = s.pool.QueryRow(ctx, `
			SELECT COALESCE(MAX(failed_attempts), 0) FROM limits_blocks
			WHERE merchant_id = $1;
		`, merchantID).Scan(&failures)

		if amount >= 1000000 { // > 1,000,000 IQD in a single ticket
			isAbnormal = true
		}
	}

	score, flags := ComputeRisk(txHour, velocity, failures, isAbnormal)

	action := "allow"
	isFrozen := false

	if score >= 90 {
		action = "freeze"
		isFrozen = true
		if s.pool != nil {
			_, _ = s.pool.Exec(ctx, "UPDATE users SET wallet_status = 'frozen' WHERE id = $1;", merchantID)
		}
	} else if score >= 70 {
		action = "alert"
	}

	if s.pool != nil {
		flagsJSON, _ := json.Marshal(flags)
		upsertQuery := `
			INSERT INTO risk_profiles (merchant_id, risk_score, velocity_count, failed_auth_count, suspicious_flags, is_auto_frozen, last_evaluated_at)
			VALUES ($1, $2, $3, $4, $5, $6, now())
			ON CONFLICT (merchant_id)
			DO UPDATE SET risk_score = EXCLUDED.risk_score,
			              velocity_count = EXCLUDED.velocity_count,
			              failed_auth_count = EXCLUDED.failed_auth_count,
			              suspicious_flags = EXCLUDED.suspicious_flags,
			              is_auto_frozen = EXCLUDED.is_auto_frozen,
			              last_evaluated_at = now();
		`
		_, _ = s.pool.Exec(ctx, upsertQuery, merchantID, score, velocity, failures, flagsJSON, isFrozen)
	}

	return &RiskAssessment{
		MerchantID:  merchantID,
		Score:       score,
		Flags:       flags,
		ActionTaken: action,
		IsFrozen:    isFrozen,
	}, nil
}

// ListRiskProfiles returns all evaluated profiles for the Admin Console.
func (s *Service) ListRiskProfiles(ctx context.Context) ([]RiskProfile, error) {
	if s.pool == nil {
		return []RiskProfile{}, nil
	}

	query := `
		SELECT r.merchant_id, COALESCE(u.store_name, u.full_name, u.email), r.risk_score,
		       r.velocity_count, r.failed_auth_count, r.suspicious_flags, r.is_auto_frozen,
		       COALESCE(u.wallet_status, 'active'), r.last_evaluated_at
		FROM risk_profiles r
		JOIN users u ON u.id = r.merchant_id
		ORDER BY r.risk_score DESC, r.last_evaluated_at DESC;
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list risk profiles: %w", err)
	}
	defer rows.Close()

	var list []RiskProfile
	for rows.Next() {
		var p RiskProfile
		var flagsRaw []byte
		if err := rows.Scan(&p.MerchantID, &p.StoreName, &p.RiskScore, &p.VelocityCount, &p.FailedAuthCount, &flagsRaw, &p.IsAutoFrozen, &p.WalletStatus, &p.LastEvaluatedAt); err == nil {
			_ = json.Unmarshal(flagsRaw, &p.SuspiciousFlags)
			list = append(list, p)
		}
	}
	return list, nil
}

// OverrideRiskProfile allows admin to reset score and unfreeze account.
func (s *Service) OverrideRiskProfile(ctx context.Context, merchantID int64, newScore int, unfreeze bool) error {
	if s.pool == nil {
		return nil
	}
	if newScore < 0 {
		newScore = 0
	}
	if newScore > 100 {
		newScore = 100
	}

	query := `
		UPDATE risk_profiles
		SET risk_score = $1, is_auto_frozen = FALSE, last_evaluated_at = now()
		WHERE merchant_id = $2;
	`
	_, err := s.pool.Exec(ctx, query, newScore, merchantID)
	if err != nil {
		return err
	}

	if unfreeze {
		_, err = s.pool.Exec(ctx, "UPDATE users SET wallet_status = 'active' WHERE id = $1;", merchantID)
	}
	return err
}
