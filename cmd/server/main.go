// VENUS POS backend entry point.
// Fiber v2 + pgx/v5 + JWT auth + WebSocket realtime hub.
// All timestamps are rendered in Asia/Baghdad (UTC+3).
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"venus_backend/internal/db"
	"venus_backend/internal/handlers"
	"venus_backend/internal/middleware"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	// Baghdad timezone (UTC+3) for all local time rendering.
	loc, err := time.LoadLocation("Asia/Baghdad")
	if err != nil {
		log.Fatalf("load Asia/Baghdad timezone: %v", err)
	}
	time.Local = loc
	log.Printf("timezone set to %s (UTC+3)", loc)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}
	port := getenv("PORT", "8080")
	superAdminEmail := getenv("SUPER_ADMIN_EMAIL", "Mahmoudraheem76@gmail.com")
	tokenTTL := 24 * time.Hour

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatalf("database connect: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("migration: %v", err)
	}
	log.Println("schema migrated successfully")

	hub := handlers.NewWSHub()

	adminH := &handlers.AdminHandler{Pool: pool, Hub: hub}
	// R1: one-time clean reset (reset_v2_clean) — runs exactly ONCE per deploy,
	// guarded by the system_meta marker so restarts never wipe data again.
	if err := adminH.RunOneTimeResets(ctx); err != nil {
		log.Printf("one-time reset check skipped: %v", err)
	} else {
		log.Println("one-time migrations verified (reset_v2_clean)")
	}

	authH := &handlers.AuthHandler{Pool: pool, JWTSecret: jwtSecret, TokenTTL: tokenTTL, SuperAdminEmail: superAdminEmail}
	simH := &handlers.SimHandler{Pool: pool, Hub: hub}
	debtsH := &handlers.DebtsHandler{Pool: pool, Hub: hub}
	ticketsH := &handlers.TicketsHandler{Pool: pool, Hub: hub}
	backupsH := &handlers.BackupsHandler{Pool: pool, Hub: hub}
	expensesH := &handlers.ExpensesHandler{Pool: pool, Hub: hub}
	announcementsH := &handlers.AnnouncementsHandler{Pool: pool, Hub: hub}
	zainH := &handlers.ZainCashHandler{Pool: pool, Hub: hub, JWTSecret: jwtSecret}
	vouchersH := &handlers.VouchersHandler{Pool: pool, Hub: hub}
	syncH := &handlers.SyncHandler{Pool: pool, Hub: hub}

	app := fiber.New(fiber.Config{
		AppName:      "VENUS POS Backend",
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	})
	app.Use(cors.New(cors.Config{
		AllowOrigins:     "*",
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, X-User-Id, X-User-Email, X-Requested-With, Accept-Language",
		ExposeHeaders:    "Content-Length, Content-Type",
		AllowCredentials: false,
	}))
	app.Use(recover.New())
	app.Use(logger.New())

	// OPTIONS preflight handler
	app.Options("/*", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "time": time.Now()})
	})

	// Realtime WebSocket endpoint (JWT via ?token= or Authorization header).
	app.Get("/ws", hub.Handler(jwtSecret))

	api := app.Group("/api")

	// Public auth endpoints.
	api.Post("/auth/login", authH.Login)
	api.Post("/auth/register", authH.Register)

	// Unified authoritative sync state (public fallback to UID 1 or header-based UID).
	api.Get("/sync/state", syncH.GetSyncState)
	api.Post("/subscriptions/redeem-code", authH.RedeemActivationCode)

	// Public ZainCash browser, webhook, and payment endpoints (NO Authorization header required).
	api.Post("/payments/zaincash/init", zainH.InitZainCash)
	api.Post("/payments/zaincash/confirm", zainH.ConfirmInAppZainCash)
	api.Get("/payments/zaincash/inquiry/:transactionId", zainH.InquiryZainCash)
	api.Get("/zaincash/checkout", zainH.CheckoutZainCash)
	api.Post("/zaincash/checkout/confirm", zainH.ConfirmCheckoutZainCash)
	api.Get("/zaincash/redirect", zainH.RedirectZainCash)
	api.Post("/zaincash/webhook", zainH.WebhookZainCash)

	// Authenticated subscriber area.
	auth := api.Group("", middleware.JWT(jwtSecret))
	auth.Get("/auth/me", authH.Me)
	auth.Put("/auth/credentials", authH.ChangeCredentials)
	auth.Get("/sync/state", syncH.GetSyncState)
	auth.Post("/subscriptions/redeem-code", authH.RedeemActivationCode)
	auth.Post("/payments/zaincash/confirm", zainH.ConfirmInAppZainCash)

	auth.Get("/sims", simH.ListSims)
	auth.Post("/sims", simH.AddSims)
	auth.Post("/sims/sell", simH.SellSim)
	auth.Delete("/sims/:id", simH.DeleteSim)
	auth.Get("/sales", simH.ListSales)
	auth.Delete("/sales/:id", simH.DeleteSale)
	auth.Post("/credit-sales", simH.AddCreditSale)
	auth.Get("/credit-sales", simH.ListCreditSales)
	auth.Delete("/credit-sales/:id", simH.DeleteCreditSale)

	auth.Get("/debts", debtsH.ListDebts)
	auth.Post("/debts", debtsH.CreateDebt)
	auth.Put("/debts/:id", debtsH.UpdateDebt)
	auth.Delete("/debts/:id", debtsH.DeleteDebt)
	auth.Post("/debts/:id/pay", debtsH.PayDebt)
	auth.Get("/debts/:id/payments", debtsH.ListPayments)

	auth.Get("/tickets", ticketsH.ListMyTickets)
	auth.Post("/tickets", ticketsH.CreateTicket)

	auth.Get("/expenses", expensesH.ListExpenses)
	auth.Post("/expenses", expensesH.AddExpense)
	auth.Delete("/expenses/:id", expensesH.DeleteExpense)

	auth.Get("/announcements", announcementsH.ListAnnouncements)

	// Encrypted Voucher Vault
	auth.Post("/vouchers/upload", vouchersH.UploadVouchers)
	auth.Get("/vouchers", vouchersH.GetVouchers)
	auth.Post("/vouchers/dispense", vouchersH.DispenseVoucher)

	// Admin-only area.
	admin := api.Group("/admin", middleware.JWT(jwtSecret), middleware.AdminOnly())
	admin.Get("/stats", adminH.GetAdminStats)
	admin.Get("/zaincash-transactions", adminH.ListZainCashTransactions)
	admin.Get("/users", authH.ListUsers)
	admin.Post("/users", adminH.AdminCreateUser)
	admin.Put("/users/:id/approve", authH.ApproveUser)
	admin.Put("/users/:id/credentials", authH.AdminChangeUserCredentials)
	admin.Put("/users/:id", adminH.AdminUpdateUser)
	admin.Delete("/users/:id", adminH.AdminDeleteUser)
	admin.Post("/users/:id/wallet", adminH.AdminAdjustWallet)
	admin.Post("/wallet/credit", adminH.AdminAdjustWallet)
	admin.Get("/users/:id/backup", backupsH.Backup)
	admin.Post("/users/:id/restore", backupsH.Restore)
	admin.Get("/tickets", ticketsH.ListAllTickets)
	admin.Put("/tickets/:id/reply", ticketsH.ReplyTicket)
	admin.Post("/announcements", announcementsH.SaveAnnouncement)
	admin.Delete("/announcements/:id", announcementsH.DeleteAnnouncement)

	// Admin Activation Codes management
	admin.Post("/activation-codes/generate", adminH.GenerateActivationCodes)
	admin.Get("/activation-codes", adminH.ListActivationCodes)

	// Admin Voucher Vault management
	admin.Get("/vouchers", vouchersH.GetVouchers)
	admin.Post("/vouchers/upload", vouchersH.UploadVouchers)

	// R1/R4/R5A: factory reset, audit trail, digital cards management.
	admin.Post("/factory-reset", adminH.FactoryReset)
	admin.Get("/audit-logs", adminH.ListAuditLogs)
	admin.Get("/login-history", adminH.ListLoginHistory)
	admin.Get("/digital-cards", adminH.ListDigitalCards)
	admin.Post("/digital-cards", adminH.AdminSaveDigitalCard)
	admin.Put("/digital-cards/:id", adminH.AdminSaveDigitalCard)
	admin.Delete("/digital-cards/:id", adminH.AdminDeleteDigitalCard)
	admin.Get("/cards", adminH.ListDigitalCards)
	admin.Post("/cards", adminH.AdminSaveDigitalCard)
	admin.Put("/cards/:id", adminH.AdminSaveDigitalCard)
	admin.Delete("/cards/:id", adminH.AdminDeleteDigitalCard)

	// R5A: digital cards catalog for the mobile بطاقات screen.
	auth.Get("/digital-cards", adminH.ListDigitalCards)
	auth.Get("/cards", adminH.ListDigitalCards)

	// Additional sales route aliases for seamless compatibility
	auth.Post("/sim/sales", simH.SellSim)
	auth.Get("/sim/sales", simH.ListSales)
	auth.Post("/credit/sales", simH.AddCreditSale)
	auth.Get("/credit/sales", simH.ListCreditSales)

	// R5B: domain-isolated web console hosting:
	// - console.urukiq.com: serves Flutter Web Admin Console
	// - vapi.urukiq.com: API only (returns status JSON on /, 404 on /console)
	adminDist := getenv("ADMIN_DIST_DIR", "./admin_dist")
	registerConsoleRoutes(app, adminDist)
	log.Printf("domain-isolated routing enabled (console.urukiq.com -> %s)", adminDist)

	go func() {
		log.Printf("VENUS POS backend listening on :%s", port)
		if err := app.Listen(":" + port); err != nil {
			log.Fatalf("listen: %v", err)
		}
	}()

	// Graceful shutdown.
	<-ctx.Done()
	log.Println("shutdown signal received, draining...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Printf("graceful shutdown error: %v", err)
	}
	log.Println("server stopped")
}
