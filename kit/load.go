package kit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
)

type loadResult struct {
	done chan struct{}
	data json.RawMessage
	err  error
}

// errParentFailed marks a load that stopped because a parent layout failed.
var errParentFailed = errors.New("kit: a parent layout failed")

// runLoads runs the load functions for a chain of nodes (layouts, then the
// page) concurrently. A load that calls Parent waits only for the nodes
// above it. It returns each node's JSON data (nil for nodes without a load)
// and, on failure, the index of the outermost failing node and its error.
func (a *App) runLoads(c *Ctx, fns []LoadFunc) (nodes []json.RawMessage, failed int, err error) {
	results := make([]*loadResult, len(fns))
	for i := range fns {
		results[i] = &loadResult{done: make(chan struct{})}
	}

	var wg sync.WaitGroup
	for i, fn := range fns {
		res := results[i]
		if fn == nil {
			close(res.done)
			continue
		}
		parent := func() (map[string]json.RawMessage, error) {
			merged := map[string]json.RawMessage{}
			for _, p := range results[:i] {
				<-p.done
				if p.err != nil {
					return nil, errParentFailed
				}
				if err := mergeInto(merged, p.data); err != nil {
					return nil, err
				}
			}
			return merged, nil
		}
		wg.Go(func() {
			defer close(res.done)
			var v any
			res.err = a.safely(func() error {
				var err error
				v, err = fn(c.fork(parent))
				return err
			})
			if res.err == nil {
				res.data, res.err = encodeNode(v)
			}
		})
	}
	wg.Wait()

	nodes = make([]json.RawMessage, len(fns))
	for i, res := range results {
		if res.err != nil && !errors.Is(res.err, errParentFailed) {
			return nodes, i, res.err
		}
		nodes[i] = res.data
	}
	return nodes, len(fns), nil
}

// encodeNode marshals a load result, which must encode to a JSON object.
func encodeNode(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
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

func mergeInto(dst map[string]json.RawMessage, data json.RawMessage) error {
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
