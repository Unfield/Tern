package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/Unfield/Tern/internal/batch"
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

// Permanent marks an error from Export as one retrying won't fix, such as
// a rejected request: the batch is dropped.
func Permanent(err error) error { return batch.Permanent(err) }

// IsPermanent reports whether err was marked with Permanent.
func IsPermanent(err error) bool { return batch.IsPermanent(err) }

// RetryAfter marks an error from Export as one to retry after d.
func RetryAfter(err error, d time.Duration) error { return batch.RetryAfter(err, d) }

// Batch says how entries are sent to sinks. Entries wait in a queue; a
// batch goes out when Size have gathered or Interval has passed. A batch
// that fails is retried MaxRetries times, pausing 1s, 2s, 4s… up to 30s
// between tries; meanwhile new entries queue. When the queue holds Queue
// entries, new ones are dropped and counted (see [Stats]). The defaults:
// 512 entries, 1s, a queue of 16384, 5 retries and a 15s timeout.
type Batch = batch.Options

// shipper queues entries for one sink and exports them from a goroutine.
type shipper struct {
	*batch.Shipper[Entry]
	level slog.Leveler // nil: all
}

func newShipper(s Sink, b Batch, errOut io.Writer) *shipper {
	c := batch.Config{Name: sinkName(s), Prefix: "logging", What: "entries", ErrOut: errOut}
	if cl, ok := s.(io.Closer); ok {
		c.Close = cl.Close
	}
	sh := &shipper{Shipper: batch.New(c, s.Export, b)}
	if l, ok := s.(*leveled); ok {
		sh.level = l.level
	}
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

func (sh *shipper) add(e Entry)                     { sh.Add(e) }
func (sh *shipper) flush(ctx context.Context) error { return sh.Flush(ctx) }
func (sh *shipper) close(ctx context.Context) error { return sh.Close(ctx) }

var stderr io.Writer = os.Stderr
