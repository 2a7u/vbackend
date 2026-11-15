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
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	apiPkg "venus_backend/internal/api"
	"venus_backend/internal/appconfig"
	"venus_backend/internal/bi"
	"venus_backend/internal/db"
	"venus_backend/internal/db/migrations"
	"venus_backend/internal/deviceradar"
	"venus_backend/internal/forecast"
	"venus_backend/internal/fraud"
	"venus_backend/internal/guard"
	"venus_backend/internal/handlers"
	"venus_backend/internal/hierarchy"
	"venus_backend/internal/idem"
	"venus_backend/internal/jobs"
	"venus_backend/internal/ledger"
	"venus_backend/internal/limits"
	"venus_backend/internal/middleware"
	"venus_backend/internal/notify"
	"venus_backend/internal/pricing"
	"venus_backend/internal/promotions"
	"venus_backend/internal/radar"
	"venus_backend/internal/reconcile"
	"venus_backend/internal/refund"
	"venus_backend/internal/reports"
	"venus_backend/internal/restock"
	"venus_backend/internal/rewards"
	"venus_backend/internal/risk"
	"venus_backend/internal/scheduler"
	"venus_backend/internal/seal"
	"venus_backend/internal/security"
	"venus_backend/internal/subscribers"
	syncPkg "venus_backend/internal/sync"
	"venus_backend/internal/surge"
	"venus_backend/internal/tills"
	"venus_backend/internal/vault"
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

	if n, err := migrations.ApplyPending(ctx, pool); err != nil {
		log.Fatalf("apply versioned migrations: %v", err)
	} else if n > 0 {
		log.Printf("applied %d pending database migrations", n)
	}

	hub := handlers.NewWSHub()

	// Fintech Core Services (Phase 1)
	ledgerSvc := ledger.NewService(pool, hub)
	fraudEngine := fraud.NewEngine(pool, hub)
	reconcileSvc := reconcile.NewService(pool)

	// Vault, Pricing, Hierarchy & Radar Services (Phase 2)
	vaultMgr := vault.Default()
	if err := vaultMgr.VerifyStartup(ctx, pool); err != nil {
		log.Fatalf("CRITICAL SECURITY VIOLATION: %v", err)
	}
	pricingSvc := pricing.NewService(pool)
	hierarchySvc := hierarchy.NewService(pool, ledgerSvc)
	radarSvc := radar.NewService(pool)

	fintechH := handlers.NewFintechHandler(pool, hub, ledgerSvc, fraudEngine, reconcileSvc, vaultMgr, pricingSvc, hierarchySvc, radarSvc)

	// Financial cron scheduler: daily integrity check, daily reconciliation, daily/monthly close
	cronSched := scheduler.NewScheduler(pool, hub, ledgerSvc, reconcileSvc)
	cronSched.Start(ctx)
	defer cronSched.Stop()

	// Verify versioned migrations schema level (Server boot only verifies schema version and fails fast if behind)
	if err := migrations.VerifySchemaVersion(ctx, pool, migrations.TargetVersion); err != nil {
		log.Fatalf("SCHEMA INTEGRITY FAILURE: %v", err)
	}

	adminH := &handlers.AdminHandler{Pool: pool, Hub: hub, LedgerSvc: ledgerSvc, FraudEngine: fraudEngine}

	guardSvc := guard.NewService(pool)
	idemSvc := idem.NewService(pool)
	limitsSvc := limits.NewService(pool)

	authH := &handlers.AuthHandler{
		Pool:            pool,
		JWTSecret:       jwtSecret,
		TokenTTL:        tokenTTL,
		SuperAdminEmail: superAdminEmail,
		LimitsSvc:       limitsSvc,
	}
	simH := &handlers.SimHandler{Pool: pool, Hub: hub}
	debtsH := &handlers.DebtsHandler{Pool: pool, Hub: hub}
	ticketsH := &handlers.TicketsHandler{Pool: pool, Hub: hub}
	backupsH := &handlers.BackupsHandler{Pool: pool, Hub: hub}
	expensesH := &handlers.ExpensesHandler{Pool: pool, Hub: hub}
	announcementsH := &handlers.AnnouncementsHandler{Pool: pool, Hub: hub}
	zainH := &handlers.ZainCashHandler{Pool: pool, Hub: hub, JWTSecret: jwtSecret, LedgerSvc: ledgerSvc, FraudEngine: fraudEngine}
	vouchersH := &handlers.VouchersHandler{
		Pool:         pool,
		Hub:          hub,
		LedgerSvc:    ledgerSvc,
		FraudEngine:  fraudEngine,
		VaultMgr:     vaultMgr,
		PricingSvc:   pricingSvc,
		HierarchySvc: hierarchySvc,
		GuardSvc:     guardSvc,
		IdemSvc:      idemSvc,
	}

	// Stage 2: Reliability (jobs, refund, sync)
	jobsSvc := jobs.NewService(pool)
	refundSvc := refund.NewService(pool, ledgerSvc)
	syncSvc := syncPkg.NewService(pool)

	jobsSvc.RegisterHandler("zaincash_poll", func(ctx context.Context, job *jobs.Job) error {
		// ZainCash poll handler checking status with backoff
		return nil
	})
	jobsSvc.Start(5 * time.Second)
	defer jobsSvc.Stop()

	jobsAndRefundH := handlers.NewJobsAndRefundHandler(jobsSvc, refundSvc)
	syncH := &handlers.SyncHandler{Pool: pool, Hub: hub, SyncSvc: syncSvc}

	subSvc := subscribers.NewService(pool, hub, jwtSecret, ledgerSvc)
	promoSvc := promotions.NewService(pool, ledgerSvc)
	appConfigSvc := appconfig.NewService(pool, hub)
	deviceRadarSvc := deviceradar.NewService(pool, hub)
	securitySvc := security.NewService(pool, ledgerSvc)
	biSvc := bi.NewService(pool)

	enterpriseH := handlers.NewEnterpriseHandler(pool, hub, subSvc, promoSvc, appConfigSvc, deviceRadarSvc, securitySvc, biSvc, guardSvc, idemSvc, limitsSvc)

	// Stage 3: Receipts, Notifications, Reports
	sealSvc := seal.NewService(pool, getenv("SEAL_HMAC_SECRET", "uruk_seal_secure_secret_2026"), getenv("SERVER_URL", "https://vapi.urukiq.com"))
	notifySvc := notify.NewService(pool, hub.Broadcast)
	reportsSvc := reports.NewService(pool, notifySvc)
	stage3H := handlers.NewStage3Handler(sealSvc, notifySvc, reportsSvc)

	// Stage 4: Growth & Governance
	rewardsSvc := rewards.NewService(pool, ledgerSvc)
	restockSvc := restock.NewService(pool, jobsSvc)
	riskSvc := risk.NewService(pool)
	forecastSvc := forecast.NewService(pool)
	tillsSvc := tills.NewService(pool)
	apiSvc := apiPkg.NewService(pool)
	surgeSvc := surge.NewService(pool)
	stage4H := handlers.NewStage4Handler(rewardsSvc, restockSvc, riskSvc, forecastSvc, tillsSvc, apiSvc, surgeSvc)

	app := fiber.New(fiber.Config{
		AppName:      "VENUS POS Backend",
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	})
	app.Use(compress.New(compress.Config{
		Level: compress.LevelBestSpeed,
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins:     "*",
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, X-User-Id, X-User-Email, X-Requested-With, Accept-Language, Idempotency-Key, X-Idempotency-Key",
		ExposeHeaders:    "Content-Length, Content-Type, X-Idempotent-Replay",
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
	app.Get("/healthz", enterpriseH.Healthz)
	app.Get("/readyz", enterpriseH.Readyz)
	app.Get("/verify/:token", stage3H.VerifyReceipt)
	app.Get("/forecast", stage4H.GetForecast)

	// Realtime WebSocket endpoint (JWT via ?token= or Authorization header).
	app.Get("/ws", hub.Handler(jwtSecret))

	api := app.Group("/api")
	api.Use(limitsSvc.Middleware(120, 4.0))

	// Public app config & banners
	api.Get("/app-config", enterpriseH.GetPublicAppConfig)
	api.Get("/banners", enterpriseH.GetPublicAppConfig)

	// Public auth endpoints.
	api.Post("/auth/login", authH.Login)
	api.Post("/auth/register", authH.Register)

	// Unified authoritative sync state (public fallback to UID 1 or header-based UID).
	api.Get("/sync/state", syncH.GetSyncState)
	api.Get("/subscriptions/plans", authH.GetSubscriptionPlans)
	api.Post("/subscriptions/redeem-code", authH.RedeemActivationCode)

	// Public ZainCash browser, webhook, and payment endpoints (NO Authorization header required).
	api.Post("/payments/zaincash/init", zainH.InitZainCash)
	api.Post("/payments/zaincash/confirm", zainH.ConfirmInAppZainCash)
	api.Get("/payments/zaincash/inquiry/:transactionId", zainH.InquiryZainCash)
	api.Get("/zaincash/checkout", zainH.CheckoutZainCash)
	api.Post("/zaincash/checkout/confirm", zainH.ConfirmCheckoutZainCash)
	api.Get("/zaincash/redirect", zainH.RedirectZainCash)
	api.Post("/zaincash/webhook", zainH.WebhookZainCash)
	api.Get("/docs/openapi.json", stage4H.GetOpenAPISpec)
	api.Get("/surge/quote", stage4H.GetSurgeQuote)

	idempotencyMw := middleware.Idempotency(pool)

	// Authenticated subscriber area.
	auth := api.Group("", middleware.JWT(jwtSecret))
	auth.Get("/auth/me", authH.Me)
	auth.Put("/auth/credentials", authH.ChangeCredentials)
	auth.Get("/sync/state", syncH.GetSyncState)
	auth.Get("/sync", syncH.GetDelta)
	auth.Post("/refund", idempotencyMw, jobsAndRefundH.PostRefund)
	auth.Post("/seal/create", stage3H.CreateSealToken)
	auth.Get("/forecast", stage4H.GetForecast)
	auth.Get("/surge/quote", stage4H.GetSurgeQuote)
	auth.Get("/tills/active", stage4H.GetActiveShift)
	auth.Post("/tills/open", stage4H.OpenShift)
	auth.Post("/tills/close", stage4H.CloseShift)
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
	auth.Post("/vouchers/dispense", idempotencyMw, vouchersH.DispenseVoucher)
	auth.Get("/pricing/my-price/:productId", fintechH.ResolveSubscriberPrice)

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
	admin.Post("/users/:id/wallet", idempotencyMw, adminH.AdminAdjustWallet)
	admin.Post("/wallet/credit", idempotencyMw, adminH.AdminAdjustWallet)
	admin.Get("/users/:id/backup", backupsH.Backup)
	admin.Post("/users/:id/restore", backupsH.Restore)
	admin.Get("/tickets", ticketsH.ListAllTickets)
	admin.Put("/tickets/:id/reply", ticketsH.ReplyTicket)
	admin.Post("/announcements", announcementsH.SaveAnnouncement)
	admin.Delete("/announcements/:id", announcementsH.DeleteAnnouncement)

	// Phase 1 Fintech Core API endpoints
	fintech := admin.Group("/fintech")
	fintech.Get("/ledger", fintechH.GetLedgerEntries)
	fintech.Get("/integrity/alerts", fintechH.ListIntegrityAlerts)
	fintech.Post("/integrity/verify", fintechH.VerifyIntegrityNow)
	fintech.Get("/fraud/rules", fintechH.ListFraudRules)
	fintech.Put("/fraud/rules/:id", fintechH.UpdateFraudRule)
	fintech.Get("/fraud/events", fintechH.ListFraudEvents)
	fintech.Post("/wallets/:id/unfreeze", fintechH.UnfreezeWallet)
	fintech.Post("/wallets/:id/credit-limit", fintechH.SetCreditLimit)
	fintech.Get("/wallets/debts", fintechH.ListAgentDebts)
	fintech.Post("/reconcile/run", fintechH.RunReconciliation)
	fintech.Get("/reconcile/reports", fintechH.ListReconciliationReports)
	fintech.Get("/reconcile/reports/:id/export", fintechH.ExportReconciliationReportCSV)
	fintech.Post("/close/seal", fintechH.SealFinancialClose)
	fintech.Get("/close/history", fintechH.ListFinancialCloses)
	fintech.Get("/statements/:id", fintechH.GetAccountStatement)

	// Phase 2: Pricing Tiers & Custom Overrides
	admin.Get("/pricing/tiers", fintechH.ListPriceTiers)
	admin.Post("/pricing/tiers/:id/prices", fintechH.SetTierPrice)
	admin.Get("/pricing/overrides", fintechH.ListSubscriberPriceOverrides)
	admin.Post("/pricing/subscribers/:id/overrides", fintechH.SetSubscriberPriceOverride)
	admin.Delete("/pricing/subscribers/:id/overrides/:productId", fintechH.RemoveSubscriberPriceOverride)
	admin.Get("/pricing/resolve/:userId/:productId", fintechH.ResolveSubscriberPrice)

	// Phase 2: Agent Hierarchy & Commissions
	admin.Post("/hierarchy/users/:id/parent", fintechH.SetParentAgent)
	admin.Post("/hierarchy/agents/:id/commissions", fintechH.SetAgentCommission)
	admin.Get("/hierarchy/commissions", fintechH.ListAgentCommissions)
	admin.Get("/hierarchy/agents/:id/report", fintechH.GetAgentCommissionReport)

	// Phase 2: Inventory Radar & Thresholds
	admin.Get("/radar/summary", fintechH.GetInventoryRadar)
	admin.Post("/radar/thresholds", fintechH.SetCategoryThreshold)
	admin.Post("/vault/rotate-keys", fintechH.RotateVaultKeys)

	// Admin Activation Codes management
	admin.Post("/activation-codes/generate", adminH.GenerateActivationCodes)
	admin.Get("/activation-codes", adminH.ListActivationCodes)
	admin.Put("/subscriptions/plans", authH.UpdateSubscriptionPlans)
	admin.Post("/users/:id/subscription", adminH.AdminManageSubscription)

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
	auth.Get("/coupons/validate", enterpriseH.ValidateCoupon)
	auth.Post("/commands/ack", enterpriseH.AckRemoteCommand)

	// Phase 3: CRM & Lifecycle
	admin.Get("/subscribers/:id/360", enterpriseH.GetSubscriber360)
	admin.Post("/subscribers/bulk/preview", enterpriseH.BulkSubscribersPreview)
	admin.Post("/subscribers/bulk/execute", enterpriseH.BulkSubscribersExecute)
	admin.Post("/users/:id/lock-device", enterpriseH.LockDevice)
	admin.Post("/users/:id/reset-device", enterpriseH.ResetDevice)
	admin.Post("/users/:id/kick-sessions", enterpriseH.KickSessions)
	admin.Post("/users/:id/impersonate", enterpriseH.ImpersonateSubscriber)

	// Phase 4: Promotions & Coupons
	admin.Get("/coupons", enterpriseH.ListCoupons)
	admin.Post("/coupons", enterpriseH.CreateCoupon)

	// Phase 5: Remote App Config & Banners
	admin.Get("/app-config", enterpriseH.GetAllAppConfigs)
	admin.Put("/app-config/:key", enterpriseH.UpdateAppConfig)
	admin.Get("/banners", enterpriseH.ListBanners)
	admin.Post("/banners", enterpriseH.SaveBanner)
	admin.Delete("/banners/:id", enterpriseH.DeleteBanner)

	// Phase 6: Live Radar & Remote Commands
	admin.Get("/radar/online-devices", enterpriseH.GetOnlineDevices)
	admin.Post("/commands/queue", enterpriseH.QueueRemoteCommand)

	// Phase 7: Rollback, WAF, API Keys
	admin.Post("/security/rollback", enterpriseH.RollbackOperation)
	admin.Get("/security/ip-rules", enterpriseH.ListIPRules)
	admin.Post("/security/ip-rules", enterpriseH.SaveIPRule)
	admin.Delete("/security/ip-rules/:id", enterpriseH.DeleteIPRule)
	admin.Get("/security/api-keys", enterpriseH.ListAPIKeys)
	admin.Post("/security/api-keys", enterpriseH.GenerateAPIKey)

	// Phase 8: BI Analytics, Backups, Streaming Export
	admin.Get("/bi/dashboard", enterpriseH.GetBIDashboard)
	admin.Get("/bi/backup", enterpriseH.DownloadSecureBackup)
	admin.Get("/export/subscribers", enterpriseH.ExportSubscribersCSV)

	// Stage 1: Financial Security (Guard & Limits)
	admin.Get("/guard/settings", enterpriseH.ListGuardSettings)
	admin.Post("/guard/settings", enterpriseH.SaveGuardSetting)
	admin.Delete("/guard/settings/:item_id", enterpriseH.DeleteGuardSetting)
	admin.Get("/limits/blocks", enterpriseH.ListLimitsBlocks)
	admin.Post("/limits/unblock/:id", enterpriseH.UnblockLimit)
	admin.Post("/limits/block", enterpriseH.BlockLimitManual)

	// Stage 2: Reliability (Jobs & Dead Queue)
	admin.Get("/jobs/dead", jobsAndRefundH.ListDeadJobs)
	admin.Post("/jobs/:id/retry", jobsAndRefundH.RetryDeadJob)

	// Stage 3: Receipts, Notifications, Reports
	admin.Get("/notify/channels", stage3H.ListNotifyChannels)
	admin.Post("/notify/channels", stage3H.SaveNotifyChannel)
	admin.Delete("/notify/channels/:id", stage3H.DeleteNotifyChannel)
	admin.Post("/reports/send-now", stage3H.SendReportsNow)

	// Stage 4: Growth & Governance (Rewards, Restock, Risk, Forecast, Tills, API Keys, Surge)
	admin.Get("/forecast", stage4H.GetForecast)
	admin.Get("/rewards/rules", stage4H.ListRewardRules)
	admin.Post("/rewards/rules", stage4H.SaveRewardRule)
	admin.Post("/rewards/evaluate/:merchant_id/:rule_id", stage4H.EvaluateReward)
	admin.Get("/restock/orders", stage4H.ListRestockOrders)
	admin.Post("/restock/order", stage4H.TriggerRestock)
	admin.Get("/risk/profiles", stage4H.ListRiskProfiles)
	admin.Post("/risk/override/:merchant_id", stage4H.OverrideRisk)
	admin.Get("/surge/config", stage4H.GetSurgeConfig)
	admin.Post("/surge/config", stage4H.UpdateSurgeConfig)
	admin.Get("/api-keys-v2", stage4H.ListApiKeys)
	admin.Post("/api-keys-v2", stage4H.GenerateApiKey)
	admin.Delete("/api-keys-v2/:id", stage4H.RevokeApiKey)

	// R5B: domain-isolated web console hosting:
	// - console.urukiq.com: serves Native Go Admin Console (embedded console.html)
	// - vapi.urukiq.com: API only (returns status JSON on /, 404 on /console)
	adminDist := getenv("ADMIN_DIST_DIR", "")
	registerConsoleRoutes(app, adminDist)
	log.Printf("domain-isolated routing enabled (console.urukiq.com -> embedded Native Go Console, override: %s)", adminDist)

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
