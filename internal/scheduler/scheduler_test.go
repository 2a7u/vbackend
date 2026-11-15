package scheduler_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"venus_backend/internal/scheduler"
	"venus_backend/internal/testhelper"
)

func TestSchedulerAdvisoryLock(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		return
	}

	ctx := context.Background()
	lockKey := int64(scheduler.AdvisoryLockDailyClose)

	// Acquire a dedicated connection for testing session-level advisory locks
	conn1, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire conn1 failed: %v", err)
	}
	defer conn1.Release()

	conn2, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire conn2 failed: %v", err)
	}
	defer conn2.Release()

	// 1. conn1 acquires lock
	var acquired1 bool
	err = conn1.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, lockKey).Scan(&acquired1)
	if err != nil || !acquired1 {
		t.Fatalf("expected conn1 to acquire lock, got err=%v, acquired=%v", err, acquired1)
	}

	// 2. conn2 attempts to acquire same lock: MUST return false
	var acquired2 bool
	err = conn2.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, lockKey).Scan(&acquired2)
	if err != nil {
		t.Fatalf("conn2 query error: %v", err)
	}
	if acquired2 {
		t.Fatalf("expected conn2 lock acquisition to FAIL while conn1 holds it, but got true")
	}

	// 3. conn1 releases lock
	var released bool
	err = conn1.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, lockKey).Scan(&released)
	if err != nil || !released {
		t.Fatalf("expected conn1 to release lock, got err=%v, released=%v", err, released)
	}

	// 4. conn2 re-attempts: MUST succeed now
	err = conn2.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, lockKey).Scan(&acquired2)
	if err != nil || !acquired2 {
		t.Fatalf("expected conn2 to acquire lock after release, got err=%v, acquired=%v", err, acquired2)
	}

	// Clean up conn2
	_, _ = conn2.Exec(ctx, `SELECT pg_advisory_unlock($1)`, lockKey)
}

func TestSchedulerMultiInstanceJobExclusion(t *testing.T) {
	pool, cleanup := testhelper.GetTestPool(t)
	defer cleanup()
	if pool == nil {
		return
	}

	ctx := context.Background()
	lockKey := int64(scheduler.AdvisoryLockDailyReconcile)

	var jobExecutionCount atomic.Int64
	var attempts atomic.Int64
	var wg sync.WaitGroup
	startSignal := make(chan struct{})

	// Simulate 10 distributed workers simultaneously trying to run the daily financial close
	const workers = 10
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startSignal // Wait for all workers to be ready so they contend simultaneously

			conn, err := pool.Acquire(ctx)
			if err != nil {
				return
			}
			defer conn.Release()

			var acquired bool
			err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, lockKey).Scan(&acquired)
			attempts.Add(1)

			if err == nil && acquired {
				defer func() {
					_, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, lockKey)
				}()

				// Exactly one worker enters here
				jobExecutionCount.Add(1)

				// Hold lock until all other workers have attempted
				for attempts.Load() < int64(workers) {
					time.Sleep(10 * time.Millisecond)
				}
			}
		}()
	}

	// Trigger all workers at once
	close(startSignal)
	wg.Wait()

	if jobExecutionCount.Load() != 1 {
		t.Fatalf("Multi-instance exclusion failed: job executed %d times, expected exactly 1", jobExecutionCount.Load())
	}
}
