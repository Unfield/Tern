package logging

import (
	"context"
	"log/slog"
	"runtime"
	"slices"
)

// core is what a logger and the handlers derived from it share.
type core struct {
	level    slog.Leveler
	out      slog.Handler // the formatted output; nil for none
	redact   *redactor
	sample   *sampler
	ships    []*shipper
	source   bool
	minShip  slog.Level // the lowest level any shipper takes
	hasShips bool
}

// handler is the slog.Handler of loggers from New.
type handler struct {
	c *core
	// goa are the WithGroup and WithAttrs calls made on this handler, in
	// order; their attributes already cleaned.
	goa []groupOrAttrs
	// out is c.out with goa applied, so the output handler formats them
	// once rather than for each record.
	out slog.Handler
}

type groupOrAttrs struct {
	group string
	attrs []slog.Attr
}

func (h *handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.c.level.Level()
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var clean []slog.Attr
	for _, a := range attrs {
		clean = h.c.redact.clean(clean, a)
	}
	if len(clean) == 0 {
		return h
	}
	n := &handler{c: h.c, goa: append(slices.Clip(h.goa), groupOrAttrs{attrs: clean})}
	if h.out != nil {
		n.out = h.out.WithAttrs(clean)
	}
	return n
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	n := &handler{c: h.c, goa: append(slices.Clip(h.goa), groupOrAttrs{group: name})}
	if h.out != nil {
		n.out = h.out.WithGroup(name)
	}
	return n
}

// readsContext marks handlers that add a context's attributes themselves.
func (*handler) readsContext() {}

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	c := h.c
	if !c.sample.keep(r.Level, r.Message, r.Time) {
		return nil
	}
	ctxAttrs := Attrs(ctx)
	var cleanCtx []slog.Attr
	for _, a := range ctxAttrs {
		cleanCtx = c.redact.clean(cleanCtx, a)
	}
	var recAttrs []slog.Attr
	r.Attrs(func(a slog.Attr) bool {
		recAttrs = c.redact.clean(recAttrs, a)
		return true
	})

	var err error
	if h.out != nil && h.out.Enabled(ctx, r.Level) {
		nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
		out := h.out
		first := firstGroup(h.goa)
		if len(cleanCtx) > 0 && first < len(h.goa) {
			// The context's attributes go before the first group, where
			// tree puts them too.
			out = c.out
			for i, g := range h.goa {
				if i == first {
					out = out.WithAttrs(cleanCtx)
				}
				if g.group != "" {
					out = out.WithGroup(g.group)
				} else {
					out = out.WithAttrs(g.attrs)
				}
			}
		} else {
			nr.AddAttrs(cleanCtx...)
		}
		nr.AddAttrs(recAttrs...)
		err = out.Handle(ctx, nr)
	}
	if c.hasShips && r.Level >= c.minShip {
		e := Entry{Time: r.Time, Level: r.Level, Message: r.Message, Attrs: tree(cleanCtx, h.goa, recAttrs)}
		if c.source && r.PC != 0 {
			e.Source = source(r.PC)
		}
		for _, sh := range c.ships {
			if sh.accepts(r.Level) {
				sh.add(e)
			}
		}
	}
	return err
}

// firstGroup is the index of the first group in goa, len(goa) if none.
func firstGroup(goa []groupOrAttrs) int {
	for i, g := range goa {
		if g.group != "" {
			return i
		}
	}
	return len(goa)
}

// tree nests the attributes as the record has them: each WithAttrs in the
// groups opened before it, the context's before the first group, and the
// record's own innermost. The output has them in the same order.
func tree(ctx []slog.Attr, goa []groupOrAttrs, rec []slog.Attr) []slog.Attr {
	var out []slog.Attr
	for i, g := range goa {
		if g.group == "" {
			out = append(out, g.attrs...)
			continue
		}
		out = append(out, ctx...)
		if inner := nest(goa[i+1:], rec); len(inner) > 0 {
			out = append(out, slog.Attr{Key: g.group, Value: slog.GroupValue(inner...)})
		}
		return out
	}
	out = append(out, ctx...)
	return append(out, rec...)
}

func nest(goa []groupOrAttrs, rec []slog.Attr) []slog.Attr {
	var out []slog.Attr
	for i, g := range goa {
		if g.group == "" {
			out = append(out, g.attrs...)
			continue
		}
		if inner := nest(goa[i+1:], rec); len(inner) > 0 {
			out = append(out, slog.Attr{Key: g.group, Value: slog.GroupValue(inner...)})
		}
		return out
	}
	return append(out, rec...)
}

func source(pc uintptr) *slog.Source {
	fs := runtime.CallersFrames([]uintptr{pc})
	f, _ := fs.Next()
	return &slog.Source{Function: f.Function, File: f.File, Line: f.Line}
}
