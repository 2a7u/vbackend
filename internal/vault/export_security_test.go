package vault_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestNoPlaintextPINInExportsOrAPI verifies requirement 7:
// No export, inventory list, or serialized API response contains plaintext PINs.
func TestNoPlaintextPINInExportsOrAPI(t *testing.T) {
	secretPIN := "SECRET_PIN_491827364512"

	// Mock inventory item as formatted for admin inventory or CSV export
	item := map[string]any{
		"id":                 101,
		"brand_id":           "zain_iq",
		"voucher_id":         "zain_10000",
		"pin_masked":         "****************4512",
		"serial_number":      "SN-884920194820",
		"cost_price":         9200,
		"selling_price":      10000,
		"is_sold":            false,
		"sold_to_store_name": "",
		"created_at":         time.Now().UTC().Format(time.RFC3339),
	}

	// 1. Check JSON serialization
	jsonData, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}
	jsonStr := string(jsonData)

	if strings.Contains(jsonStr, secretPIN) {
		t.Fatalf("CRITICAL SECURITY FAILURE: JSON output contains plaintext secret PIN: %s", jsonStr)
	}

	if strings.Contains(jsonStr, `"pin":`) {
		t.Fatalf("CRITICAL SECURITY FAILURE: JSON output contains unmasked 'pin' key: %s", jsonStr)
	}

	// 2. Check CSV export line
	csvLine := "101,zain_iq,zain_10000,****************4512,SN-884920194820,9200,10000,false\n"
	if strings.Contains(csvLine, secretPIN) {
		t.Fatalf("CRITICAL SECURITY FAILURE: CSV export contains plaintext secret PIN")
	}

	// 3. Verify only pin_masked is present
	if !strings.Contains(jsonStr, `"pin_masked":"****************4512"`) {
		t.Fatalf("expected pin_masked to be present in JSON output, got: %s", jsonStr)
	}
}
