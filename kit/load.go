package kit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"sync"

	"github.com/Unfield/Tern/internal/jsonenc"
)

type loadResult struct {
	once sync.Once
	done chan struct{}
	run  func()
	data jsonenc.Verbatim
	// value is what the load returned, which data encodes, for native
	// rendering to read without parsing data.
	value any
	uses  *nodeUses
	err   error
	ran   bool
}

// ensure runs the load once and waits for it.
func (r *loadResult) ensure() {
	r.once.Do(r.run)
	<-r.done
}

// nodeUses records what a load read, so the client knows when it must run
// again: route parameters (ctx.Param), the URL (ctx.URL), parent data
// (kit.Parent) and explicit dependencies (ctx.Depends).
type nodeUses struct {
	mu     sync.Mutex
	URL    bool     `json:"url,omitempty"`
	Params []string `json:"params,omitempty"`
	Deps   []string `json:"deps,omitempty"`
	Parent bool     `json:"parent,omitempty"`
}

func (u *nodeUses) param(name string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if !slices.Contains(u.Params, name) {
		u.Params = append(u.Params, name)
	}
}

func (u *nodeUses) dep(key string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if !slices.Contains(u.Deps, key) {
		u.Deps = append(u.Deps, key)
	}
}

func (u *nodeUses) set(f func(*nodeUses)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	f(u)
}

// loadOutcome is the result of running a chain of loads.
type loadOutcome struct {
	nodes  []jsonenc.Verbatim
	values []any // what encodes to nodes
	uses   []*nodeUses
	reused []int // nodes the client already has, skipped by request
	failed int
	err    error
}

// errParentFailed marks a load that stopped because a parent layout failed.
var errParentFailed = errors.New("kit: a parent layout failed")

// runLoads runs the load functions for a chain of nodes (layouts, then the
// page) concurrently. A load that calls Parent waits only for the nodes
// above it. Nodes in skip are not run unless a child's Parent call needs
// their data; the client reuses what it already has for them.
func (a *App) runLoads(c *Ctx, fns []LoadFunc, skip map[int]bool) loadOutcome {
	results := make([]*loadResult, len(fns))
	for i, fn := range fns {
		res := &loadResult{done: make(chan struct{})}
		results[i] = res
		if fn == nil {
			res.run = func() { close(res.done) }
			continue
		}
		parent := func() (map[string]json.RawMessage, error) {
			merged := map[string]json.RawMessage{}
			for _, p := range results[:i] {
				p.ensure()
				if p.err != nil {
					return nil, errParentFailed
				}
				if err := mergeInto(merged, p.data); err != nil {
					return nil, err
				}
			}
			return merged, nil
		}
		res.run = func() {
			defer close(res.done)
			res.ran = true
			res.uses = &nodeUses{}
			fork := c.fork(parent)
			fork.uses = res.uses
			var v any
			res.err = a.safely(func() error {
				var err error
				v, err = fn(fork)
				return err
			})
			if res.err == nil {
				res.data, res.err = encodeNode(v)
				res.value = v
			}
		}
	}

	var wg sync.WaitGroup
	for i, res := range results {
		if !skip[i] {
			wg.Go(res.ensure)
		}
	}
	wg.Wait()

	out := loadOutcome{nodes: make([]jsonenc.Verbatim, len(fns)), values: make([]any, len(fns)), uses: make([]*nodeUses, len(fns)), failed: len(fns)}
	for i, res := range results {
		if skip[i] && !res.ran {
			out.reused = append(out.reused, i)
			continue
		}
		if res.err != nil && !errors.Is(res.err, errParentFailed) {
			out.failed, out.err = i, res.err
			out.nodes, out.values = out.nodes[:i], out.values[:i]
			return out
		}
		out.nodes[i] = res.data
		out.values[i] = res.value
		out.uses[i] = res.uses
	}
	return out
}

// encodeNode marshals a load result, which must encode to a JSON object.
func encodeNode(v any) (jsonenc.Verbatim, error) {
	b, err := jsonenc.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("kit: encoding load data: %w", err)
	}
	if bytes.Equal(b, []byte("null")) {
		return nil, nil
	}
	if len(b) == 0 || b[0] != '{' {
		return nil, fmt.Errorf("kit: load data must encode to a JSON object (a struct or map), got %s", truncate(b, 40))
	}
	return b, nil
}

func mergeInto(dst map[string]json.RawMessage, data jsonenc.Verbatim) error {
	if data == nil {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	for k, v := range m {
		dst[k] = v
	}
	return nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

// panicError is a recovered panic from user code.
type panicError struct {
	value any
	stack []byte
}

func (e *panicError) Error() string { return fmt.Sprintf("panic: %v\n\n%s", e.value, e.stack) }

func stack() []byte { return debug.Stack() }
