package ssr

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Unfield/Tern/native/js"
)

// NewRealm makes a realm with an app's compiled modules evaluated in it,
// after setup has added host functions. tern build generates one for an
// app (ADR 0011).
type NewRealm func(setup func(*js.Realm)) *js.Realm

// Native renders with the app compiled to Go: one realm per worker, so
// renders run in parallel without the JavaScript engine. A render a realm
// can't finish (code compiled as unsupported, an exception, a bug in the
// compiled code) is rendered again by the QuickJS pool, made the first
// time it's needed.
type Native struct {
	opts     Options
	newRealm NewRealm
	bundle   string
	slots    chan *realmSlot // idle realms; nil entries are free slots
	closed   chan struct{}
	once     sync.Once

	poolOnce sync.Once
	pool     *Pool
	poolErr  error
	reasons  sync.Map // fallback reasons already logged
	// engineOnly holds the paths that fell back, so their next renders go
	// straight to the engine instead of failing again in a fresh realm.
	engineOnly sync.Map
	engineN    atomic.Int64
}

// maxEngineOnly bounds engineOnly: past it, paths that fall back are
// tried natively again each time.
const maxEngineOnly = 4096

type realmSlot struct {
	rt      *js.Realm
	render  js.Value
	raw     js.Value // __tern_render_raw, which returns the result as an object; nil if absent
	renders int
	ctx     context.Context // the running render's, for Funcs
}

// NativeOptions configure a Native renderer beyond the pool's Options.
type NativeOptions struct {
	// Verify renders every page with the JavaScript engine too and logs
	// where the HTML differs. It returns the engine's result, so it is
	// safe in production while native rendering is checked.
	Verify bool
}

// NewNative returns a native renderer. bundle is the server bundle, for
// the QuickJS fallback. It makes one realm up front: if the compiled
// modules can't be evaluated, every page renders with the engine.
func NewNative(newRealm NewRealm, bundle string, opts Options, nopts NativeOptions) (Renderer, error) {
	if opts.Workers <= 0 {
		opts.Workers = runtime.GOMAXPROCS(0)
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.MaxRenders == 0 {
		opts.MaxRenders = 10_000
	}
	n := &Native{opts: opts, newRealm: newRealm, bundle: bundle, slots: make(chan *realmSlot, opts.Workers), closed: make(chan struct{})}
	first, err := n.newSlot()
	if err != nil {
		opts.Logger.Warn("ssr: rendering every page with the JavaScript engine: native rendering can't start", "err", err)
		return NewPool(bundle, opts)
	}
	n.slots <- first
	for range opts.Workers - 1 {
		n.slots <- nil
	}
	if nopts.Verify {
		return &verifying{native: n}, nil
	}
	return n, nil
}

// newSlot makes a realm: evaluating the modules is the app's startup
// code, which may fail.
func (n *Native) newSlot() (s *realmSlot, err error) {
	s = &realmSlot{}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("ssr: evaluating the compiled server modules: %s", describePanic(r))
		}
	}()
	s.rt = n.newRealm(func(rt *js.Realm) { n.hostFuncs(rt, s) })
	s.rt.DropJobs()
	render, ok := s.rt.Global.Get("__tern_render").(*js.Object)
	if !ok || !render.IsCallable() {
		return nil, errors.New("ssr: the compiled server modules don't define __tern_render")
	}
	s.render = render
	if raw, ok := s.rt.Global.Get("__tern_render_raw").(*js.Object); ok && raw.IsCallable() {
		s.raw = raw
	}
	return s, nil
}

// hostFuncs adds what the QuickJS renderer's VMs have: __tern_log for
// console, __tern_random for crypto.getRandomValues, and Options.Funcs.
func (n *Native) hostFuncs(rt *js.Realm, s *realmSlot) {
	logger := n.opts.Logger
	rt.HostFunc("__tern_log", func(args []any) (any, error) {
		if len(args) < 2 {
			return nil, nil
		}
		level, _ := args[0].(string)
		msg, _ := args[1].(string)
		switch level {
		case "error":
			logger.Error(msg, "source", "ssr")
		case "warn":
			logger.Warn(msg, "source", "ssr")
		case "debug":
			logger.Debug(msg, "source", "ssr")
		default:
			logger.Info(msg, "source", "ssr")
		}
		return nil, nil
	})
	rt.HostFunc("__tern_random", func(args []any) (any, error) {
		k := 0
		if len(args) > 0 {
			switch v := args[0].(type) {
			case int:
				k = v
			case float64:
				k = int(v)
			}
		}
		if k < 0 || k > 65536 {
			return nil, errors.New("getRandomValues: at most 65536 bytes")
		}
		b := make([]byte, k)
		_, _ = rand.Read(b)
		return hex.EncodeToString(b), nil
	})
	for name, fn := range n.opts.Funcs {
		rt.HostFunc(name, func(args []any) (any, error) {
			ctx := s.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			return fn(ctx, args)
		})
	}
}

// fallbackError is a render the realm couldn't do, for the engine to redo.
type fallbackError struct {
	reason string
	// thrown is a JavaScript exception, which the engine may throw too:
	// then it's the page's own error, not native rendering's.
	thrown bool
}

func (e *fallbackError) Error() string { return "ssr: native rendering failed: " + e.reason }

// Render renders payload natively, or with the JavaScript engine when the
// realm can't.
func (n *Native) Render(ctx context.Context, payload []byte, href string) (Result, error) {
	path := pathOf(href)
	if _, ok := n.engineOnly.Load(path); ok {
		p, err := n.fallbackPool()
		if err != nil {
			return Result{}, err
		}
		return p.Render(ctx, payload, href)
	}
	res, err := n.renderNative(ctx, payload, href)
	var fe *fallbackError
	if errors.As(err, &fe) {
		p, perr := n.fallbackPool()
		if perr != nil {
			return Result{}, perr
		}
		res, err := p.Render(ctx, payload, href)
		if fe.thrown && err != nil {
			// The engine throws too: the page's error, which renders
			// natively next time as well.
			return res, err
		}
		if _, seen := n.reasons.LoadOrStore(fe.reason, true); !seen {
			n.opts.Logger.Warn("ssr: rendering with the JavaScript engine: native rendering failed", "reason", fe.reason, "url", href)
		}
		if n.engineN.Load() < maxEngineOnly {
			if _, loaded := n.engineOnly.LoadOrStore(path, true); !loaded {
				n.engineN.Add(1)
			}
		}
		return res, err
	}
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

func (n *Native) fallbackPool() (*Pool, error) {
	n.poolOnce.Do(func() {
		opts := n.opts
		opts.Workers = min(opts.Workers, DefaultMaxWorkers)
		n.pool, n.poolErr = NewPool(n.bundle, opts)
	})
	return n.pool, n.poolErr
}

// renderNative runs __tern_render in a free realm.
func (n *Native) renderNative(ctx context.Context, payload []byte, href string) (Result, error) {
	var s *realmSlot
	select {
	case s = <-n.slots:
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-n.closed:
		return Result{}, errors.New("ssr: renderer closed")
	}
	if s == nil {
		var err error
		if s, err = n.newSlot(); err != nil {
			n.slots <- nil
			return Result{}, &fallbackError{reason: err.Error()}
		}
	}
	out, err := n.run(ctx, s, payload, href)
	if err != nil {
		// A failed render may leave the modules' state half changed, so the
		// realm goes, as a failed VM does.
		n.slots <- nil
		return Result{}, err
	}
	s.renders++
	if n.opts.MaxRenders > 0 && s.renders >= n.opts.MaxRenders {
		go n.replace()
		return out, nil
	}
	n.slots <- s
	return out, nil
}

func (n *Native) run(ctx context.Context, s *realmSlot, payload []byte, href string) (out Result, err error) {
	ctx, cancel := context.WithTimeout(ctx, n.opts.Timeout)
	defer cancel()
	stop := context.AfterFunc(ctx, s.rt.Interrupt)
	defer stop()
	s.ctx = ctx
	defer func() {
		s.ctx = nil
		r := recover()
		if r == nil {
			return
		}
		if r == any(js.ErrInterrupted) {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				err = ErrTimeout
			} else {
				err = ctx.Err()
			}
			return
		}
		_, thrown := r.(*js.Throw)
		err = &fallbackError{reason: describePanic(r), thrown: thrown}
	}()
	if s.raw != nil {
		v := js.Call(s.raw, js.Undefined, string(payload), href)
		s.rt.DropJobs()
		o, ok := v.(*js.Object)
		if !ok {
			return Result{}, &fallbackError{reason: fmt.Sprintf("__tern_render_raw returned %s", js.Typeof(v))}
		}
		str := func(k string) string {
			if x, ok := o.Get(k).(string); ok {
				return x
			}
			return ""
		}
		return Result{Head: str("head"), Body: str("body"), Skip: js.Truthy(o.Get("skip"))}, nil
	}
	v := js.Call(s.render, js.Undefined, string(payload), href)
	s.rt.DropJobs()
	str, ok := v.(string)
	if !ok {
		return Result{}, &fallbackError{reason: fmt.Sprintf("__tern_render returned %s", js.Typeof(v))}
	}
	var res Result
	if err := json.Unmarshal([]byte(str), &res); err != nil {
		return Result{}, fmt.Errorf("ssr: decoding the render result: %w", err)
	}
	return res, nil
}

// pathOf is href without its origin, query and fragment.
func pathOf(href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	return u.Path
}

func describePanic(r any) string {
	switch x := r.(type) {
	case *js.Unsupported:
		return x.What + " (not compiled to Go)"
	case *js.Throw:
		return x.Error()
	case error:
		return x.Error()
	}
	return fmt.Sprint(r)
}

// replace builds a realm for a retired one's slot.
func (n *Native) replace() {
	select {
	case <-n.closed:
		n.slots <- nil
		return
	default:
	}
	s, err := n.newSlot()
	if err != nil {
		n.opts.Logger.Error("ssr: replacing a native renderer", "err", err)
		n.slots <- nil
		return
	}
	n.slots <- s
}

// Close waits for renders in progress and releases the realms and the
// fallback pool.
func (n *Native) Close() error {
	n.once.Do(func() { close(n.closed) })
	for range cap(n.slots) {
		<-n.slots
	}
	if n.pool != nil {
		return n.pool.Close()
	}
	return nil
}

// verifying renders natively and with the JavaScript engine, logs where
// they differ, and returns the engine's result.
type verifying struct{ native *Native }

func (v *verifying) Render(ctx context.Context, payload []byte, href string) (Result, error) {
	n := v.native
	p, err := n.fallbackPool()
	if err != nil {
		return Result{}, err
	}
	want, err := p.Render(ctx, payload, href)
	if err != nil {
		return want, err
	}
	got, nerr := n.renderNative(ctx, payload, href)
	switch {
	case nerr != nil && strings.Contains(nerr.Error(), "not compiled to Go"):
		n.opts.Logger.Info("ssr: page uses what isn't compiled to Go; the engine renders it", "url", href, "err", nerr)
	case nerr != nil:
		n.opts.Logger.Error("ssr: native rendering failed where the engine didn't", "url", href, "err", nerr)
	case got != want:
		n.opts.Logger.Error("ssr: native rendering differs from the engine", "url", href, "at", firstDiff(got, want))
	}
	return want, nil
}

func (v *verifying) Close() error { return v.native.Close() }

// firstDiff describes where two results first differ.
func firstDiff(a, b Result) string {
	if a.Skip != b.Skip {
		return "skip"
	}
	for _, f := range []struct {
		name string
		x, y string
	}{{"head", a.Head, b.Head}, {"body", a.Body, b.Body}} {
		if f.x == f.y {
			continue
		}
		i := 0
		for i < len(f.x) && i < len(f.y) && f.x[i] == f.y[i] {
			i++
		}
		start := max(0, i-40)
		return fmt.Sprintf("%s byte %d: native %q, engine %q", f.name, i, clip(f.x, start, i+40), clip(f.y, start, i+40))
	}
	return ""
}

func clip(s string, from, to int) string {
	return s[min(from, len(s)):min(to, len(s))]
}
