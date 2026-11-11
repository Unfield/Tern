package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Entry is a log record as sinks get it: its attributes resolved, redacted
// and nested in their groups, those of the context first.
type Entry struct {
	Time    time.Time
	Level   slog.Level
	Message string
	// Attrs are the record's attributes. Groups are slog.KindGroup values.
	Attrs []slog.Attr
	// Source is where the record was logged, when [Options.AddSource] is
	// set.
	Source *slog.Source
}

// Walk calls fn for each attribute of e that isn't a group, with the keys
// of the groups it is in joined by ".": "http.status" for status in group
// http.
func (e Entry) Walk(fn func(key string, v slog.Value)) {
	walk("", e.Attrs, fn)
}

func walk(prefix string, attrs []slog.Attr, fn func(string, slog.Value)) {
	for _, a := range attrs {
		key := a.Key
		if prefix != "" {
			key = prefix + "." + a.Key
		}
		if a.Value.Kind() == slog.KindGroup {
			walk(key, a.Value.Group(), fn)
			continue
		}
		fn(key, a.Value)
	}
}

// Lookup returns the value of the top-level attribute key.
func (e Entry) Lookup(key string) (slog.Value, bool) {
	for _, a := range e.Attrs {
		if a.Key == key {
			return a.Value, true
		}
	}
	return slog.Value{}, false
}

// A Sink sends log entries somewhere: a collector, a log service, a file.
// Its Export gets them in batches from one goroutine, in the order they
// were logged; see [Options.Batch] for how batches are made and retried.
//
// Export returns an error to have the batch sent again, after a growing
// pause, or [Permanent] to drop it. [RetryAfter] asks for a pause of its
// own, as a service's Retry-After header does.
type Sink interface {
	Export(ctx context.Context, entries []Entry) error
}

// SinkFunc makes a function a Sink.
type SinkFunc func(ctx context.Context, entries []Entry) error

// Export implements Sink.
func (f SinkFunc) Export(ctx context.Context, entries []Entry) error { return f(ctx, entries) }

// MinLevel sends s only entries at level or above.
func MinLevel(s Sink, level slog.Leveler) Sink { return &leveled{Sink: s, level: level} }

type leveled struct {
	Sink
	level slog.Leveler
}

// Close closes the sink it wraps.
func (l *leveled) Close() error {
	if c, ok := l.Sink.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

type permanent struct{ err error }

func (p permanent) Error() string { return p.err.Error() }
func (p permanent) Unwrap() error { return p.err }

// Permanent marks an error from Export as one retrying won't fix, such as
// a rejected request: the batch is dropped.
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

// RetryAfter marks an error from Export as one to retry after d.
func RetryAfter(err error, d time.Duration) error {
	if err == nil {
		return nil
	}
	return retryAfter{err, d}
}

// Batch says how entries are sent to sinks. Entries wait in a queue; a
// batch goes out when Size have gathered or Interval has passed. A batch
// that fails is retried MaxRetries times, pausing 1s, 2s, 4s… up to 30s
// between tries; meanwhile new entries queue. When the queue holds Queue
// entries, new ones are dropped and counted (see [Stats]).
type Batch struct {
	Size       int           // default 512
	Interval   time.Duration // default 1s
	Queue      int           // default 16384
	MaxRetries int           // default 5; -1 for none
	// Timeout bounds each Export call. Default 15s.
	Timeout time.Duration
}

func (b Batch) withDefaults() Batch {
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

// shipper queues entries for one sink and exports them from a goroutine.
type shipper struct {
	sink  Sink
	name  string
	level slog.Leveler // nil: all
	b     Batch

	mu      sync.Mutex
	queue   []Entry
	closed  bool
	flushes []chan struct{} // waiting Flush calls, answered after the queue empties
	wake    chan struct{}
	done    chan struct{}

	dropped atomic.Uint64 // queue full
	failed  atomic.Uint64 // given up on after retries, or permanent errors

	// errOut gets a line when exporting fails, at most once a minute.
	errOut    io.Writer
	lastError time.Time
	sleep     func(ctx context.Context, d time.Duration) bool
}

func newShipper(s Sink, b Batch, errOut io.Writer) *shipper {
	sh := &shipper{sink: s, name: sinkName(s), b: b.withDefaults(), wake: make(chan struct{}, 1), done: make(chan struct{}), errOut: errOut, sleep: sleepCtx}
	if l, ok := s.(*leveled); ok {
		sh.level = l.level
	}
	go sh.loop()
	return sh
}

func sinkName(s Sink) string {
	if l, ok := s.(*leveled); ok {
		s = l.Sink
	}
	if n, ok := s.(interface{ Name() string }); ok {
		return n.Name()
	}
	return strings.TrimPrefix(fmt.Sprintf("%T", s), "*")
}

func (sh *shipper) accepts(level slog.Level) bool {
	return sh.level == nil || level >= sh.level.Level()
}

func (sh *shipper) add(e Entry) {
	sh.mu.Lock()
	if sh.closed || len(sh.queue) >= sh.b.Queue {
		sh.mu.Unlock()
		sh.dropped.Add(1)
		return
	}
	sh.queue = append(sh.queue, e)
	full := len(sh.queue) >= sh.b.Size
	sh.mu.Unlock()
	if full {
		sh.signal()
	}
}

func (sh *shipper) signal() {
	select {
	case sh.wake <- struct{}{}:
	default:
	}
}

func (sh *shipper) loop() {
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
			sh.export(batch, closed)
			sh.mu.Lock()
			// The batch is the front of the queue: drop it, keeping the
			// backing array from growing without bound.
			rest := sh.queue[n:]
			if cap(sh.queue) > 4*sh.b.Queue || len(rest) == 0 {
				sh.queue = append(make([]Entry, 0, max(len(rest), sh.b.Size)), rest...)
			} else {
				sh.queue = rest
			}
			sh.mu.Unlock()
		}
	}
}

// export sends one batch, retrying as Batch says. While closing, it tries
// once more at most, so shutdown isn't held up by a sink that is down.
func (sh *shipper) export(batch []Entry, closing bool) {
	pause := time.Second
	tries := sh.b.MaxRetries
	if closing {
		tries = min(tries, 1)
	}
	for attempt := 0; ; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), sh.b.Timeout)
		err := sh.sink.Export(ctx, batch)
		cancel()
		if err == nil {
			return
		}
		var p permanent
		if errors.As(err, &p) || attempt >= tries {
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
		if !sh.sleep(context.Background(), wait) {
			return
		}
		pause = min(pause*2, 30*time.Second)
	}
}

func (sh *shipper) report(err error, n int) {
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
		fmt.Fprintf(sh.errOut, "logging: %s: dropped %d entries: %v\n", sh.name, n, err)
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

// flush waits until what was queued before it has been sent or ctx ends.
func (sh *shipper) flush(ctx context.Context) error {
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

// close sends what is queued, stops the goroutine and closes the sink if
// it is an io.Closer.
func (sh *shipper) close(ctx context.Context) error {
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
	if c, ok := sh.sink.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

var stderr io.Writer = os.Stderr
