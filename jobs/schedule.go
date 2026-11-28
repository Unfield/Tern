package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type schedule struct {
	name string
	sch  Schedule
	kind *Kind[struct{}]
}

// Schedule runs fn on a recurring schedule (see ParseSchedule), such as
// "*/5 * * * *" or "@every 30s". Each run is a job of kind
// "schedule:<name>", so it is retried like any job. Across instances
// sharing a database, each run happens once, and a run doesn't start while
// the previous one is still going. Register schedules before the queue
// starts.
func (q *Queue) Schedule(name, spec string, fn func(ctx context.Context) error, opts ...HandleOption) error {
	sch, err := ParseSchedule(spec, q.opts.Location)
	if err != nil {
		return err
	}
	if q.started {
		return errors.New("jobs: register schedules before the queue starts")
	}
	kind := Handle(q, "schedule:"+name, func(ctx context.Context, _ struct{}) error { return fn(ctx) }, opts...)
	q.sched = append(q.sched, &schedule{name: name, sch: sch, kind: kind})
	return nil
}

func (q *Queue) runSchedule(ctx context.Context, s *schedule) {
	for {
		wait, err := q.tick(ctx, s)
		if err != nil && ctx.Err() == nil {
			q.log.Error("jobs: schedule", "name", s.name, "err", err)
			wait = q.opts.PollInterval
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// tick runs the schedule if it is due and returns how long to wait.
func (q *Queue) tick(ctx context.Context, s *schedule) (time.Duration, error) {
	now := time.Now()
	next := s.sch.Next(now)
	if next.IsZero() {
		return time.Hour, nil
	}
	// The first instance to start sets the first run.
	if _, err := q.db.ExecContext(ctx, q.d.Rebind(`INSERT INTO tern_schedules (name, next_run) VALUES (?, ?) ON CONFLICT DO NOTHING`), s.name, next.UnixMilli()); err != nil {
		return 0, err
	}
	var due int64
	if err := q.db.QueryRowContext(ctx, q.d.Rebind(`SELECT next_run FROM tern_schedules WHERE name = ?`), s.name).Scan(&due); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return q.opts.PollInterval, nil
		}
		return 0, err
	}
	if now.UnixMilli() < due {
		// Wake at the due time, but check again at least every poll
		// interval in case another instance changed it.
		return min(time.Until(time.UnixMilli(due)), max(q.opts.PollInterval, time.Second)*10), nil
	}
	// Claim this run: only the instance whose update matches the old value
	// enqueues it. Runs missed while no instance was up are not made up.
	res, err := q.db.ExecContext(ctx, q.d.Rebind(`UPDATE tern_schedules SET next_run = ? WHERE name = ? AND next_run = ?`), next.UnixMilli(), s.name, due)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		if _, err := s.kind.Enqueue(ctx, struct{}{}, Unique("schedule:"+s.name)); err != nil {
			return 0, fmt.Errorf("enqueueing: %w", err)
		}
	}
	return time.Until(next), nil
}

// Next reports when a job or a scheduled run is next due, and how many jobs
// this process is running now. at is zero when nothing is waiting, and no
// later than now when something is due already.
//
// A platform that stops idle apps asks before stopping one: it waits while
// running is above zero, and starts the app again at at. The queue runs
// what is due when it starts, so nothing more is needed to catch up.
func (q *Queue) Next(ctx context.Context) (at time.Time, running int, err error) {
	running = int(q.busy.Load())
	now := time.Now()
	earliest := int64(0)
	consider := func(ms int64) {
		if ms > 0 && (earliest == 0 || ms < earliest) {
			earliest = ms
		}
	}
	names := q.kindNames()
	if len(names) > 0 {
		in := strings.TrimSuffix(strings.Repeat("?, ", len(names)), ", ")
		args := make([]any, len(names))
		for i, n := range names {
			args[i] = n
		}
		// Waiting jobs and retries, and running jobs whose lock runs out:
		// those are retried then if their worker is gone.
		var queued, locked sql.NullInt64
		row := q.db.QueryRowContext(ctx, q.d.Rebind(`SELECT
			(SELECT MIN(run_at) FROM tern_jobs WHERE state = 'queued' AND kind IN (`+in+`)),
			(SELECT MIN(locked_until) FROM tern_jobs WHERE state = 'running' AND kind IN (`+in+`))`), append(args, args...)...)
		if err := row.Scan(&queued, &locked); err != nil {
			return time.Time{}, running, fmt.Errorf("jobs: next: %w", err)
		}
		consider(queued.Int64)
		consider(locked.Int64)
	}
	for _, s := range q.sched {
		var due int64
		err := q.db.QueryRowContext(ctx, q.d.Rebind(`SELECT next_run FROM tern_schedules WHERE name = ?`), s.name).Scan(&due)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			// Not run yet: the first instance to start sets it.
			if next := s.sch.Next(now); !next.IsZero() {
				due = next.UnixMilli()
			}
		case err != nil:
			return time.Time{}, running, fmt.Errorf("jobs: next: %w", err)
		}
		consider(due)
	}
	if earliest == 0 {
		return time.Time{}, running, nil
	}
	return time.UnixMilli(earliest), running, nil
}
