// R5B: domain-isolated hosting for the built Flutter web admin console (admin_dist).
//
// Rules:
//   - When host == "console.urukiq.com" (or console.*):
//     Serve admin console at "/" with SPA fallback to index.html and strict no-cache.
//   - When host == "vapi.urukiq.com" (or any other host):
//     NEVER serve admin console.
//     GET "/" returns {"service": "VENUS Core API", "status": "online"}.
//     GET "/console" returns 404 Not Found.
package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// consoleShellFiles must never be cached by browsers or proxies.
var consoleShellFiles = map[string]bool{
	"index.html":                true,
	"flutter_service_worker.js": true,
	"flutter_bootstrap.js":      true,
	"version.json":              true,
	"manifest.json":             true,
}

// isConsoleHost returns true only when the request hostname is console.urukiq.com or console.*.
func isConsoleHost(c *fiber.Ctx) bool {
	host := strings.ToLower(c.Hostname())
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host == "console.urukiq.com" || strings.HasPrefix(host, "console.")
}

// noCacheHeaders sets strict no-cache response headers.
func noCacheHeaders(c *fiber.Ctx) {
	c.Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")
}

// serveConsoleIndex sends index.html with base href "/" and no-cache headers.
func serveConsoleIndex(c *fiber.Ctx, distDir string) error {
	data, err := os.ReadFile(filepath.Join(distDir, "index.html"))
	if err != nil {
		return fiber.ErrNotFound
	}
	noCacheHeaders(c)
	c.Set(fiber.HeaderContentType, "text/html; charset=utf-8")
	return c.SendString(string(data))
}

// sendConsoleFile serves one static asset from distDir. Returns false when
// the path does not resolve to a regular file (caller decides the fallback).
func sendConsoleFile(c *fiber.Ctx, distDir, rel string) (bool, error) {
	if strings.Contains(rel, "..") {
		return false, nil
	}
	full := filepath.Join(distDir, filepath.FromSlash(rel))
	st, err := os.Stat(full)
	if err != nil || st.IsDir() {
		return false, nil
	}
	if consoleShellFiles[strings.ToLower(filepath.Base(rel))] {
		noCacheHeaders(c)
	}
	if err := c.SendFile(full); err != nil {
		return false, err
	}
	return true, nil
}

// registerConsoleRoutes sets up domain-isolated routing:
// - console.urukiq.com: serves admin console
// - vapi.urukiq.com: API only, returns JSON status on "/", 404 on "/console"
func registerConsoleRoutes(app *fiber.App, distDir string) {
	isReserved := func(p string) bool {
		return p == "/api" || strings.HasPrefix(p, "/api/") ||
			p == "/ws" || strings.HasPrefix(p, "/ws/") ||
			p == "/health"
	}

	// 1. Root route: domain check
	app.Get("/", func(c *fiber.Ctx) error {
		if isConsoleHost(c) {
			return serveConsoleIndex(c, distDir)
		}
		// vapi.urukiq.com or any non-console host
		return c.JSON(fiber.Map{
			"service": "VENUS Core API",
			"status":  "online",
		})
	})

	// 2. /console route: 404 on vapi, redirect to / on console host
	app.All("/console", func(c *fiber.Ctx) error {
		if !isConsoleHost(c) {
			return fiber.ErrNotFound
		}
		return c.Redirect("/", fiber.StatusMovedPermanently)
	})
	app.All("/console/*", func(c *fiber.Ctx) error {
		if !isConsoleHost(c) {
			return fiber.ErrNotFound
		}
		rel := strings.TrimPrefix(c.Path(), "/console/")
		return c.Redirect("/"+rel, fiber.StatusMovedPermanently)
	})

	// 3. Fallback / SPA route for console host; 404 for API host
	app.Get("/*", func(c *fiber.Ctx) error {
		p := c.Path()
		if isReserved(p) {
			return fiber.ErrNotFound
		}
		if !isConsoleHost(c) {
			return fiber.ErrNotFound
		}

		rel := strings.TrimPrefix(p, "/")
		if rel == "" || strings.EqualFold(rel, "index.html") {
			return serveConsoleIndex(c, distDir)
		}
		served, err := sendConsoleFile(c, distDir, rel)
		if err != nil {
			return err
		}
		if !served {
			return serveConsoleIndex(c, distDir)
		}
		return nil
	})
}
