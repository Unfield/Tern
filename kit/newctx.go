package kit

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
)

// CtxOptions describes the request a Ctx made with NewCtx belongs to.
type CtxOptions struct {
	// Route is the route's ID, as RouteID returns it: "/blog/slug.param".
	Route string
	// Params are the route's parameters.
	Params map[string]string
	// Parent is the layouts' data, which Parent[T] decodes: a struct or a
	// map, encoded as a load's result is.
	Parent any
	// Locals are added to the Ctx's Locals.
	Locals map[string]any
	// NoHooks skips the App's hooks, the plugins' among them, which a Ctx
	// runs through by default, as a request does before its loads.
	NoHooks bool
}

// NewCtx makes the Ctx of request r, written to w, as the App makes one
// for a route: with the request's locale and its hooks run. Call a load,
// an action or an endpoint with it: in tests (see kit/kittest), or from a
// handler of your own. A hook's error is returned, as it would end the
// request.
func (a *App) NewCtx(w http.ResponseWriter, r *http.Request, o CtxOptions) (*Ctx, error) {
	c := newCtx(a, &responseWriter{ResponseWriter: w}, r)
	c.routeID = o.Route
	if o.Params != nil {
		c.params = maps.Clone(o.Params)
	}
	maps.Copy(c.Locals, o.Locals)
	if o.Parent != nil {
		b, err := json.Marshal(o.Parent)
		if err != nil {
			return nil, fmt.Errorf("kit.NewCtx: encoding Parent: %w", err)
		}
		var merged map[string]json.RawMessage
		if err := json.Unmarshal(b, &merged); err != nil {
			return nil, fmt.Errorf("kit.NewCtx: Parent must encode to a JSON object: %w", err)
		}
		c.parent = func() (map[string]json.RawMessage, error) { return merged, nil }
	} else {
		c.parent = func() (map[string]json.RawMessage, error) { return map[string]json.RawMessage{}, nil }
	}
	a.resolveLocale(c)
	if !o.NoHooks {
		for _, hook := range a.opts.hooks {
			if err := a.safely(func() error { return hook(c) }); err != nil {
				return c, err
			}
		}
	}
	return c, nil
}

// Flush writes the headers and cookies set on c to its response, for a
// Ctx made with NewCtx whose handler doesn't write one itself.
func (c *Ctx) Flush() { c.applyHeaders() }
