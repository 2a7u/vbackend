// R5B: hosting for the built Flutter web admin console (admin_dist).
// The console is served at BOTH "/" and "/console" with:
//   - <base href> rewrite for the /console mount point,
//   - strict no-cache headers on the app shell (index.html, service worker,
//     bootstrap and version files) so every deploy is picked up instantly,
//   - SPA fallback so deep links render the app instead of a 404.
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

// noCacheHeaders sets strict no-cache response headers.
func noCacheHeaders(c *fiber.Ctx) {
	c.Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")
}

// serveConsoleIndex sends index.html, rewriting <base href="/"> when the
// console is mounted under /console.
func serveConsoleIndex(c *fiber.Ctx, distDir, baseHref string) error {
	data, err := os.ReadFile(filepath.Join(distDir, "index.html"))
	if err != nil {
		return fiber.ErrNotFound
	}
	html := string(data)
	if baseHref != "/" {
		html = strings.Replace(html, `<base href="/">`, `<base href="`+baseHref+`">`, 1)
	}
	noCacheHeaders(c)
	c.Set(fiber.HeaderContentType, "text/html; charset=utf-8")
	return c.SendString(html)
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

// registerConsoleRoutes mounts the console at /console and at /. It must be
// called AFTER all API routes are registered so /api, /ws and /health keep
// matching priority (Fiber resolves routes in registration order).
func registerConsoleRoutes(app *fiber.App, distDir string) {
	isReserved := func(p string) bool {
		return p == "/api" || strings.HasPrefix(p, "/api/") ||
			p == "/ws" || strings.HasPrefix(p, "/ws/") ||
			p == "/health"
	}

	// ---- /console mount (prefix stripped, base href rewritten) ----
	app.All("/console", func(c *fiber.Ctx) error {
		return serveConsoleIndex(c, distDir, "/console/")
	})
	app.Get("/console/*", func(c *fiber.Ctx) error {
		rel := strings.Trim(c.Params("*"), "/")
		if rel == "" || strings.EqualFold(rel, "index.html") {
			return serveConsoleIndex(c, distDir, "/console/")
		}
		served, err := sendConsoleFile(c, distDir, rel)
		if err != nil {
			return err
		}
		if !served {
			return serveConsoleIndex(c, distDir, "/console/")
		}
		return nil
	})

	// ---- root mount (registered last: /api, /ws, /health keep priority) ----
	app.Get("/", func(c *fiber.Ctx) error {
		return serveConsoleIndex(c, distDir, "/")
	})
	app.Get("/*", func(c *fiber.Ctx) error {
		p := c.Path()
		if isReserved(p) {
			return fiber.ErrNotFound
		}
		rel := strings.TrimPrefix(p, "/")
		if rel == "" || strings.EqualFold(rel, "index.html") {
			return serveConsoleIndex(c, distDir, "/")
		}
		served, err := sendConsoleFile(c, distDir, rel)
		if err != nil {
			return err
		}
		if !served {
			return serveConsoleIndex(c, distDir, "/")
		}
		return nil
	})
}
