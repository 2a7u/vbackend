package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"venus_backend/internal/ledger"
	"venus_backend/internal/reconcile"
)

type Broadcaster interface {
	Broadcast(userID int64, event string, payload any)
}

type Scheduler struct {
	Pool         *pgxpool.Pool
	Hub          Broadcaster
	LedgerSvc    *ledger.Service
	ReconcileSvc *reconcile.Service
	Loc          *time.Location

	stopCh chan struct{}
	wg     sync.WaitGroup

	lastDailyReconcileDay string
	lastDailyCloseDay     string
	lastMonthlyCloseMonth string
	lastIntegrityDay      string
}

func NewScheduler(pool *pgxpool.Pool, hub Broadcaster, ledgerSvc *ledger.Service, reconcileSvc *reconcile.Service) *Scheduler {
	loc, err := time.LoadLocation("Asia/Baghdad")
	if err != nil {
		loc = time.FixedZone("UTC+3", 3*3600)
	}
	return &Scheduler{
		Pool:         pool,
		Hub:          hub,
		LedgerSvc:    ledgerSvc,
		ReconcileSvc: reconcileSvc,
		Loc:          loc,
		stopCh:       make(chan struct{}),
	}
}

// Start launches the background scheduler checking cron schedules every minute.
func (s *Scheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		log.Println("[Scheduler] Automated financial cron scheduler started (Asia/Baghdad UTC+3)")

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("[Scheduler] Shutting down scheduler...")
				return
			case <-s.stopCh:
				log.Println("[Scheduler] Stopped scheduler.")
				return
			case <-ticker.C:
				s.tick(ctx)
			}
		}
	}()
}

// Stop stops the scheduler and waits for in-flight jobs to drain.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *Scheduler) tick(ctx context.Context) {
	nowBaghdad := time.Now().In(s.Loc)
	todayStr := nowBaghdad.Format("2006-01-02")
	thisMonthStr := nowBaghdad.Format("2006-01")
	hour := nowBaghdad.Hour()
	min := nowBaghdad.Minute()

	// 1. Daily Integrity Check at 04:00 AM Baghdad time
	if hour == 4 && min == 0 && s.lastIntegrityDay != todayStr {
		s.lastIntegrityDay = todayStr
		log.Printf("[Scheduler] Running daily automated ledger integrity check for %s...", todayStr)
		go func() {
			summary, err := ledger.VerifyGlobalIntegrity(ctx, s.Pool, s.Hub)
			if err != nil {
				log.Printf("[Scheduler] Daily integrity check error: %v", err)
			} else if !summary.IsClean {
				log.Printf("[Scheduler] ALERT: Daily integrity check detected %d violations!", summary.ViolationsCount)
			} else {
				log.Printf("[Scheduler] Daily integrity check clean: %d entries verified across %d accounts.", summary.TotalEntriesChecked, summary.AccountsChecked)
			}
		}()
	}

	// 2. Daily Reconciliation at 23:59:00 Baghdad time
	if hour == 23 && min == 59 && s.lastDailyReconcileDay != todayStr {
		s.lastDailyReconcileDay = todayStr
		log.Printf("[Scheduler] Running daily automated reconciliation for %s...", todayStr)
		go func() {
			rep, err := s.ReconcileSvc.RunDailyReconciliation(ctx, nowBaghdad, "system_automated_cron")
			if err != nil {
				log.Printf("[Scheduler] Daily reconciliation error: %v", err)
			} else {
				log.Printf("[Scheduler] Daily reconciliation %s: discrepancy = %d IQD (%s)", rep.ReportDate, rep.Discrepancy, rep.Status)
				if rep.Status != "matched" && s.Hub != nil {
					s.Hub.Broadcast(0, "reconciliation_alert", rep)
				}
			}
		}()
	}

	// 3. Daily Financial Close at 23:59:30 Baghdad time
	if hour == 23 && min == 59 && nowBaghdad.Second() >= 30 && s.lastDailyCloseDay != todayStr {
		s.lastDailyCloseDay = todayStr
		log.Printf("[Scheduler] Running daily automated financial close for %s...", todayStr)
		go func() {
			closeRec, err := s.ReconcileSvc.SealFinancialClose(ctx, "daily", nowBaghdad, 0)
			if err != nil {
				log.Printf("[Scheduler] Daily financial close error: %v", err)
			} else {
				log.Printf("[Scheduler] Daily financial close sealed: %s, hash=%s", closeRec.PeriodDate, closeRec.ClosingHash[:16])
			}
		}()
	}

	// 4. Monthly Financial Close on Day 1 at 00:05 AM for preceding month
	if nowBaghdad.Day() == 1 && hour == 0 && min == 5 && s.lastMonthlyCloseMonth != thisMonthStr {
		s.lastMonthlyCloseMonth = thisMonthStr
		lastMonthDate := nowBaghdad.AddDate(0, -1, 0)
		log.Printf("[Scheduler] Running monthly automated financial close for %s...", lastMonthDate.Format("2006-01"))
		go func() {
			closeRec, err := s.ReconcileSvc.SealFinancialClose(ctx, "monthly", lastMonthDate, 0)
			if err != nil {
				log.Printf("[Scheduler] Monthly financial close error: %v", err)
			} else {
				log.Printf("[Scheduler] Monthly financial close sealed: %s, hash=%s", closeRec.PeriodDate, closeRec.ClosingHash[:16])
			}
		}()
	}
}

// Advisory lock keys for multi-node cron coordination
const (
	AdvisoryLockIntegrityCheck = 900101
	AdvisoryLockDailyReconcile = 900102
	AdvisoryLockDailyClose     = 900103
	AdvisoryLockMonthlyClose   = 900104
)

// TryAcquireAdvisoryLock attempts to acquire a PostgreSQL session advisory lock.
// Returns true if the lock was successfully acquired, or false if another node is holding it.
func TryAcquireAdvisoryLock(ctx context.Context, pool *pgxpool.Pool, lockKey int64) (bool, error) {
	if pool == nil {
		return true, nil
	}
	var acquired bool
	err := pool.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, lockKey).Scan(&acquired)
	return acquired, err
}

// ReleaseAdvisoryLock releases a PostgreSQL session advisory lock.
func ReleaseAdvisoryLock(ctx context.Context, pool *pgxpool.Pool, lockKey int64) error {
	if pool == nil {
		return nil
	}
	var released bool
	return pool.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, lockKey).Scan(&released)
}

