// R5B: domain-isolated hosting for the Native Go Admin Console.
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
	_ "embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

//go:embed console.html
var embeddedConsoleHTML string

//go:embed appicon.png
var embeddedAppIcon []byte

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

func isLocalHost(c *fiber.Ctx) bool {
	host := strings.ToLower(c.Hostname())
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host == "localhost" || host == "127.0.0.1" || strings.HasPrefix(host, "192.168.")
}

// noCacheHeaders sets strict no-cache response headers.
func noCacheHeaders(c *fiber.Ctx) {
	c.Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")
}

// serveConsoleIndex sends console.html (or distDir index.html if present) with strict no-cache headers.
func serveConsoleIndex(c *fiber.Ctx, distDir string) error {
	noCacheHeaders(c)
	c.Set(fiber.HeaderContentType, "text/html; charset=utf-8")
	if distDir != "" {
		if data, err := os.ReadFile(filepath.Join(distDir, "index.html")); err == nil && len(data) > 0 {
			return c.SendString(string(data))
		}
	}
	if embeddedConsoleHTML != "" {
		return c.SendString(embeddedConsoleHTML)
	}
	return fiber.ErrNotFound
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
// - console.urukiq.com: serves Native Go admin console
// - vapi.urukiq.com: API only, returns JSON status on "/", 404 on "/console"
func registerConsoleRoutes(app *fiber.App, distDir string) {
	// Public app icon / logo routes (accessible everywhere)
	app.Get("/assets/appicon.png", func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderContentType, "image/png")
		c.Set("Cache-Control", "public, max-age=86400")
		if len(embeddedAppIcon) > 0 {
			return c.Send(embeddedAppIcon)
		}
		return c.SendFile("./public/appicon.png")
	})
	app.Get("/appicon.png", func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderContentType, "image/png")
		c.Set("Cache-Control", "public, max-age=86400")
		if len(embeddedAppIcon) > 0 {
			return c.Send(embeddedAppIcon)
		}
		return c.SendFile("./public/appicon.png")
	})
	app.Get("/favicon.ico", func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderContentType, "image/png")
		if len(embeddedAppIcon) > 0 {
			return c.Send(embeddedAppIcon)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	isReserved := func(p string) bool {
		return p == "/api" || strings.HasPrefix(p, "/api/") ||
			p == "/ws" || strings.HasPrefix(p, "/ws/") ||
			p == "/health" ||
			p == "/assets/appicon.png" || p == "/appicon.png" || p == "/favicon.ico"
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

	// 2. /console route: 404 on vapi, redirect to / on console host, serve on localhost
	app.All("/console", func(c *fiber.Ctx) error {
		if isConsoleHost(c) {
			return c.Redirect("/", fiber.StatusMovedPermanently)
		}
		if isLocalHost(c) {
			return serveConsoleIndex(c, distDir)
		}
		return fiber.ErrNotFound
	})
	app.All("/console/*", func(c *fiber.Ctx) error {
		if isConsoleHost(c) {
			rel := strings.TrimPrefix(c.Path(), "/console/")
			return c.Redirect("/"+rel, fiber.StatusMovedPermanently)
		}
		if isLocalHost(c) {
			return serveConsoleIndex(c, distDir)
		}
		return fiber.ErrNotFound
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
