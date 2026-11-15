package seal

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrTokenNotFound = errors.New("invalid or expired receipt verification token")
	ErrInvalidToken  = errors.New("seal signature verification failed")
)

type SealToken struct {
	ID          int64      `json:"id"`
	MerchantID  int64      `json:"merchant_id"`
	Token       string     `json:"token"`
	ReceiptType string     `json:"receipt_type"`
	ReceiptID   string     `json:"receipt_id"`
	Amount      int64      `json:"amount"` // in Iraqi Dinars
	Signature   string     `json:"signature"`
	IssuedAt    time.Time  `json:"issued_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type VerificationResult struct {
	Valid        bool      `json:"valid"`
	ReceiptType  string    `json:"receipt_type"`
	ReceiptID    string    `json:"receipt_id"`
	Amount       int64     `json:"amount"`
	Currency     string    `json:"currency"`
	MerchantName string    `json:"merchant_name"`
	StoreName    string    `json:"store_name"`
	IssuedAt     time.Time `json:"issued_at"`
	BaghdadTime  string    `json:"baghdad_time"`
	VerifiedAt   time.Time `json:"verified_at"`
}

type Service struct {
	pool      *pgxpool.Pool
	hmacKey   []byte
	serverURL string
}

func NewService(pool *pgxpool.Pool, hmacSecret, serverURL string) *Service {
	if hmacSecret == "" {
		hmacSecret = "venus_seal_default_secret_key_32bytes!"
	}
	if serverURL == "" {
		serverURL = "https://vapi.urukiq.com"
	}
	return &Service{
		pool:      pool,
		hmacKey:   []byte(hmacSecret),
		serverURL: serverURL,
	}
}

// GenerateSignature produces HMAC-SHA256 signature for (id + amount + timestamp).
func (s *Service) GenerateSignature(receiptID string, amount int64, issuedAt time.Time) string {
	payload := fmt.Sprintf("%s:%d:%d", receiptID, amount, issuedAt.Unix())
	mac := hmac.New(sha256.New, s.hmacKey)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func generateShortToken() string {
	const charset = "abcdefghjkmnpqrstuvwxyz23456789" // omit confusing 0,o,1,l
	b := make([]byte, 10)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// CreateTokenTx creates and stores a cryptographically signed seal token inside an existing transaction.
func (s *Service) CreateTokenTx(ctx context.Context, tx pgx.Tx, merchantID int64, receiptType, receiptID string, amount int64) (*SealToken, error) {
	issuedAt := time.Now().UTC()
	sig := s.GenerateSignature(receiptID, amount, issuedAt)
	tokenStr := fmt.Sprintf("v-%s", generateShortToken())

	query := `
		INSERT INTO seal_tokens (merchant_id, token, receipt_type, receipt_id, amount, signature, issued_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
		RETURNING id;
	`
	var id int64
	err := tx.QueryRow(ctx, query, merchantID, tokenStr, receiptType, receiptID, amount, sig, issuedAt).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("insert seal token: %w", err)
	}

	return &SealToken{
		ID:          id,
		MerchantID:  merchantID,
		Token:       tokenStr,
		ReceiptType: receiptType,
		ReceiptID:   receiptID,
		Amount:      amount,
		Signature:   sig,
		IssuedAt:    issuedAt,
		CreatedAt:   issuedAt,
	}, nil
}

// CreateToken creates and stores a seal token using pool.
func (s *Service) CreateToken(ctx context.Context, merchantID int64, receiptType, receiptID string, amount int64) (*SealToken, error) {
	if s.pool == nil {
		issuedAt := time.Now().UTC()
		sig := s.GenerateSignature(receiptID, amount, issuedAt)
		return &SealToken{
			Token:       fmt.Sprintf("v-%s", generateShortToken()),
			MerchantID:  merchantID,
			ReceiptType: receiptType,
			ReceiptID:   receiptID,
			Amount:      amount,
			Signature:   sig,
			IssuedAt:    issuedAt,
		}, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin seal tx: %w", err)
	}
	defer tx.Rollback(ctx)

	token, err := s.CreateTokenTx(ctx, tx, merchantID, receiptType, receiptID, amount)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit seal tx: %w", err)
	}
	return token, nil
}

// FormatVerificationURL returns the full public URL for scanning.
func (s *Service) FormatVerificationURL(token string) string {
	return fmt.Sprintf("%s/verify/%s", s.serverURL, token)
}

// VerifyToken validates the token and HMAC signature, retrieving invoice details.
func (s *Service) VerifyToken(ctx context.Context, token string) (*VerificationResult, error) {
	if s.pool == nil {
		return nil, ErrTokenNotFound
	}

	query := `
		SELECT s.receipt_type, s.receipt_id, s.amount, s.signature, s.issued_at,
		       COALESCE(u.full_name, 'نقطة بيع معتمدة'), COALESCE(u.store_name, 'متجر أوروك')
		FROM seal_tokens s
		JOIN users u ON u.id = s.merchant_id
		WHERE s.token = $1;
	`
	var (
		recType, recID, sig string
		amount              int64
		issuedAt            time.Time
		fullName, storeName string
	)
	err := s.pool.QueryRow(ctx, query, token).Scan(&recType, &recID, &amount, &sig, &issuedAt, &fullName, &storeName)
	if err == pgx.ErrNoRows {
		return nil, ErrTokenNotFound
	} else if err != nil {
		return nil, fmt.Errorf("query seal token: %w", err)
	}

	// Verify HMAC signature
	expectedSig := s.GenerateSignature(recID, amount, issuedAt)
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return nil, ErrInvalidToken
	}

	// Format Baghdad Time (UTC + 3)
	loc := time.FixedZone("Asia/Baghdad", 3*60*60)
	baghdadTime := issuedAt.In(loc).Format("2006-01-02 15:04:05")

	return &VerificationResult{
		Valid:        true,
		ReceiptType:  recType,
		ReceiptID:    recID,
		Amount:       amount,
		Currency:     "IQD",
		MerchantName: fullName,
		StoreName:    storeName,
		IssuedAt:     issuedAt,
		BaghdadTime:  baghdadTime,
		VerifiedAt:   time.Now().In(loc),
	}, nil
}
