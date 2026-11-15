package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

var (
	ErrInvalidCiphertext = errors.New("vault: invalid ciphertext or corrupted data")
	ErrInvalidNonce      = errors.New("vault: invalid nonce length for AES-GCM")
	ErrKeyNotFound       = errors.New("vault: encryption key version not found")
)

// VaultManager handles AES-256-GCM encryption, decryption, and HMAC duplicate guards.
type VaultManager struct {
	mu         sync.RWMutex
	keys       map[int][]byte
	currentVer int
	hmacSecret []byte
}

var (
	defaultManager *VaultManager
	once           sync.Once
)

// Default returns the singleton VaultManager initialized from environment variables.
func Default() *VaultManager {
	once.Do(func() {
		defaultManager = NewVaultManagerFromEnv()
	})
	return defaultManager
}

// NewVaultManagerFromEnv initializes the vault manager reading VAULT_ENCRYPTION_KEY and VAULT_HMAC_SECRET.
func NewVaultManagerFromEnv() *VaultManager {
	rawKey := os.Getenv("VAULT_ENCRYPTION_KEY")
	if strings.TrimSpace(rawKey) == "" {
		rawKey = "VENUS_VAULT_AES256_ROOT_KEY_2026_PRODUCTION"
	}

	rawHMAC := os.Getenv("VAULT_HMAC_SECRET")
	if strings.TrimSpace(rawHMAC) == "" {
		rawHMAC = "VENUS_VAULT_HMAC_SECRET_2026_PRODUCTION"
	}

	key32 := derive32ByteKey(rawKey)
	hmacKey := derive32ByteKey(rawHMAC)

	vm := &VaultManager{
		keys:       make(map[int][]byte),
		currentVer: 1,
		hmacSecret: hmacKey,
	}
	vm.keys[1] = key32

	return vm
}

// SetKey adds or updates an encryption key for a specific key version (supports key rotation).
func (v *VaultManager) SetKey(version int, rawKey string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.keys[version] = derive32ByteKey(rawKey)
	if version > v.currentVer {
		v.currentVer = version
	}
}

// EncryptPIN encrypts a plaintext PIN using AES-256-GCM with a unique 12-byte cryptographically secure random nonce.
func (v *VaultManager) EncryptPIN(pin string) (encryptedHex, nonceHex string, keyVer int, err error) {
	v.mu.RLock()
	keyVer = v.currentVer
	key, ok := v.keys[keyVer]
	v.mu.RUnlock()

	if !ok {
		return "", "", 0, fmt.Errorf("%w: version %d", ErrKeyNotFound, keyVer)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", 0, fmt.Errorf("aes new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", 0, fmt.Errorf("cipher new gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", "", 0, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(pin), nil)

	return hex.EncodeToString(ciphertext), hex.EncodeToString(nonce), keyVer, nil
}

// DecryptPIN decrypts an AES-256-GCM encrypted PIN using the specified key version.
func (v *VaultManager) DecryptPIN(encryptedHex, nonceHex string, keyVer int) (string, error) {
	v.mu.RLock()
	key, ok := v.keys[keyVer]
	v.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("%w: version %d", ErrKeyNotFound, keyVer)
	}

	ciphertext, err := hex.DecodeString(encryptedHex)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext hex: %w", err)
	}

	nonce, err := hex.DecodeString(nonceHex)
	if err != nil {
		return "", fmt.Errorf("decode nonce hex: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("cipher new gcm: %w", err)
	}

	if len(nonce) != gcm.NonceSize() {
		return "", ErrInvalidNonce
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrInvalidCiphertext
	}

	return string(plaintext), nil
}

// ComputeHMAC generates a deterministic HMAC-SHA256 hash for global duplicate PIN detection.
func (v *VaultManager) ComputeHMAC(pin string) string {
	mac := hmac.New(sha256.New, v.hmacSecret)
	mac.Write([]byte(strings.TrimSpace(pin)))
	return hex.EncodeToString(mac.Sum(nil))
}

// derive32ByteKey derives a deterministic 32-byte key from any raw input using SHA-256.
func derive32ByteKey(input string) []byte {
	hash := sha256.Sum256([]byte(input))
	key := make([]byte, 32)
	copy(key, hash[:])
	return key
}
