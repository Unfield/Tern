package jobs_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Unfield/Tern/internal/dbtest"
	"github.com/Unfield/Tern/jobs"
)

type email struct {
	To string `json:"to"`
}

func newQueue(t *testing.T, db *sql.DB) *jobs.Queue {
	t.Helper()
	q, err := jobs.New(db, jobs.Options{PollInterval: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func start(t *testing.T, qs ...*jobs.Queue) {
	t.Helper()
	for _, q := range qs {
		if err := q.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = q.Stop(ctx)
		})
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRunsJobs(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) {
		q := newQueue(t, db)
		var mu sync.Mutex
		var got []string
		send := jobs.Handle(q, "email", func(ctx context.Context, e email) error {
			mu.Lock()
			defer mu.Unlock()
			got = append(got, e.To)
			return nil
		})
		start(t, q)
		ctx := context.Background()
		id, err := send.Enqueue(ctx, email{To: "ada@example.com"})
		if err != nil || id == 0 {
			t.Fatalf("Enqueue = %d, %v", id, err)
		}
		waitFor(t, "the job", func() bool { mu.Lock(); defer mu.Unlock(); return len(got) == 1 })
		if got[0] != "ada@example.com" {
			t.Errorf("payload = %v", got)
		}
		waitFor(t, "done state", func() bool { s, _ := q.Stats(ctx); return s[jobs.Done] == 1 })
		list, err := q.List(ctx, jobs.Done, 10)
		if err != nil || len(list) != 1 || list[0].Kind != "email" || list[0].FinishedAt == nil || list[0].Attempts != 1 {
			t.Errorf("List = %+v, %v", list, err)
		}
	})
}

func TestRetriesAndFailures(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) {
		q := newQueue(t, db)
		var flaky, broken atomic.Int32
		flakyKind := jobs.Handle(q, "flaky", func(ctx context.Context, _ struct{}) error {
			if flaky.Add(1) < 2 {
				return errors.New("try again")
			}
			return nil
		}, jobs.MaxAttempts(3))
		brokenKind := jobs.Handle(q, "broken", func(ctx context.Context, _ struct{}) error {
			if broken.Add(1) == 1 {
				panic("boom")
			}
			return errors.New("still broken")
		}, jobs.MaxAttempts(2))
		start(t, q)
		ctx := context.Background()
		_, _ = flakyKind.Enqueue(ctx, struct{}{})
		id, _ := brokenKind.Enqueue(ctx, struct{}{})

		// The first retry waits 0.5 to 1 second.
		waitFor(t, "the flaky job to succeed", func() bool { s, _ := q.Stats(ctx); return s[jobs.Done] == 1 })
		waitFor(t, "the broken job to fail", func() bool { s, _ := q.Stats(ctx); return s[jobs.Failed] == 1 })
		failed, _ := q.List(ctx, jobs.Failed, 10)
		if len(failed) != 1 || failed[0].Attempts != 2 || failed[0].LastError != "still broken" {
			t.Errorf("failed = %+v", failed)
		}
		if err := q.Retry(ctx, id); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "the retried job to fail again", func() bool { return broken.Load() == 4 })
		if err := q.Retry(ctx, 999999); err == nil {
			t.Error("retrying a missing job should fail")
		}
	})
}

func TestPanicsAreErrors(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) {
		q := newQueue(t, db)
		k := jobs.Handle(q, "panics", func(ctx context.Context, _ struct{}) error { panic("kaboom") }, jobs.MaxAttempts(1))
		start(t, q)
		ctx := context.Background()
		_, _ = k.Enqueue(ctx, struct{}{})
		waitFor(t, "failure", func() bool { s, _ := q.Stats(ctx); return s[jobs.Failed] == 1 })
		failed, _ := q.List(ctx, jobs.Failed, 1)
		if !strings.Contains(failed[0].LastError, "panic: kaboom") {
			t.Errorf("last error = %q", failed[0].LastError)
		}
	})
}

func TestUniqueDelayAndTx(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) {
		q := newQueue(t, db)
		var runs atomic.Int32
		k := jobs.Handle(q, "remind", func(ctx context.Context, _ struct{}) error { runs.Add(1); return nil })
		ctx := context.Background()
		if err := q.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
		const delay = time.Second
		enqueued := time.Now()
		a, err := k.Enqueue(ctx, struct{}{}, jobs.Unique("remind:1"), jobs.Delay(delay))
		if err != nil || a == 0 {
			t.Fatalf("first = %d, %v", a, err)
		}
		if b, err := k.Enqueue(ctx, struct{}{}, jobs.Unique("remind:1")); err != nil || b != 0 {
			t.Fatalf("duplicate = %d, %v", b, err)
		}

		tx, _ := db.Begin()
		if _, err := k.Enqueue(ctx, struct{}{}, jobs.InTx(tx)); err != nil {
			t.Fatal(err)
		}
		_ = tx.Rollback() // the job goes with the transaction

		start(t, q)
		time.Sleep(150 * time.Millisecond)
		// Only a run before the delay is up is early; slow machines may get there late.
		if ran := runs.Load(); ran != 0 && time.Since(enqueued) < delay {
			t.Error("the delayed job ran early")
		}
		waitFor(t, "the delayed job", func() bool { return runs.Load() == 1 })
		time.Sleep(200 * time.Millisecond)
		if n := runs.Load(); n != 1 {
			t.Errorf("runs = %d, want 1", n)
		}
		// Once it finished, the key is free again.
		if c, err := k.Enqueue(ctx, struct{}{}, jobs.Unique("remind:1")); err != nil || c == 0 {
			t.Errorf("after finishing = %d, %v", c, err)
		}
	})
}

func TestEachJobRunsOnceAcrossInstances(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) {
		var mu sync.Mutex
		seen := map[int]int{}
		var queues []*jobs.Queue
		var kind *jobs.Kind[int]
		for range 2 {
			q := newQueue(t, db)
			kind = jobs.Handle(q, "count", func(ctx context.Context, n int) error {
				mu.Lock()
				seen[n]++
				mu.Unlock()
				return nil
			})
			queues = append(queues, q)
		}
		if err := queues[0].Migrate(context.Background()); err != nil {
			t.Fatal(err)
		}
		start(t, queues...)
		for i := range 60 {
			if _, err := kind.Enqueue(context.Background(), i); err != nil {
				t.Fatal(err)
			}
		}
		waitFor(t, "all jobs", func() bool { mu.Lock(); defer mu.Unlock(); return len(seen) == 60 })
		time.Sleep(200 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		for n, c := range seen {
			if c != 1 {
				t.Errorf("job %d ran %d times", n, c)
			}
		}
	})
}

func TestScheduleRunsOncePerTick(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) {
		var runs atomic.Int32
		var queues []*jobs.Queue
		for range 2 {
			q := newQueue(t, db)
			if err := q.Schedule("tick", "@every 1s", func(ctx context.Context) error { runs.Add(1); return nil }); err != nil {
				t.Fatal(err)
			}
			queues = append(queues, q)
		}
		if err := queues[0].Migrate(context.Background()); err != nil {
			t.Fatal(err)
		}
		start(t, queues...)
		time.Sleep(3200 * time.Millisecond)
		if n := runs.Load(); n < 2 || n > 4 {
			t.Errorf("runs in 3.2s = %d, want about 3 (once per second, not per instance)", n)
		}
	})
}

func TestStopWaitsForRunningJobs(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) {
		q := newQueue(t, db)
		started := make(chan struct{})
		var finished atomic.Bool
		k := jobs.Handle(q, "slow", func(ctx context.Context, _ struct{}) error {
			close(started)
			time.Sleep(300 * time.Millisecond)
			finished.Store(true)
			return ctx.Err()
		})
		if err := q.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		_, _ = k.Enqueue(context.Background(), struct{}{})
		<-started
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := q.Stop(ctx); err != nil {
			t.Fatal(err)
		}
		if !finished.Load() {
			t.Error("Stop returned before the running job finished")
		}
		s, _ := q.Stats(context.Background())
		if s[jobs.Done] != 1 {
			t.Errorf("stats = %v", s)
		}
	})
}
