//go:build !tern_nojs

package ssr

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	ctx     context.Context // the running call's, for Funcs
}

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
	v := &vm{js: js}
	for name, fn := range p.opts.Funcs {
		if err := js.RegisterHostFunc(name, func(args []any) (any, error) {
			ctx := v.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			return fn(ctx, args)
		}); err != nil {
			_ = js.Close()
			return nil, fmt.Errorf("ssr: %w", err)
		}
	}
	// crypto.getRandomValues in the server polyfills.
	err = js.RegisterHostFunc("__tern_random", func(args []any) (any, error) {
		n := 0
		if len(args) > 0 {
			switch v := args[0].(type) {
			case int:
				n = v
			case int64:
				n = int(v)
			case float64:
				n = int(v)
			}
		}
		if n < 0 || n > 65536 {
			return nil, errors.New("getRandomValues: at most 65536 bytes")
		}
		b := make([]byte, n)
		_, _ = rand.Read(b)
		return hex.EncodeToString(b), nil
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
	return v, nil
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
	out, err := p.call(ctx, "__tern_render", string(payload), href)
	if err != nil {
		return Result{}, err
	}
	var res Result
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return Result{}, fmt.Errorf("ssr: decoding the render result: %w", err)
	}
	return res, nil
}

// Email renders an email template with props (JSON).
func (p *Pool) Email(ctx context.Context, name string, props []byte) (EmailResult, []byte, error) {
	out, err := p.call(ctx, "__tern_email", name, string(props))
	if err != nil {
		return EmailResult{}, nil, err
	}
	return decodeEmail(name, out)
}

// call runs a global function of the bundle in a free VM.
func (p *Pool) call(ctx context.Context, fn string, args ...any) (string, error) {
	var v *vm
	select {
	case v = <-p.slots:
	case <-ctx.Done():
		return "", ctx.Err()
	case <-p.closed:
		return "", errors.New("ssr: pool closed")
	}

	if v == nil {
		var err error
		if v, err = p.newVM(); err != nil {
			p.slots <- nil
			return "", err
		}
	}
	if fn != "__tern_render" {
		if ok, _ := v.js.Eval("typeof globalThis."+fn+" === 'function'", quickjs.EvalGlobal); ok != true {
			p.slots <- v
			return "", ErrNoEmails
		}
	}

	out, err := p.callWith(ctx, v, fn, args...)
	if err != nil {
		// The VM may be in an inconsistent state; replace it.
		_ = v.js.Close()
		p.slots <- nil
		return "", err
	}
	v.renders++
	if p.opts.MaxRenders > 0 && v.renders >= p.opts.MaxRenders {
		_ = v.js.Close()
		go p.replace()
		return out, nil
	}
	p.slots <- v
	return out, nil
}

func (p *Pool) callWith(ctx context.Context, v *vm, fn string, args ...any) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.opts.Timeout)
	defer cancel()
	v.ctx = ctx
	defer func() { v.ctx = nil }()
	stop := context.AfterFunc(ctx, v.js.Interrupt)
	out, err := v.js.Call(fn, args...)
	interrupted := !stop()
	if interrupted {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", ErrTimeout
		}
		return "", ctx.Err()
	}
	if err != nil {
		return "", fmt.Errorf("ssr: %w", err)
	}
	s, ok := out.(string)
	if !ok {
		return "", fmt.Errorf("ssr: %s returned %T, want a string", fn, out)
	}
	return s, nil
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

// ErrNoEngine is what renders that need the JavaScript engine return in a
// binary built without it, with the tern_nojs tag.
var ErrNoEngine = errors.New("ssr: this binary has no JavaScript engine")
