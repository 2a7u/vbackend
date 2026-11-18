package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestConsoleDomainIsolation(t *testing.T) {
	// Create a dummy dist directory
	tmpDir, err := os.MkdirTemp("", "console_dist_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dummyHTML := "<html><head><title>Admin Console</title></head><body><h1>Console</h1></body></html>"
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte(dummyHTML), 0644); err != nil {
		t.Fatalf("failed to write dummy index.html: %v", err)
	}

	app := fiber.New()
	registerConsoleRoutes(app, tmpDir)

	// 1. Host = vapi.urukiq.com -> GET / MUST return JSON {"service":"VENUS Core API","status":"online"}
	req1 := httptest.NewRequest(http.MethodGet, "http://vapi.urukiq.com/", nil)
	resp1, err := app.Test(req1, 2000)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("vapi.urukiq.com GET / status expected 200, got %d", resp1.StatusCode)
	}
	body1, _ := io.ReadAll(resp1.Body)
	var map1 map[string]string
	if err := json.Unmarshal(body1, &map1); err != nil {
		t.Fatalf("failed to parse JSON response on vapi /: %v, body=%s", err, string(body1))
	}
	if map1["service"] != "VENUS Core API" || map1["status"] != "online" {
		t.Errorf("unexpected json on vapi /: %v", map1)
	}

	// 2. Host = vapi.urukiq.com -> GET /console MUST return 404 Not Found
	req2 := httptest.NewRequest(http.MethodGet, "http://vapi.urukiq.com/console", nil)
	resp2, err := app.Test(req2, 2000)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("vapi.urukiq.com GET /console expected 404, got %d", resp2.StatusCode)
	}

	// 3. Host = vapi.urukiq.com -> GET /console/users MUST return 404 Not Found
	req3 := httptest.NewRequest(http.MethodGet, "http://vapi.urukiq.com/console/users", nil)
	resp3, err := app.Test(req3, 2000)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp3.StatusCode != http.StatusNotFound {
		t.Errorf("vapi.urukiq.com GET /console/users expected 404, got %d", resp3.StatusCode)
	}

	// 4. Host = console.urukiq.com -> GET / MUST return index.html
	req4 := httptest.NewRequest(http.MethodGet, "http://console.urukiq.com/", nil)
	resp4, err := app.Test(req4, 2000)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("console.urukiq.com GET / status expected 200, got %d", resp4.StatusCode)
	}
	body4, _ := io.ReadAll(resp4.Body)
	if string(body4) != dummyHTML {
		t.Errorf("console.urukiq.com GET / expected dummy html, got %s", string(body4))
	}

	// 5. Host = console.urukiq.com -> GET /users (SPA route) MUST fallback to index.html
	req5 := httptest.NewRequest(http.MethodGet, "http://console.urukiq.com/users", nil)
	resp5, err := app.Test(req5, 2000)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp5.StatusCode != http.StatusOK {
		t.Fatalf("console.urukiq.com GET /users status expected 200, got %d", resp5.StatusCode)
	}
	body5, _ := io.ReadAll(resp5.Body)
	if string(body5) != dummyHTML {
		t.Errorf("console.urukiq.com GET /users expected dummy html, got %s", string(body5))
	}

	// 6. Embedded Console Fallback: When distDir is non-existent, serve embeddedConsoleHTML
	appEmbedded := fiber.New()
	registerConsoleRoutes(appEmbedded, "/nonexistent_dir")
	req6 := httptest.NewRequest(http.MethodGet, "http://console.urukiq.com/", nil)
	resp6, err := appEmbedded.Test(req6, 2000)
	if err != nil {
		t.Fatalf("appEmbedded.Test error: %v", err)
	}
	if resp6.StatusCode != http.StatusOK {
		t.Fatalf("console.urukiq.com embedded GET / status expected 200, got %d", resp6.StatusCode)
	}
	body6, _ := io.ReadAll(resp6.Body)
	if !strings.Contains(string(body6), "فينوس") || !strings.Contains(string(body6), "لوحة التحكم") {
		t.Errorf("expected embedded console html containing فينوس and لوحة التحكم, got %s", string(body6)[:100])
	}

	// 7. Logo Endpoint: GET /assets/appicon.png MUST return 200 image/png
	req7 := httptest.NewRequest(http.MethodGet, "http://console.urukiq.com/assets/appicon.png", nil)
	resp7, err := appEmbedded.Test(req7, 2000)
	if err != nil {
		t.Fatalf("appEmbedded.Test logo error: %v", err)
	}
	if resp7.StatusCode != http.StatusOK {
		t.Fatalf("GET /assets/appicon.png status expected 200, got %d", resp7.StatusCode)
	}
	if ct := resp7.Header.Get("Content-Type"); !strings.Contains(ct, "image/png") {
		t.Errorf("GET /assets/appicon.png expected Content-Type image/png, got %s", ct)
	}

	// 8. Localhost fallback: GET http://localhost/console MUST return 200 OK
	req8 := httptest.NewRequest(http.MethodGet, "http://localhost/console", nil)
	resp8, err := appEmbedded.Test(req8, 2000)
	if err != nil {
		t.Fatalf("localhost /console error: %v", err)
	}
	if resp8.StatusCode != http.StatusOK {
		t.Fatalf("localhost GET /console status expected 200, got %d", resp8.StatusCode)
	}
}
