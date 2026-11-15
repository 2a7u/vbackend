package security_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestHMACSignatureComputation(t *testing.T) {
	secret := "webhook_super_secret_key"
	payload := []byte(`{"event":"wallet_frozen","user_id":101,"reason":"fraud_velocity"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig1 := hex.EncodeToString(mac.Sum(nil))

	mac2 := hmac.New(sha256.New, []byte(secret))
	mac2.Write(payload)
	sig2 := hex.EncodeToString(mac2.Sum(nil))

	if sig1 != sig2 {
		t.Fatalf("HMAC signature must be deterministic")
	}

	// Tampered payload
	mac3 := hmac.New(sha256.New, []byte(secret))
	mac3.Write([]byte(`{"event":"wallet_frozen","user_id":102,"reason":"fraud_velocity"}`))
	sig3 := hex.EncodeToString(mac3.Sum(nil))

	if sig1 == sig3 {
		t.Fatalf("tampered payload must produce distinct HMAC signature")
	}
}

func TestAPIKeyHashUniqueness(t *testing.T) {
	key1 := "vsk_live_111122223333444455556666"
	key2 := "vsk_live_999988887777666655554444"

	h1 := sha256.Sum256([]byte(key1))
	h2 := sha256.Sum256([]byte(key2))

	if hex.EncodeToString(h1[:]) == hex.EncodeToString(h2[:]) {
		t.Fatalf("distinct keys must have distinct hashes")
	}
}
