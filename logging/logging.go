// Package logging is structured logging for Tern apps, on log/slog. A
// logger from [New] adds the request's ID to every record logged in a
// request, redacts passwords, tokens and other secrets, can sample floods
// of the same message, and ships records to collectors and log services
// through sinks, such as [github.com/Unfield/Tern/logging/otlp], in the
// background:
//
//	logger := logging.New(logging.Options{
//		Sinks: []logging.Sink{otlp.New(otlp.Options{})},
//	})
//	slog.SetDefault(logger)
//	server, err := ternApp(kit.WithLogger(logger))
//
// kit flushes the sinks when the server shuts down. Under tern dev, records
// also appear in the dev server's log view, at /__tern/dev/logs.
//
// The defaults come from the environment: TERN_LOG_LEVEL (debug, info,
// warn, error) and TERN_LOG_FORMAT (text, json). Without TERN_LOG_FORMAT,
// output is text on a terminal and under tern dev, JSON otherwise.
package logging

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Options configures a logger.
type Options struct {
	// Level is the lowest level logged. Pass a *slog.LevelVar to change it
	// while the app runs. The default is TERN_LOG_LEVEL's, or Info.
	Level slog.Leveler
	// Format is "text" or "json". See the package documentation for the
	// default.
	Format string
	// Output is where formatted records go. The default is os.Stderr; use
	// io.Discard to only ship records to sinks.
	Output io.Writer
	// AddSource adds the file and line that logged each record.
	AddSource bool

	// Redact adds keys to [DefaultRedact]; Keep takes keys out of it.
	// NoDefaultRedact leaves only Redact.
	Redact          []string
	Keep            []string
	NoDefaultRedact bool

	// Sampling, when set, samples floods of records with the same message.
	Sampling *Sampling

	// Sinks get every record at or above Level (or [MinLevel]'s level), in
	// batches, from a goroutine each.
	Sinks []Sink
	// Batch says how records are batched and retried for sinks.
	Batch Batch
}

// New returns a logger as opts say.
func New(opts Options) *slog.Logger {
	return slog.New(NewHandler(opts))
}

// NewHandler returns the handler of a logger from [New], for use with
// handlers of other packages.
func NewHandler(opts Options) slog.Handler {
	level := opts.Level
	if level == nil {
		level = envLevel()
	}
	out := opts.Output
	if out == nil {
		out = os.Stderr
	}
	c := &core{
		level:  level,
		redact: newRedactor(opts.Redact, opts.Keep, opts.NoDefaultRedact),
		sample: newSampler(opts.Sampling),
		source: opts.AddSource,
	}
	if out != io.Discard {
		ho := &slog.HandlerOptions{Level: level, AddSource: opts.AddSource}
		if format(opts.Format, out) == "json" {
			c.out = slog.NewJSONHandler(out, ho)
		} else {
			c.out = slog.NewTextHandler(out, ho)
		}
	}
	for _, s := range opts.Sinks {
		c.ships = append(c.ships, newShipper(s, opts.Batch, stderr))
	}
	if d := devShipper(); d != nil {
		c.ships = append(c.ships, d)
	}
	c.hasShips = len(c.ships) > 0
	c.minShip = slog.LevelDebug - 100
	if c.hasShips {
		c.minShip = shipFloor(c.ships)
	}
	return &handler{c: c, out: c.out}
}

// shipFloor is the lowest level any of the shippers takes.
func shipFloor(ships []*shipper) slog.Level {
	floor := slog.Level(1 << 30)
	for _, sh := range ships {
		if sh.level == nil {
			return slog.Level(-1 << 30)
		}
		floor = min(floor, sh.level.Level())
	}
	return floor
}

func envLevel() slog.Leveler {
	var l slog.Level
	if s := os.Getenv("TERN_LOG_LEVEL"); s != "" {
		if err := l.UnmarshalText([]byte(s)); err == nil {
			return l
		}
	}
	return slog.LevelInfo
}

func format(f string, out io.Writer) string {
	if f == "" {
		f = os.Getenv("TERN_LOG_FORMAT")
	}
	switch strings.ToLower(f) {
	case "json":
		return "json"
	case "text":
		return "text"
	}
	if os.Getenv("TERN_DEV_VITE_URL") != "" {
		return "text"
	}
	if f, ok := out.(*os.File); ok {
		if st, err := f.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
			return "text"
		}
	}
	return "json"
}

// Flush waits until the records l logged so far have been sent to its
// sinks, or ctx ends. l is a logger from [New] or [Wrap]; for others it
// does nothing. kit calls it when the server shuts down.
func Flush(ctx context.Context, l *slog.Logger) error {
	var errs []error
	for _, sh := range shippersOf(l) {
		errs = append(errs, sh.flush(ctx))
	}
	return errors.Join(errs...)
}

// Close flushes l's sinks and stops them; records logged after it are
// written to the output but not shipped.
func Close(ctx context.Context, l *slog.Logger) error {
	var errs []error
	for _, sh := range shippersOf(l) {
		if sh == dev.sh {
			errs = append(errs, sh.flush(ctx)) // shared by every logger
			continue
		}
		errs = append(errs, sh.close(ctx))
	}
	return errors.Join(errs...)
}

func shippersOf(l *slog.Logger) []*shipper {
	if l == nil {
		return nil
	}
	switch h := l.Handler().(type) {
	case *handler:
		return h.c.ships
	case *wrapped:
		if h.dev != nil {
			return []*shipper{h.dev}
		}
	}
	return nil
}

// Stats counts what a logger didn't ship.
type Stats struct {
	// Sampled is the records sampling left out.
	Sampled uint64
	// Dropped is the entries left out because a sink's queue was full.
	Dropped uint64
	// Failed is the entries a sink failed to take after retrying.
	Failed uint64
}

// StatsOf returns l's counts, for a logger from [New].
func StatsOf(l *slog.Logger) Stats {
	var s Stats
	h, ok := l.Handler().(*handler)
	if !ok {
		return s
	}
	if h.c.sample != nil {
		s.Sampled = h.c.sample.dropped.Load()
	}
	for _, sh := range h.c.ships {
		s.Dropped += sh.dropped.Load()
		s.Failed += sh.failed.Load()
	}
	return s
}

// Wrap makes h add the attributes of the context a record is logged with,
// as handlers from [NewHandler] do, and under tern dev send records to the
// dev server's log view. A handler from NewHandler is returned as it is.
// kit wraps the handler of the logger it is given with it.
func Wrap(h slog.Handler) slog.Handler {
	if _, ok := h.(interface{ readsContext() }); ok {
		return h
	}
	return &wrapped{h: h, dev: devShipper()}
}

type wrapped struct {
	h   slog.Handler
	dev *shipper
	goa []groupOrAttrs
}

func (*wrapped) readsContext() {}

func (w *wrapped) Enabled(ctx context.Context, l slog.Level) bool { return w.h.Enabled(ctx, l) }

func (w *wrapped) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return w
	}
	n := &wrapped{h: w.h.WithAttrs(attrs), dev: w.dev, goa: w.goa}
	if w.dev != nil {
		var clean []slog.Attr
		for _, a := range attrs {
			clean = defaultRedactor.clean(clean, a)
		}
		n.goa = append(append([]groupOrAttrs(nil), w.goa...), groupOrAttrs{attrs: clean})
	}
	return n
}

func (w *wrapped) WithGroup(name string) slog.Handler {
	if name == "" {
		return w
	}
	n := &wrapped{h: w.h.WithGroup(name), dev: w.dev, goa: w.goa}
	if w.dev != nil {
		n.goa = append(append([]groupOrAttrs(nil), w.goa...), groupOrAttrs{group: name})
	}
	return n
}

func (w *wrapped) Handle(ctx context.Context, r slog.Record) error {
	attrs := Attrs(ctx)
	if w.dev != nil {
		var rec []slog.Attr
		r.Attrs(func(a slog.Attr) bool {
			rec = defaultRedactor.clean(rec, a)
			return true
		})
		var cleanCtx []slog.Attr
		for _, a := range attrs {
			cleanCtx = defaultRedactor.clean(cleanCtx, a)
		}
		w.dev.add(Entry{Time: r.Time, Level: r.Level, Message: r.Message, Attrs: tree(cleanCtx, w.goa, rec)})
	}
	if len(attrs) > 0 {
		r = r.Clone()
		r.AddAttrs(attrs...)
	}
	return w.h.Handle(ctx, r)
}

// dev is the shipper to tern dev's log view, one per process.
var (
	dev struct {
		once atomic.Bool
		sh   *shipper
	}
	devMu sync.Mutex
)

// devShipper returns the shipper to tern dev, or nil outside it.
func devShipper() *shipper {
	if dev.once.Load() {
		return dev.sh
	}
	devMu.Lock()
	defer devMu.Unlock()
	if !dev.once.Load() {
		if u := os.Getenv("TERN_DEV_LOGS"); u != "" {
			dev.sh = newShipper(&devSink{url: u, token: os.Getenv("TERN_DEV_LOGS_TOKEN")}, Batch{Size: 256, Interval: 100 * time.Millisecond, MaxRetries: 2, Timeout: 2 * time.Second}, nil)
		}
		dev.once.Store(true)
	}
	return dev.sh
}
