// Package ssr renders Svelte pages on the server by running the app's
// server bundle in embedded QuickJS VMs. It is pure Go: no cgo and no Node.
package ssr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"time"

	"modernc.org/quickjs"
)

// Result is a rendered page.
type Result struct {
	Head string `json:"head"`
	Body string `json:"body"`
	// Skip reports that the payload is not something the server renders.
	Skip bool `json:"skip"`
}

// Renderer renders a page payload. href is the page's absolute URL.
type Renderer interface {
	Render(ctx context.Context, payload []byte, href string) (Result, error)
}

// Options configure a Pool.
type Options struct {
	// Workers is the number of VMs, and so the number of concurrent
	// renders. The default is GOMAXPROCS, at most DefaultMaxWorkers.
	Workers int
	// Timeout bounds a single render. The default is 5 seconds.
	Timeout time.Duration
	// MemoryLimit caps each VM's heap, in bytes. The default is 256 MiB.
	MemoryLimit uintptr
	// Logger receives console output from server-side code.
	Logger *slog.Logger
	// MaxRenders retires a VM after this many renders and replaces it with
	// a fresh one, built in the background. It bounds memory that libraries
	// keep across renders. The default is 10,000; a negative value never
	// retires VMs.
	MaxRenders int
}

// DefaultMaxWorkers caps the default number of VMs. The VMs share the C
// runtime's allocator, which takes a global lock on every JavaScript
// function call, so throughput peaks around four concurrent renders and
// falls with more (see docs/benchmarks.md).
const DefaultMaxWorkers = 4

// Pool renders with a fixed number of reusable VMs. VMs are created on
// demand; a VM that times out or fails is discarded and replaced.
type Pool struct {
	opts     Options
	bundle   string
	bytecode []byte
	slots    chan *vm // idle VMs; nil entries are free slots
	closed   chan struct{}
	once     sync.Once
}

type vm struct {
	js      *quickjs.VM
	renders int
}

// ErrTimeout is returned when a render exceeds the pool's timeout.
var ErrTimeout = errors.New("ssr: render timed out")

// NewPool compiles bundle, which must define globalThis.__tern_render,
// and returns a pool ready to render. It creates one VM up front so a
// broken bundle fails at startup rather than on the first request.
func NewPool(bundle string, opts Options) (*Pool, error) {
	if opts.Workers <= 0 {
		opts.Workers = min(runtime.GOMAXPROCS(0), DefaultMaxWorkers)
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	if opts.MemoryLimit == 0 {
		opts.MemoryLimit = 256 << 20
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.MaxRenders == 0 {
		opts.MaxRenders = 10_000
	}

	compiler, err := quickjs.NewVM()
	if err != nil {
		return nil, fmt.Errorf("ssr: %w", err)
	}
	bytecode, err := compiler.Compile(bundle, quickjs.EvalGlobal)
	_ = compiler.Close()
	if err != nil {
		return nil, fmt.Errorf("ssr: compiling the server bundle: %w", err)
	}

	p := &Pool{opts: opts, bundle: bundle, bytecode: bytecode, slots: make(chan *vm, opts.Workers), closed: make(chan struct{})}
	first, err := p.newVM()
	if err != nil {
		return nil, err
	}
	p.slots <- first
	for range opts.Workers - 1 {
		p.slots <- nil
	}
	return p, nil
}

func (p *Pool) newVM() (*vm, error) {
	js, err := quickjs.NewVM()
	if err != nil {
		return nil, fmt.Errorf("ssr: %w", err)
	}
	js.SetMemoryLimit(p.opts.MemoryLimit)
	js.SetMaxStackSize(8 << 20)
	logger := p.opts.Logger
	err = js.RegisterHostFunc("__tern_log", func(args []any) (any, error) {
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
	if err != nil {
		_ = js.Close()
		return nil, fmt.Errorf("ssr: %w", err)
	}
	if _, err := js.EvalBytecode(p.bytecode); err != nil {
		_ = js.Close()
		return nil, fmt.Errorf("ssr: loading the server bundle: %w%s", err, p.loadStack())
	}
	if ok, _ := js.Eval("typeof globalThis.__tern_render === 'function'", quickjs.EvalGlobal); ok != true {
		_ = js.Close()
		return nil, errors.New("ssr: the server bundle does not define __tern_render")
	}
	return &vm{js: js}, nil
}

// loadStack re-runs the bundle in a scratch VM to capture the JavaScript
// stack of a load error, which the bytecode error does not include.
func (p *Pool) loadStack() string {
	js, err := quickjs.NewVM()
	if err != nil {
		return ""
	}
	defer func() { _ = js.Close() }()
	g := js.GlobalObject()
	atom, err := js.NewAtom("__tern_bundle")
	if err != nil || js.SetProperty(g, atom, p.bundle) != nil {
		return ""
	}
	out, _ := js.Eval("try { (0, eval)(globalThis.__tern_bundle); '' } catch (e) { e && e.stack ? String(e.stack) : '' }", quickjs.EvalGlobal)
	stack, _ := out.(string)
	if stack == "" {
		return ""
	}
	return "\n" + strings.TrimRight(stack, "\n") + "\n(a library probably uses a browser or Node API at load time; see https://github.com/Unfield/Tern/blob/main/docs/guide.md#rendering)"
}

// Render renders payload, waiting for a free VM if all are busy.
func (p *Pool) Render(ctx context.Context, payload []byte, href string) (Result, error) {
	var v *vm
	select {
	case v = <-p.slots:
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-p.closed:
		return Result{}, errors.New("ssr: pool closed")
	}

	if v == nil {
		var err error
		if v, err = p.newVM(); err != nil {
			p.slots <- nil
			return Result{}, err
		}
	}

	res, err := p.renderWith(ctx, v, payload, href)
	if err != nil {
		// The VM may be in an inconsistent state; replace it.
		_ = v.js.Close()
		p.slots <- nil
		return Result{}, err
	}
	v.renders++
	if p.opts.MaxRenders > 0 && v.renders >= p.opts.MaxRenders {
		_ = v.js.Close()
		go p.replace()
		return res, nil
	}
	p.slots <- v
	return res, nil
}

func (p *Pool) renderWith(ctx context.Context, v *vm, payload []byte, href string) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, p.opts.Timeout)
	defer cancel()
	stop := context.AfterFunc(ctx, v.js.Interrupt)
	out, err := v.js.Call("__tern_render", string(payload), href)
	interrupted := !stop()
	if interrupted {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Result{}, ErrTimeout
		}
		return Result{}, ctx.Err()
	}
	if err != nil {
		return Result{}, fmt.Errorf("ssr: %w", err)
	}
	s, ok := out.(string)
	if !ok {
		return Result{}, fmt.Errorf("ssr: __tern_render returned %T, want a string", out)
	}
	var res Result
	if err := json.Unmarshal([]byte(s), &res); err != nil {
		return Result{}, fmt.Errorf("ssr: decoding the render result: %w", err)
	}
	return res, nil
}

// replace builds a VM for a retired one's slot, so requests don't wait.
func (p *Pool) replace() {
	select {
	case <-p.closed:
		p.slots <- nil
		return
	default:
	}
	v, err := p.newVM()
	if err != nil {
		p.opts.Logger.Error("ssr: replacing a renderer", "err", err)
		p.slots <- nil // the next render tries again
		return
	}
	p.slots <- v
}

// Close releases the pool's VMs. Renders in progress finish first.
func (p *Pool) Close() error {
	p.once.Do(func() { close(p.closed) })
	for range cap(p.slots) {
		if v := <-p.slots; v != nil {
			_ = v.js.Close()
		}
	}
	return nil
}
