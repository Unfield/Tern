package kit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Unfield/Tern/internal/nilslices"
)

// Deferred is a value that a load starts computing but does not wait for.
// The page renders as soon as the rest of the data is ready, and the
// deferred value streams to the browser when it resolves. In Svelte it is
// a Promise:
//
//	type Data struct {
//		Post     Post                     `json:"post"`
//		Comments kit.Deferred[[]Comment] `json:"comments"`
//	}
//
//	{#await data.comments}Loading…{:then comments}…{:catch error}…{/await}
//
// Create one with [Defer].
type Deferred[T any] struct {
	d *deferred
}

type deferred struct {
	id    int
	done  chan struct{}
	value any
	err   error
}

// Defer runs fn in the background and returns its result as a Deferred,
// so slow data does not hold up the page. fn should respect ctx for
// cancellation; it runs only for the lifetime of the request.
func Defer[T any](ctx *Ctx, fn func() (T, error)) Deferred[T] {
	root := ctx.rootCtx()
	d := &deferred{done: make(chan struct{})}
	root.mu.Lock()
	root.deferredSeq++
	d.id = root.deferredSeq
	root.deferreds = append(root.deferreds, d)
	root.mu.Unlock()

	go func() {
		defer close(d.done)
		defer func() {
			if r := recover(); r != nil {
				d.err = &panicError{value: r, stack: stack()}
			}
		}()
		v, err := fn()
		d.value, d.err = v, err
	}()
	return Deferred[T]{d: d}
}

// MarshalJSON encodes a placeholder that the client replaces with a Promise.
func (x Deferred[T]) MarshalJSON() ([]byte, error) {
	if x.d == nil {
		return []byte("null"), nil
	}
	return fmt.Appendf(nil, `{"__tern_deferred":%d}`, x.d.id), nil
}

// UnmarshalJSON ignores its input: deferred values are not available to
// kit.Parent, only to the browser.
func (x *Deferred[T]) UnmarshalJSON([]byte) error { return nil }

// pendingDeferreds returns the deferred values registered for a request.
func (c *Ctx) pendingDeferreds() []*deferred {
	r := c.rootCtx()
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*deferred(nil), r.deferreds...)
}

type deferredChunk struct {
	ID    int        `json:"id"`
	Data  any        `json:"data,omitempty"`
	Error *errorBody `json:"error,omitempty"`
}

// streamDeferred writes each deferred value as it resolves, using write
// to frame it, until all are done or the client goes away.
func (a *App) streamDeferred(c *Ctx, write func(chunk []byte)) {
	rc := http.NewResponseController(c.writer)
	sent := 0
	for {
		pending := c.pendingDeferreds()
		if sent == len(pending) {
			return
		}
		results := make(chan *deferred, len(pending)-sent)
		for _, d := range pending[sent:] {
			go func() {
				select {
				case <-d.done:
					results <- d
				case <-c.Request.Context().Done():
				}
			}()
		}
		for range len(pending) - sent {
			var d *deferred
			select {
			case d = <-results:
			case <-c.Request.Context().Done():
				return
			}
			chunk := deferredChunk{ID: d.id}
			if d.err != nil {
				_, msg, expected := classify(d.err)
				if !expected {
					a.logError(c, d.err)
					if a.dev != nil {
						msg = d.err.Error()
					}
				}
				chunk.Error = &errorBody{Message: msg}
			} else {
				chunk.Data = nilslices.Remove(d.value)
			}
			b, err := json.Marshal(chunk)
			if err != nil {
				a.logError(c, fmt.Errorf("kit: encoding deferred data: %w", err))
				b, _ = json.Marshal(deferredChunk{ID: d.id, Error: &errorBody{Message: "Internal Error"}})
			}
			write(b)
			_ = rc.Flush()
		}
		sent = len(pending)
	}
}

// resolveScript defines window.__tern_resolve before any chunk arrives.
const resolveScript = `__tern_d={};__tern_resolve=function(c){__tern_d[c.id]=c;if(window.__tern_on)window.__tern_on(c)}`

// splitBody splits a document before its closing </body>, where streamed
// chunks are written.
func splitBody(doc string) (before, after string) {
	if i := strings.LastIndex(doc, "</body>"); i >= 0 {
		return doc[:i], doc[i:]
	}
	return doc, ""
}
