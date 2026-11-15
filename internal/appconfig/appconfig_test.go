package appconfig_test

import (
	"testing"
	"venus_backend/internal/appconfig"
)

func TestAppConfigHelpers(t *testing.T) {
	svc := appconfig.NewService(nil, nil)

	// In-memory set
	_ = svc.Set(nil, "feature_flags", map[string]any{
		"zaincash_gateway": true,
		"beta_feature":     false,
	}, 1)

	if !svc.IsFeatureEnabled("zaincash_gateway") {
		t.Fatalf("expected feature zaincash_gateway to be enabled")
	}
	if svc.IsFeatureEnabled("beta_feature") {
		t.Fatalf("expected feature beta_feature to be disabled")
	}

	// Maintenance mode
	_ = svc.Set(nil, "maintenance_mode", map[string]any{
		"enabled":          true,
		"message":          "صيانة مجدولة",
		"whitelisted_uids": []any{float64(1), float64(99)},
	}, 1)

	blocked, _ := svc.IsMaintenanceActive(50)
	if !blocked {
		t.Fatalf("expected user 50 to be blocked during maintenance")
	}
	whitelisted, _ := svc.IsMaintenanceActive(1)
	if whitelisted {
		t.Fatalf("expected admin user 1 to bypass maintenance")
	}

	// Force update
	_ = svc.Set(nil, "force_update", map[string]any{
		"min_version": "2.0.0",
		"is_forced":   true,
		"update_url":  "https://urukiq.com",
	}, 1)

	reqUpdate, minVer, _ := svc.CheckForceUpdate("1.9.0")
	if !reqUpdate || minVer != "2.0.0" {
		t.Fatalf("expected force update required for version 1.9.0")
	}
	reqUpdateOld, _, _ := svc.CheckForceUpdate("2.0.1")
	if reqUpdateOld {
		t.Fatalf("expected version 2.0.1 to not require forced update")
	}
}
