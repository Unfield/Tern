// Package batch sends items to an exporter in batches, from a goroutine,
// with a bounded queue and retries: what logging's sinks and tracing's
// exporters share.
package batch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// Options says how items are sent. Items wait in a queue; a batch goes out
// when Size have gathered or Interval has passed. A batch that fails is
// retried MaxRetries times, pausing 1s, 2s, 4s… up to 30s between tries;
// meanwhile new items queue. When the queue holds Queue items, new ones are
// dropped and counted.
type Options struct {
	Size       int           // default 512
	Interval   time.Duration // default 1s
	Queue      int           // default 16384
	MaxRetries int           // default 5; -1 for none
	// Timeout bounds each export. Default 15s.
	Timeout time.Duration
}

// WithDefaults fills in the zero fields.
func (b Options) WithDefaults() Options {
	if b.Size <= 0 {
		b.Size = 512
	}
	if b.Interval <= 0 {
		b.Interval = time.Second
	}
	if b.Queue <= 0 {
		b.Queue = 16384
	}
	if b.Queue < b.Size {
		b.Queue = b.Size
	}
	if b.MaxRetries == 0 {
		b.MaxRetries = 5
	} else if b.MaxRetries < 0 {
		b.MaxRetries = 0
	}
	if b.Timeout <= 0 {
		b.Timeout = 15 * time.Second
	}
	return b
}

type permanent struct{ err error }

func (p permanent) Error() string { return p.err.Error() }
func (p permanent) Unwrap() error { return p.err }

// Permanent marks an export error as one retrying won't fix.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanent{err}
}

// IsPermanent reports whether err was marked with Permanent.
func IsPermanent(err error) bool {
	var p permanent
	return errors.As(err, &p)
}

type retryAfter struct {
	err   error
	after time.Duration
}

func (r retryAfter) Error() string { return r.err.Error() }
func (r retryAfter) Unwrap() error { return r.err }

// RetryAfter marks an export error as one to retry after d.
func RetryAfter(err error, d time.Duration) error {
	if err == nil {
		return nil
	}
	return retryAfter{err, d}
}

// Shipper queues items and exports them from a goroutine. Make one with
// New.
type Shipper[T any] struct {
	name   string
	export func(context.Context, []T) error
	closer func() error
	what   string // "entries", "spans": for the error line
	b      Options

	mu      sync.Mutex
	queue   []T
	closed  bool
	flushes []chan struct{} // waiting Flush calls, answered after the queue empties
	wake    chan struct{}
	done    chan struct{}

	dropped atomic.Uint64 // queue full
	failed  atomic.Uint64 // given up on after retries, or permanent errors

	// errOut gets a line when exporting fails, at most once a minute.
	errOut    io.Writer
	prefix    string
	lastError time.Time
	// Sleep pauses between tries; tests replace it.
	Sleep func(ctx context.Context, d time.Duration) bool
}

// Config names a shipper and says where its errors go.
type Config struct {
	// Name is the exporter's, for error lines: "otlp".
	Name string
	// Prefix starts each error line: "logging", "tracing".
	Prefix string
	// What the items are, plural: "entries", "spans".
	What string
	// ErrOut gets a line when exporting fails, at most once a minute;
	// nil for none.
	ErrOut io.Writer
	// Close is called by Close after the last batch, if set.
	Close func() error
}

// New starts a shipper that exports with export.
func New[T any](c Config, export func(context.Context, []T) error, b Options) *Shipper[T] {
	sh := &Shipper[T]{name: c.Name, export: export, closer: c.Close, what: c.What, prefix: c.Prefix, errOut: c.ErrOut, b: b.WithDefaults(), wake: make(chan struct{}, 1), done: make(chan struct{}), Sleep: sleepCtx}
	if sh.what == "" {
		sh.what = "items"
	}
	go sh.loop()
	return sh
}

// Dropped counts items left out because the queue was full, or added after
// Close.
func (sh *Shipper[T]) Dropped() uint64 { return sh.dropped.Load() }

// Failed counts items the exporter didn't take after retrying.
func (sh *Shipper[T]) Failed() uint64 { return sh.failed.Load() }

// Add queues an item.
func (sh *Shipper[T]) Add(item T) {
	sh.mu.Lock()
	if sh.closed || len(sh.queue) >= sh.b.Queue {
		sh.mu.Unlock()
		sh.dropped.Add(1)
		return
	}
	sh.queue = append(sh.queue, item)
	full := len(sh.queue) >= sh.b.Size
	sh.mu.Unlock()
	if full {
		sh.signal()
	}
}

func (sh *Shipper[T]) signal() {
	select {
	case sh.wake <- struct{}{}:
	default:
	}
}

func (sh *Shipper[T]) loop() {
	defer close(sh.done)
	t := time.NewTicker(sh.b.Interval)
	defer t.Stop()
	for {
		select {
		case <-sh.wake:
		case <-t.C:
		}
		for {
			sh.mu.Lock()
			n := min(len(sh.queue), sh.b.Size)
			batch := sh.queue[:n:n]
			closed := sh.closed
			var waiting []chan struct{}
			if n == 0 {
				waiting, sh.flushes = sh.flushes, nil
				if closed {
					sh.queue = nil
				}
			}
			sh.mu.Unlock()
			if n == 0 {
				for _, w := range waiting {
					close(w)
				}
				if closed {
					return
				}
				break
			}
			sh.send(batch, closed)
			sh.mu.Lock()
			// The batch is the front of the queue: drop it, keeping the
			// backing array from growing without bound.
			rest := sh.queue[n:]
			if cap(sh.queue) > 4*sh.b.Queue || len(rest) == 0 {
				sh.queue = append(make([]T, 0, max(len(rest), sh.b.Size)), rest...)
			} else {
				sh.queue = rest
			}
			sh.mu.Unlock()
		}
	}
}

// send exports one batch, retrying as Options says. While closing, it
// tries once more at most, so shutdown isn't held up by an exporter that
// is down.
func (sh *Shipper[T]) send(batch []T, closing bool) {
	pause := time.Second
	tries := sh.b.MaxRetries
	if closing {
		tries = min(tries, 1)
	}
	for attempt := 0; ; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), sh.b.Timeout)
		err := sh.export(ctx, batch)
		cancel()
		if err == nil {
			return
		}
		if IsPermanent(err) || attempt >= tries {
			sh.failed.Add(uint64(len(batch)))
			sh.report(err, len(batch))
			return
		}
		wait := pause
		var ra retryAfter
		if errors.As(err, &ra) && ra.after > 0 {
			wait = min(ra.after, time.Minute)
		}
		if closing {
			wait = min(wait, time.Second)
		}
		if !sh.Sleep(context.Background(), wait) {
			return
		}
		pause = min(pause*2, 30*time.Second)
	}
}

func (sh *Shipper[T]) report(err error, n int) {
	if sh.errOut == nil {
		return
	}
	now := time.Now()
	sh.mu.Lock()
	quiet := now.Sub(sh.lastError) < time.Minute
	if !quiet {
		sh.lastError = now
	}
	sh.mu.Unlock()
	if !quiet {
		fmt.Fprintf(sh.errOut, "%s: %s: dropped %d %s: %v\n", sh.prefix, sh.name, n, sh.what, err)
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// Flush waits until what was queued before it has been sent, or ctx ends.
func (sh *Shipper[T]) Flush(ctx context.Context) error {
	ch := make(chan struct{})
	sh.mu.Lock()
	if sh.closed && len(sh.queue) == 0 {
		sh.mu.Unlock()
		return nil
	}
	sh.flushes = append(sh.flushes, ch)
	sh.mu.Unlock()
	sh.signal()
	select {
	case <-ch:
		return nil
	case <-sh.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close sends what is queued, stops the goroutine and calls Config.Close.
func (sh *Shipper[T]) Close(ctx context.Context) error {
	sh.mu.Lock()
	already := sh.closed
	sh.closed = true
	sh.mu.Unlock()
	if already {
		select {
		case <-sh.done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	sh.signal()
	select {
	case <-sh.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	if sh.closer != nil {
		return sh.closer()
	}
	return nil
}
