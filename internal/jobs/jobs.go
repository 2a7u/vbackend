package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrJobNotFound = errors.New("job not found")
	ErrJobNotDead  = errors.New("job is not in dead status")
)

type Job struct {
	ID          int64           `json:"id"`
	MerchantID  int64           `json:"merchant_id"`
	Kind        string          `json:"kind"`
	Payload     json.RawMessage `json:"payload"`
	Status      string          `json:"status"` // pending, processing, completed, failed, dead
	Attempts    int             `json:"attempts"`
	MaxAttempts int             `json:"max_attempts"`
	NextAt      time.Time       `json:"next_at"`
	LastError   string          `json:"last_error"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type HandlerFunc func(ctx context.Context, job *Job) error

type Service struct {
	pool      *pgxpool.Pool
	handlers  map[string]HandlerFunc
	mu        sync.RWMutex
	stopChan  chan struct{}
	wg        sync.WaitGroup
	isRunning bool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool:     pool,
		handlers: make(map[string]HandlerFunc),
		stopChan: make(chan struct{}),
	}
}

// RegisterHandler registers an execution handler for a specific job kind.
func (s *Service) RegisterHandler(kind string, fn HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[kind] = fn
}

// Enqueue inserts a new job into the queue.
func (s *Service) Enqueue(ctx context.Context, merchantID int64, kind string, payload interface{}, maxAttempts int) (int64, error) {
	if s.pool == nil {
		return 0, nil
	}
	if maxAttempts <= 0 {
		maxAttempts = 10
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("marshal payload: %w", err)
	}

	query := `
		INSERT INTO jobs (merchant_id, kind, payload, status, attempts, max_attempts, next_at, created_at, updated_at)
		VALUES ($1, $2, $3, 'pending', 0, $4, now(), now(), now())
		RETURNING id;
	`
	var id int64
	err = s.pool.QueryRow(ctx, query, merchantID, kind, payloadBytes, maxAttempts).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("enqueue job: %w", err)
	}
	return id, nil
}

// Start launches the worker background loop.
func (s *Service) Start(pollInterval time.Duration) {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return
	}
	s.isRunning = true
	s.stopChan = make(chan struct{})
	s.mu.Unlock()

	s.wg.Add(1)
	go s.workerLoop(pollInterval)
}

// Stop gracefully terminates the background worker loop.
func (s *Service) Stop() {
	s.mu.Lock()
	if !s.isRunning {
		s.mu.Unlock()
		return
	}
	s.isRunning = false
	close(s.stopChan)
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Service) workerLoop(pollInterval time.Duration) {
	defer s.wg.Done()
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.ProcessBatch(context.Background(), 10)
		}
	}
}

// ProcessBatch fetches and processes up to limit pending jobs using SELECT ... FOR UPDATE SKIP LOCKED.
func (s *Service) ProcessBatch(ctx context.Context, limit int) (int, error) {
	if s.pool == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 10
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin worker tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Fetch lockable jobs
	query := `
		SELECT id, merchant_id, kind, payload, status, attempts, max_attempts, next_at, last_error, created_at, updated_at
		FROM jobs
		WHERE status = 'pending' AND next_at <= now()
		ORDER BY id ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED;
	`
	rows, err := tx.Query(ctx, query, limit)
	if err != nil {
		return 0, fmt.Errorf("fetch pending jobs: %w", err)
	}
	defer rows.Close()

	var jobsList []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(
			&j.ID, &j.MerchantID, &j.Kind, &j.Payload, &j.Status,
			&j.Attempts, &j.MaxAttempts, &j.NextAt, &j.LastError,
			&j.CreatedAt, &j.UpdatedAt,
		); err != nil {
			return 0, fmt.Errorf("scan job: %w", err)
		}
		jobsList = append(jobsList, j)
	}
	rows.Close()

	if len(jobsList) == 0 {
		return 0, nil
	}

	// Mark them as processing in this tx
	for _, j := range jobsList {
		_, _ = tx.Exec(ctx, "UPDATE jobs SET status = 'processing', updated_at = now() WHERE id = $1", j.ID)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit processing status: %w", err)
	}

	// Execute each job outside the lock transaction
	processedCount := 0
	for _, j := range jobsList {
		s.executeJob(ctx, j)
		processedCount++
	}

	return processedCount, nil
}

func (s *Service) executeJob(ctx context.Context, job Job) {
	s.mu.RLock()
	handler, exists := s.handlers[job.Kind]
	s.mu.RUnlock()

	var execErr error
	if !exists {
		execErr = fmt.Errorf("no handler registered for job kind: %s", job.Kind)
	} else {
		// Run handler with timeout
		jobCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		execErr = handler(jobCtx, &job)
		cancel()
	}

	now := time.Now().UTC()
	attempts := job.Attempts + 1

	if execErr == nil {
		// Completed successfully
		query := `
			UPDATE jobs
			SET status = 'completed', attempts = $1, updated_at = $2
			WHERE id = $3;
		`
		_, _ = s.pool.Exec(ctx, query, attempts, now, job.ID)
		return
	}

	// Job failed: check if it should become dead or retry with backoff
	errStr := execErr.Error()
	if attempts >= job.MaxAttempts {
		// Moved to dead queue
		query := `
			UPDATE jobs
			SET status = 'dead', attempts = $1, last_error = $2, updated_at = $3
			WHERE id = $4;
		`
		_, _ = s.pool.Exec(ctx, query, attempts, errStr, now, job.ID)
		// Record audit alert for dead job
		s.recordDeadJobAlert(ctx, job.MerchantID, job.ID, job.Kind, errStr)
	} else {
		// Backoff: 30s * attempts
		backoffDuration := time.Duration(30*attempts) * time.Second
		nextAt := now.Add(backoffDuration)
		query := `
			UPDATE jobs
			SET status = 'pending', attempts = $1, next_at = $2, last_error = $3, updated_at = $4
			WHERE id = $5;
		`
		_, _ = s.pool.Exec(ctx, query, attempts, nextAt, errStr, now, job.ID)
	}
}

func (s *Service) recordDeadJobAlert(ctx context.Context, merchantID, jobID int64, kind, errMsg string) {
	if s.pool == nil {
		return
	}
	details, _ := json.Marshal(map[string]interface{}{
		"job_id": jobID,
		"kind":   kind,
		"error":  errMsg,
		"alert":  "JOB_MOVED_TO_DEAD_QUEUE",
	})
	query := `
		INSERT INTO audit_log (user_id, action, target_type, target_id, details, ip_address, created_at)
		VALUES ($1, 'JOB_DEAD_ALERT', 'job', $2, $3, '127.0.0.1', now());
	`
	_, _ = s.pool.Exec(ctx, query, merchantID, fmt.Sprintf("%d", jobID), details)
}

// ListDeadJobs retrieves dead jobs for the admin console.
func (s *Service) ListDeadJobs(ctx context.Context, limit, offset int) ([]Job, error) {
	if s.pool == nil {
		return []Job{}, nil
	}
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, merchant_id, kind, payload, status, attempts, max_attempts, next_at, last_error, created_at, updated_at
		FROM jobs
		WHERE status = 'dead'
		ORDER BY id DESC
		LIMIT $1 OFFSET $2;
	`
	rows, err := s.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list dead jobs: %w", err)
	}
	defer rows.Close()

	var list []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(
			&j.ID, &j.MerchantID, &j.Kind, &j.Payload, &j.Status,
			&j.Attempts, &j.MaxAttempts, &j.NextAt, &j.LastError,
			&j.CreatedAt, &j.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan dead job: %w", err)
		}
		list = append(list, j)
	}
	return list, nil
}

// RetryJob resets a dead job to pending with 0 attempts and next_at = now().
func (s *Service) RetryJob(ctx context.Context, jobID int64) error {
	if s.pool == nil {
		return nil
	}

	query := `
		UPDATE jobs
		SET status = 'pending', attempts = 0, next_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'dead';
	`
	cmd, err := s.pool.Exec(ctx, query, jobID)
	if err != nil {
		return fmt.Errorf("retry dead job: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrJobNotDead
	}
	return nil
}

// CountDeadJobs returns total number of dead jobs.
func (s *Service) CountDeadJobs(ctx context.Context) (int64, error) {
	if s.pool == nil {
		return 0, nil
	}
	var count int64
	err := s.pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE status = 'dead';").Scan(&count)
	return count, err
}
