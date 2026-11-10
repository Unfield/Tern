// Package jobs runs background work: durable jobs with retries, stored in
// SQLite or PostgreSQL, and recurring schedules. Several instances of an
// app can share one database; each job and each scheduled run happens once.
//
//	q, err := jobs.New(db, jobs.Options{})
//	sendWelcome := jobs.Handle(q, "email.welcome", func(ctx context.Context, u User) error {
//		return mailer.Send(ctx, u.Email, "Welcome!")
//	}, jobs.MaxAttempts(5))
//	q.Schedule("cleanup", "0 3 * * *", func(ctx context.Context) error {
//		return deleteExpiredSessions(ctx, db)
//	})
//	app, err := ternApp(kit.WithPlugins(q)) // workers run while the server does
//
//	// In an action:
//	_, err := sendWelcome.Enqueue(ctx, user)
//
// Handlers receive the payload decoded from JSON. A handler that returns an
// error or panics is retried with exponential backoff until it has run
// MaxAttempts times. Finished jobs are kept for a day (Options.Retention)
// so they can be inspected with List and Stats.
package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/sqldb"
)

// Options configure a Queue.
type Options struct {
	// Workers is how many jobs run at once. The default is 4.
	Workers int
	// PollInterval is how often idle workers look for jobs enqueued by
	// other instances. Jobs enqueued in this process start at once. The
	// default is one second.
	PollInterval time.Duration
	// Retention is how long finished and failed jobs are kept. The default
	// is 24 hours.
	Retention time.Duration
	// Location is the time zone of cron schedules. The default is UTC.
	Location *time.Location
	// Logger receives job failures. The default is the app's logger.
	Logger *slog.Logger
}

// State is where a job is in its life.
type State string

// The states of a job.
const (
	Queued  State = "queued"  // waiting to run, possibly for a retry
	Running State = "running" // claimed by a worker
	Done    State = "done"    // finished successfully
	Failed  State = "failed"  // ran out of attempts
)

// Job is a job as stored, for inspection.
type Job struct {
	ID          int64           `json:"id"`
	Kind        string          `json:"kind"`
	Payload     json.RawMessage `json:"payload"`
	State       State           `json:"state"`
	Attempts    int             `json:"attempts"`
	MaxAttempts int             `json:"maxAttempts"`
	LastError   string          `json:"lastError,omitempty"`
	RunAt       time.Time       `json:"runAt"`
	CreatedAt   time.Time       `json:"createdAt"`
	FinishedAt  *time.Time      `json:"finishedAt,omitempty"`
}

// Queue stores and runs jobs. It is a kit.Plugin: added to an app, its
// tables are created when the server starts, and its workers run until the
// server shuts down. Without an app, call Start and Stop.
type Queue struct {
	db      *sql.DB
	d       sqldb.Dialect
	opts    Options
	log     *slog.Logger
	mu      sync.RWMutex
	kinds   map[string]*handler
	sched   []*schedule
	wake    chan struct{}
	stop    context.CancelFunc // stops claiming jobs
	abort   context.CancelFunc // cancels running jobs
	jobCtx  context.Context
	running sync.WaitGroup
	started bool
}

type handler struct {
	run         func(ctx context.Context, payload []byte) error
	maxAttempts int
	timeout     time.Duration
}

// New returns a queue that stores jobs in db.
func New(db *sql.DB, opts Options) (*Queue, error) {
	d, err := sqldb.Detect(db)
	if err != nil {
		return nil, err
	}
	if opts.Workers <= 0 {
		opts.Workers = 4
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = time.Second
	}
	if opts.Retention <= 0 {
		opts.Retention = 24 * time.Hour
	}
	if opts.Location == nil {
		opts.Location = time.UTC
	}
	return &Queue{db: db, d: d, opts: opts, log: opts.Logger, kinds: map[string]*handler{}, wake: make(chan struct{}, 1)}, nil
}

// HandleOption configures a job kind.
type HandleOption func(*handler)

// MaxAttempts is how many times a job runs before it fails for good. The
// default is 3.
func MaxAttempts(n int) HandleOption { return func(h *handler) { h.maxAttempts = max(1, n) } }

// Timeout bounds one run of a job; its context is canceled after it. The
// default is 5 minutes.
func Timeout(d time.Duration) HandleOption { return func(h *handler) { h.timeout = d } }

// Kind is a registered job kind with a typed payload.
type Kind[T any] struct {
	q    *Queue
	name string
}

// Handle registers fn to run jobs of kind name, and returns the kind for
// enqueueing them. Register every kind before the queue starts.
func Handle[T any](q *Queue, name string, fn func(ctx context.Context, payload T) error, opts ...HandleOption) *Kind[T] {
	h := &handler{maxAttempts: 3, timeout: 5 * time.Minute}
	for _, o := range opts {
		o(h)
	}
	h.run = func(ctx context.Context, raw []byte) error {
		var p T
		if err := json.Unmarshal(raw, &p); err != nil {
			return fmt.Errorf("decoding the payload: %w", err)
		}
		return fn(ctx, p)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, dup := q.kinds[name]; dup {
		panic(fmt.Sprintf("jobs: kind %q registered twice", name))
	}
	q.kinds[name] = h
	return &Kind[T]{q: q, name: name}
}

// EnqueueOption configures one job.
type EnqueueOption func(*enqueue)

type enqueue struct {
	runAt  time.Time
	unique string
	tx     *sql.Tx
}

// Delay runs the job after d.
func Delay(d time.Duration) EnqueueOption { return func(e *enqueue) { e.runAt = time.Now().Add(d) } }

// At runs the job at t.
func At(t time.Time) EnqueueOption { return func(e *enqueue) { e.runAt = t } }

// Unique skips enqueueing if a job with the same key is queued or running,
// for example "reminder:42". Enqueue then returns 0 and no error.
func Unique(key string) EnqueueOption { return func(e *enqueue) { e.unique = key } }

// InTx enqueues the job in tx, so it only exists if tx commits.
func InTx(tx *sql.Tx) EnqueueOption { return func(e *enqueue) { e.tx = tx } }

// Enqueue stores a job and returns its ID.
func (k *Kind[T]) Enqueue(ctx context.Context, payload T, opts ...EnqueueOption) (int64, error) {
	return k.q.enqueue(ctx, k.name, payload, opts...)
}

func (q *Queue) enqueue(ctx context.Context, kind string, payload any, opts ...EnqueueOption) (int64, error) {
	e := enqueue{runAt: time.Now()}
	for _, o := range opts {
		o(&e)
	}
	q.mu.RLock()
	h := q.kinds[kind]
	q.mu.RUnlock()
	if h == nil {
		return 0, fmt.Errorf("jobs: unknown kind %q", kind)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("jobs: encoding the payload: %w", err)
	}
	var unique any
	if e.unique != "" {
		unique = e.unique
	}
	now := time.Now().UnixMilli()
	query := q.d.Rebind(`INSERT INTO tern_jobs (kind, payload, state, attempts, max_attempts, run_at, created_at, unique_key)
		VALUES (?, ?, 'queued', 0, ?, ?, ?, ?) ON CONFLICT DO NOTHING RETURNING id`)
	var row *sql.Row
	if e.tx != nil {
		row = e.tx.QueryRowContext(ctx, query, kind, string(raw), h.maxAttempts, e.runAt.UnixMilli(), now, unique)
	} else {
		row = q.db.QueryRowContext(ctx, query, kind, string(raw), h.maxAttempts, e.runAt.UnixMilli(), now, unique)
	}
	var id int64
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil // a unique job with this key is already queued
		}
		return 0, fmt.Errorf("jobs: enqueue: %w", err)
	}
	if e.tx == nil && !e.runAt.After(time.Now()) {
		q.notify()
	}
	return id, nil
}

func (q *Queue) notify() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// Migrate creates the queue's tables if they don't exist. Start calls it.
func (q *Queue) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS tern_jobs (
			id ` + q.d.AutoID() + `,
			kind TEXT NOT NULL,
			payload TEXT NOT NULL,
			state TEXT NOT NULL,
			attempts INTEGER NOT NULL,
			max_attempts INTEGER NOT NULL,
			last_error TEXT,
			run_at BIGINT NOT NULL,
			locked_until BIGINT,
			created_at BIGINT NOT NULL,
			finished_at BIGINT,
			unique_key TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS tern_jobs_ready ON tern_jobs (state, run_at)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS tern_jobs_unique ON tern_jobs (unique_key) WHERE unique_key IS NOT NULL AND state IN ('queued', 'running')`,
		`CREATE TABLE IF NOT EXISTS tern_schedules (
			name TEXT PRIMARY KEY,
			next_run BIGINT NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := q.db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("jobs: creating tables: %w", err)
		}
	}
	return nil
}

// Name implements kit.Plugin.
func (q *Queue) Name() string { return "jobs" }

// Setup implements kit.Plugin.
func (q *Queue) Setup(s *kit.Setup) error {
	if q.log == nil {
		q.log = s.Logger()
	}
	s.OnStart(q.Start)
	s.OnShutdown(func(ctx context.Context) error { return q.Stop(ctx) })
	return nil
}

// Start creates the tables and runs the workers and schedules until ctx is
// canceled or Stop is called.
func (q *Queue) Start(ctx context.Context) error {
	if q.log == nil {
		q.log = slog.Default()
	}
	if err := q.Migrate(ctx); err != nil {
		return err
	}
	q.mu.Lock()
	if q.started {
		q.mu.Unlock()
		return errors.New("jobs: the queue is already running")
	}
	q.started = true
	// Workers stop claiming when ctx ends or Stop is called; jobs already
	// running get until Stop's deadline to finish.
	q.jobCtx, q.abort = context.WithCancel(context.WithoutCancel(ctx))
	ctx, q.stop = context.WithCancel(ctx)
	q.mu.Unlock()
	for range q.opts.Workers {
		q.running.Go(func() { q.work(ctx) })
	}
	for _, s := range q.sched {
		q.running.Go(func() { q.runSchedule(ctx, s) })
	}
	q.running.Go(func() { q.cleanup(ctx) })
	return nil
}

// Stop stops taking jobs and waits for running ones to finish, until ctx
// ends. Jobs still running then are retried after their timeout, by this or
// another instance.
func (q *Queue) Stop(ctx context.Context) error {
	q.mu.Lock()
	stop, abort := q.stop, q.abort
	q.mu.Unlock()
	if stop == nil {
		return nil
	}
	stop()
	done := make(chan struct{})
	go func() { q.running.Wait(); close(done) }()
	select {
	case <-done:
		abort()
		return nil
	case <-ctx.Done():
		abort() // their locks expire and another worker retries them
		<-done
		return ctx.Err()
	}
}

func (q *Queue) kindNames() []string {
	q.mu.RLock()
	defer q.mu.RUnlock()
	names := make([]string, 0, len(q.kinds))
	for k := range q.kinds {
		names = append(names, k)
	}
	return names
}

func (q *Queue) work(ctx context.Context) {
	t := time.NewTicker(q.opts.PollInterval)
	defer t.Stop()
	for {
		// Run jobs until none are ready, then wait.
		for ctx.Err() == nil {
			ran, err := q.runOne(ctx)
			if err != nil && ctx.Err() == nil {
				q.log.Error("jobs: claiming a job", "err", err)
				break
			}
			if !ran {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-q.wake:
		case <-t.C:
		}
	}
}

// claim marks the next ready job as running and returns it.
func (q *Queue) claim(ctx context.Context) (*Job, *handler, error) {
	names := q.kindNames()
	if len(names) == 0 {
		return nil, nil, nil
	}
	now := time.Now()
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(names)), ", ")
	lock := ""
	if q.d == sqldb.Postgres {
		lock = " FOR UPDATE SKIP LOCKED"
	}
	// A running job whose lock expired was abandoned by a crashed worker.
	query := q.d.Rebind(`UPDATE tern_jobs SET state = 'running', attempts = attempts + 1, locked_until = ?
		WHERE id = (
			SELECT id FROM tern_jobs
			WHERE kind IN (` + placeholders + `)
			  AND ((state = 'queued' AND run_at <= ?) OR (state = 'running' AND locked_until < ?))
			ORDER BY run_at, id LIMIT 1` + lock + `
		)
		RETURNING id, kind, payload, attempts, max_attempts, run_at, created_at`)
	args := []any{int64(0)} // locked_until, set below per kind
	for _, n := range names {
		args = append(args, n)
	}
	args = append(args, now.UnixMilli(), now.UnixMilli())
	// The lock lasts as long as the longest timeout; the exact kind is not
	// known before claiming.
	var longest time.Duration
	q.mu.RLock()
	for _, h := range q.kinds {
		longest = max(longest, h.timeout)
	}
	q.mu.RUnlock()
	args[0] = now.Add(longest + time.Minute).UnixMilli()

	var j Job
	var payload string
	var runAt, created int64
	err := q.db.QueryRowContext(ctx, query, args...).Scan(&j.ID, &j.Kind, &payload, &j.Attempts, &j.MaxAttempts, &runAt, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	j.Payload = json.RawMessage(payload)
	j.RunAt, j.CreatedAt = time.UnixMilli(runAt), time.UnixMilli(created)
	q.mu.RLock()
	h := q.kinds[j.Kind]
	q.mu.RUnlock()
	return &j, h, nil
}

func (q *Queue) runOne(ctx context.Context) (bool, error) {
	j, h, err := q.claim(ctx)
	if err != nil || j == nil {
		return false, err
	}
	err = q.call(ctx, h, j)
	// Record the result even if the queue is stopping.
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	now := time.Now()
	switch {
	case err == nil:
		_, err = q.db.ExecContext(rctx, q.d.Rebind(`UPDATE tern_jobs SET state = 'done', finished_at = ?, last_error = NULL, locked_until = NULL WHERE id = ?`), now.UnixMilli(), j.ID)
	case j.Attempts >= j.MaxAttempts:
		q.log.Error("jobs: job failed", "kind", j.Kind, "id", j.ID, "attempts", j.Attempts, "err", err)
		_, err = q.db.ExecContext(rctx, q.d.Rebind(`UPDATE tern_jobs SET state = 'failed', finished_at = ?, last_error = ?, locked_until = NULL WHERE id = ?`), now.UnixMilli(), err.Error(), j.ID)
	default:
		q.log.Warn("jobs: job will be retried", "kind", j.Kind, "id", j.ID, "attempt", j.Attempts, "err", err)
		_, err = q.db.ExecContext(rctx, q.d.Rebind(`UPDATE tern_jobs SET state = 'queued', run_at = ?, last_error = ?, locked_until = NULL WHERE id = ?`), now.Add(Backoff(j.Attempts)).UnixMilli(), err.Error(), j.ID)
	}
	return true, err
}

// Backoff is the delay before retrying after the given attempt: 1s, 4s,
// 16s and so on, up to an hour, with jitter.
func Backoff(attempt int) time.Duration {
	d := time.Second << (2 * min(attempt-1, 10))
	d = min(d, time.Hour)
	return d/2 + rand.N(d/2+1) //nolint:gosec // jitter, not security
}

func (q *Queue) call(_ context.Context, h *handler, j *Job) (err error) {
	ctx, cancel := context.WithTimeout(q.jobCtx, h.timeout)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	return h.run(ctx, j.Payload)
}

func (q *Queue) cleanup(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		cutoff := time.Now().Add(-q.opts.Retention).UnixMilli()
		if _, err := q.db.ExecContext(ctx, q.d.Rebind(`DELETE FROM tern_jobs WHERE state IN ('done', 'failed') AND finished_at < ?`), cutoff); err != nil && ctx.Err() == nil {
			q.log.Error("jobs: deleting old jobs", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Stats counts jobs by state.
func (q *Queue) Stats(ctx context.Context) (map[State]int, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT state, COUNT(*) FROM tern_jobs GROUP BY state`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[State]int{Queued: 0, Running: 0, Done: 0, Failed: 0}
	for rows.Next() {
		var s State
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		out[s] = n
	}
	return out, rows.Err()
}

// List returns the newest jobs, of one state if state is not "".
func (q *Queue) List(ctx context.Context, state State, limit int) ([]Job, error) {
	where, args := "", []any{}
	if state != "" {
		where, args = " WHERE state = ?", append(args, string(state))
	}
	args = append(args, limit)
	rows, err := q.db.QueryContext(ctx, q.d.Rebind(`SELECT id, kind, payload, state, attempts, max_attempts, COALESCE(last_error, ''), run_at, created_at, finished_at
		FROM tern_jobs`+where+` ORDER BY id DESC LIMIT ?`), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Job
	for rows.Next() {
		var j Job
		var payload string
		var runAt, created int64
		var finished sql.NullInt64
		if err := rows.Scan(&j.ID, &j.Kind, &payload, &j.State, &j.Attempts, &j.MaxAttempts, &j.LastError, &runAt, &created, &finished); err != nil {
			return nil, err
		}
		j.Payload = json.RawMessage(payload)
		j.RunAt, j.CreatedAt = time.UnixMilli(runAt), time.UnixMilli(created)
		if finished.Valid {
			t := time.UnixMilli(finished.Int64)
			j.FinishedAt = &t
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// Retry queues a failed job again, with its attempts reset.
func (q *Queue) Retry(ctx context.Context, id int64) error {
	res, err := q.db.ExecContext(ctx, q.d.Rebind(`UPDATE tern_jobs SET state = 'queued', attempts = 0, run_at = ?, finished_at = NULL WHERE id = ? AND state = 'failed'`), time.Now().UnixMilli(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("jobs: no failed job %d", id)
	}
	q.notify()
	return nil
}
