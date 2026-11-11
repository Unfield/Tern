package kit

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/cache"
)

// Pages rendered on demand: a route with `const Revalidate = 60` is
// rendered on its first request and served from the cache after, then
// rendered again once it's older than that, in the background while the
// old copy is still served. Its loads' tags (Ctx.Tag) let Revalidate
// refresh it sooner.
//
// The cached copy is the page as anyone sees it: it's rendered without
// the request's cookies and Authorization header, so nothing about the
// visitor can end up in it, and hooks don't run for the copies served.

// Forever is the Revalidate of a route whose page is cached until a tag
// it used is revalidated.
const Forever = time.Duration(-1)

// Seconds turns a route's `const Revalidate = 60` into a duration; the
// generated code calls it.
func Seconds(n int64) time.Duration {
	if n < 0 {
		return Forever
	}
	return time.Duration(n) * time.Second
}

// WithCache sets the cache that pages rendered on demand, and loads that
// use Ctx.Cache, keep their entries in. The default is 64 MB in memory,
// per process; a cache on a shared store (cache/redis, or cache.Disk on a
// shared volume) is shared by every process.
func WithCache(c *cache.Cache) Option { return func(o *options) { o.cache = c } }

// staleFor is how long past its Revalidate a page may still be served
// while it's rendered again.
const staleFor = 24 * time.Hour

type renderForCacheKey struct{}

// cached reports whether r is the render of a page for the cache.
func cached(r *http.Request) bool { return r.Context().Value(renderForCacheKey{}) != nil }

// Cache is the App's cache, for loads to keep results in:
//
//	posts, err := cache.Get(c, c.Cache(), "posts:recent", cache.For(time.Minute, "posts"), db.RecentPosts)
func (c *Ctx) Cache() *cache.Cache { return c.app.cache() }

func (a *App) cache() *cache.Cache {
	a.cacheOnce.Do(func() {
		if a.opts.cache == nil {
			a.opts.cache = cache.New(cache.Memory(64 << 20))
		}
	})
	return a.opts.cache
}

// Tag names data the request used, such as "posts" or "post:42", so
// Revalidate with the tag refreshes a page rendered on demand that used
// it. Outside such a render it does nothing.
func (c *Ctx) Tag(tags ...string) {
	r := c.rootCtx()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range tags {
		if !slices.Contains(r.tags, t) {
			r.tags = append(r.tags, t)
		}
	}
}

// Revalidate makes the cached pages and load results with any of tags
// stale, in every process that shares the cache. The next request renders
// them again. Call it after the data changes:
//
//	err := c.Revalidate("posts", "post:"+id)
func (c *Ctx) Revalidate(tags ...string) error { return c.app.Revalidate(c.Request.Context(), tags...) }

// Revalidate is Ctx.Revalidate, for jobs and other code without a request.
func (a *App) Revalidate(ctx context.Context, tags ...string) error {
	return a.cache().Invalidate(ctx, tags...)
}

// pageEntry is a stored response: its status and headers, then the body.
type pageEntry struct {
	Status int         `json:"s"`
	Header http.Header `json:"h"`
}

// pageKey is the cache key of a page: the locale, document or data, and
// the URL. The locale is resolved from the request as it would be when
// rendering, so each language has its own copy.
func pageKey(locale string, isData bool, r *http.Request) string {
	kind := "html"
	if isData {
		kind = "data"
	}
	return "page:" + kind + ":" + locale + ":" + r.URL.Path + "?" + r.URL.RawQuery
}

// servePageCache answers a page request from the cache, rendering it when
// it's missing or stale, and reports whether it did. orig is the request
// as it came, before the base path was taken off.
func (a *App) servePageCache(w http.ResponseWriter, r, orig *http.Request, def *RouteDef, isData bool, locale string) bool {
	ctx := r.Context()
	c := a.cache()
	key := pageKey(locale, isData, r)
	ttl := def.Revalidate
	if ttl == Forever {
		ttl = 0
	}
	opts := cache.Options{TTL: ttl, Stale: staleFor}
	e, ok, err := c.Lookup(ctx, key)
	if err != nil {
		a.logger.WarnContext(ctx, "tern: reading the page cache", "error", err)
		return false // render as usual
	}
	render := func(ctx context.Context) ([]byte, []string, error) {
		return a.renderForCache(ctx, orig)
	}
	state := "HIT"
	switch {
	case ok && e.Fresh:
	case ok && ttl > 0 && time.Since(e.Made) < ttl+staleFor:
		state = "STALE"
		go func() {
			if _, err := c.Make(context.WithoutCancel(ctx), key, opts, render); err != nil {
				a.logger.Warn("tern: rendering a page for the cache", "path", r.URL.Path, "error", err)
			}
		}()
	default:
		state = "MISS"
		v, err := c.Make(ctx, key, opts, render)
		if err != nil {
			a.logger.WarnContext(ctx, "tern: rendering a page for the cache", "path", r.URL.Path, "error", err)
			return false
		}
		e = cache.Entry{Value: v, Made: time.Now()}
	}
	return a.writeCachedPage(w, r, def, e, state)
}

// writeCachedPage writes a stored page, with a fresh nonce in place of
// the one it was rendered with.
func (a *App) writeCachedPage(w http.ResponseWriter, r *http.Request, def *RouteDef, e cache.Entry, state string) bool {
	if len(e.Value) < 4 {
		return false
	}
	n := int(binary.BigEndian.Uint32(e.Value))
	if 4+n > len(e.Value) {
		return false
	}
	var meta pageEntry
	if err := json.Unmarshal(e.Value[4:4+n], &meta); err != nil {
		return false
	}
	body := e.Value[4+n:]
	nonce := newNonce()
	h := w.Header()
	for k, vs := range meta.Header {
		for _, v := range vs {
			h.Add(k, strings.ReplaceAll(v, a.cacheNonce, nonce))
		}
	}
	h.Set("X-Tern-Cache", state)
	h.Set("Age", strconv.Itoa(int(time.Since(e.Made).Seconds())))
	if def.CacheControl != "" {
		h.Set("Cache-Control", def.CacheControl)
	}
	h.Del("Content-Length")
	w.WriteHeader(meta.Status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(bytes.ReplaceAll(body, []byte(a.cacheNonce), []byte(nonce)))
	}
	return true
}

// renderForCache renders the page as anyone sees it, and returns the
// stored form with the tags its loads used. Responses other than 200 are
// served but not stored.
func (a *App) renderForCache(ctx context.Context, orig *http.Request) ([]byte, []string, error) {
	r := orig.Clone(context.WithValue(ctx, renderForCacheKey{}, true))
	r.Header.Del("Cookie")
	r.Header.Del("Authorization")
	if strings.Contains(r.URL.RawQuery, reuseParam) {
		q := r.URL.Query()
		q.Del(reuseParam)
		u := *r.URL
		u.RawQuery = q.Encode()
		r.URL = &u
	}
	r.Method = http.MethodGet
	rec := &pageRecorder{header: http.Header{}, status: http.StatusOK}
	a.serve(rec, r)
	h := rec.header.Clone()
	// Nothing that belongs to one visitor.
	h.Del("Set-Cookie")
	h.Del("Vary")
	meta, err := json.Marshal(pageEntry{Status: rec.status, Header: h})
	if err != nil {
		return nil, nil, err
	}
	out := make([]byte, 4, 4+len(meta)+rec.body.Len())
	binary.BigEndian.PutUint32(out, uint32(len(meta)))
	out = append(out, meta...)
	out = append(out, rec.body.Bytes()...)
	if rec.status != http.StatusOK {
		return out, nil, cache.NoStore()
	}
	return out, rec.tags, nil
}

// pageRecorder keeps a response in memory, for the page cache.
type pageRecorder struct {
	header http.Header
	status int
	wrote  bool
	body   bytes.Buffer
	tags   []string
}

func (w *pageRecorder) Header() http.Header { return w.header }

func (w *pageRecorder) WriteHeader(status int) {
	if !w.wrote {
		w.status, w.wrote = status, true
	}
}

func (w *pageRecorder) Write(b []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.body.Write(b)
}

// Flush lets streamed pages render into it: the stored copy has every
// deferred value in place.
func (w *pageRecorder) Flush() {}
