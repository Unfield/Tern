// Package cache keeps values that are slow to make: load results, pages
// rendered on demand, answers from other services. A Cache sits on a
// Store, in memory, on disk or in Redis; with Redis or a shared disk,
// every process of the app shares it.
//
//	c := cache.New(cache.Memory(64 << 20))
//	posts, err := cache.Get(ctx, c, "posts:recent", cache.For(time.Minute, "posts"), func(ctx context.Context) ([]Post, error) {
//		return db.RecentPosts(ctx)
//	})
//	...
//	err = c.Invalidate(ctx, "posts") // after a post changes
//
// Entries carry tags. Invalidating a tag makes every entry with it stale
// at once, in every process: each tag has a version in the store, and an
// entry is fresh only while its tags' versions are the ones it was made
// with.
//
// While an entry is made, other callers for the same key in the process
// wait for it instead of making it too.
package cache

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// Store keeps bytes by key. Memory, Disk and Redis are stores; a Store of
// your own works too.
type Store interface {
	// Get returns the value of key, or ok false if there's none or it
	// expired.
	Get(ctx context.Context, key string) (value []byte, ok bool, err error)
	// Set stores value for ttl; 0 keeps it until it's evicted.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// Delete removes keys.
	Delete(ctx context.Context, keys ...string) error
	// Incr adds one to the number at key, starting from 0, and returns it.
	Incr(ctx context.Context, key string) (int64, error)
}

// Cache keeps entries in a Store, with tags. Create it with New.
type Cache struct {
	store  Store
	prefix string
	mu     sync.Mutex
	flight map[string]*call
}

type call struct {
	done  chan struct{}
	value []byte
	err   error
}

// New returns a cache on store.
func New(store Store) *Cache {
	return &Cache{store: store, prefix: "tern:", flight: map[string]*call{}}
}

// Store returns the cache's store.
func (c *Cache) Store() Store { return c.store }

// Options say how long an entry is fresh, and its tags.
type Options struct {
	// TTL is how long the entry is fresh; 0 is until a tag is invalidated.
	TTL time.Duration
	// Stale is how long after TTL the entry may still be served while a
	// fresh one is made in the background: stale-while-revalidate.
	Stale time.Duration
	Tags  []string
}

// For is Options with a TTL and tags.
func For(ttl time.Duration, tags ...string) Options { return Options{TTL: ttl, Tags: tags} }

// Entry is a stored value with when it was made and its tags' versions.
type Entry struct {
	Value []byte
	// Made is when it was stored.
	Made time.Time
	// Fresh is false once its TTL passed or a tag was invalidated.
	Fresh bool
	Tags  []string
}

// entry is the stored form: made, TTL, tags with their versions, value.
type entry struct {
	Made int64            `json:"m"`
	TTL  int64            `json:"t,omitempty"`
	Tags map[string]int64 `json:"g,omitempty"`
}

func (c *Cache) tagKey(tag string) string { return c.prefix + "tag:" + tag }

func (c *Cache) tagVersions(ctx context.Context, tags []string) (map[string]int64, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	out := make(map[string]int64, len(tags))
	for _, t := range tags {
		b, ok, err := c.store.Get(ctx, c.tagKey(t))
		if err != nil {
			return nil, err
		}
		var v int64
		if ok {
			v, _ = strconv.ParseInt(string(b), 10, 64)
		}
		out[t] = v
	}
	return out, nil
}

// Lookup returns the entry at key: fresh, stale, or ok false if there's
// none.
func (c *Cache) Lookup(ctx context.Context, key string) (Entry, bool, error) {
	raw, ok, err := c.store.Get(ctx, c.prefix+key)
	if err != nil || !ok {
		return Entry{}, false, err
	}
	meta, value, err := decode(raw)
	if err != nil {
		return Entry{}, false, nil //nolint:nilerr // unreadable: as if missing
	}
	e := Entry{Value: value, Made: time.UnixMilli(meta.Made), Fresh: true}
	if meta.TTL > 0 && time.Since(e.Made) > time.Duration(meta.TTL)*time.Millisecond {
		e.Fresh = false
	}
	if len(meta.Tags) > 0 {
		names := make([]string, 0, len(meta.Tags))
		for t := range meta.Tags {
			names = append(names, t)
		}
		e.Tags = names
		now, err := c.tagVersions(ctx, names)
		if err != nil {
			return Entry{}, false, err
		}
		for t, v := range meta.Tags {
			if now[t] != v {
				e.Fresh = false
			}
		}
	}
	return e, true, nil
}

// Put stores value at key. The store keeps it for TTL plus Stale, so a
// stale entry can be served while a fresh one is made.
func (c *Cache) Put(ctx context.Context, key string, value []byte, o Options) error {
	versions, err := c.tagVersions(ctx, o.Tags)
	if err != nil {
		return err
	}
	return c.put(ctx, key, value, o, versions)
}

func (c *Cache) put(ctx context.Context, key string, value []byte, o Options, versions map[string]int64) error {
	meta := entry{Made: time.Now().UnixMilli(), TTL: o.TTL.Milliseconds(), Tags: versions}
	keep := time.Duration(0)
	if o.TTL > 0 {
		keep = o.TTL + o.Stale
	}
	return c.store.Set(ctx, c.prefix+key, encode(meta, value), keep)
}

// Delete removes entries.
func (c *Cache) Delete(ctx context.Context, keys ...string) error {
	full := make([]string, len(keys))
	for i, k := range keys {
		full[i] = c.prefix + k
	}
	return c.store.Delete(ctx, full...)
}

// Invalidate makes every entry with any of tags stale, in every process
// that shares the store.
func (c *Cache) Invalidate(ctx context.Context, tags ...string) error {
	for _, t := range tags {
		if _, err := c.store.Incr(ctx, c.tagKey(t)); err != nil {
			return err
		}
	}
	return nil
}

// Fetch returns the entry at key if it's fresh; otherwise it calls fn,
// stores what it returns, and returns that. A stale entry within
// o.Stale is returned at once while fn runs in the background. Callers
// for the same key in this process share one call of fn.
func (c *Cache) Fetch(ctx context.Context, key string, o Options, fn func(ctx context.Context) ([]byte, error)) ([]byte, error) {
	e, ok, err := c.Lookup(ctx, key)
	if err == nil && ok && e.Fresh {
		return e.Value, nil
	}
	if err == nil && ok && o.Stale > 0 && o.TTL > 0 && time.Since(e.Made) < o.TTL+o.Stale {
		go func() { _, _ = c.do(context.WithoutCancel(ctx), key, o, fn) }()
		return e.Value, nil
	}
	return c.do(ctx, key, o, fn)
}

// do makes the entry once per key at a time.
func (c *Cache) do(ctx context.Context, key string, o Options, fn func(ctx context.Context) ([]byte, error)) ([]byte, error) {
	return c.Make(ctx, key, o, func(ctx context.Context) ([]byte, []string, error) {
		v, err := fn(ctx)
		return v, nil, err
	})
}

// Make makes the entry at key with fn and stores it, whether or not one
// is there, with o.Tags and the tags fn returns: those found while making
// it, such as what a page's loads read. Callers for the same key in this
// process share one call of fn.
func (c *Cache) Make(ctx context.Context, key string, o Options, fn func(ctx context.Context) ([]byte, []string, error)) ([]byte, error) {
	c.mu.Lock()
	if f, ok := c.flight[key]; ok {
		c.mu.Unlock()
		select {
		case <-f.done:
			return f.value, f.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f := &call{done: make(chan struct{})}
	c.flight[key] = f
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.flight, key)
		c.mu.Unlock()
		close(f.done)
	}()
	// The tags' versions from before making it: an invalidation while it's
	// made leaves the entry stale, not fresh with old data. Tags fn finds
	// can only be read after.
	versions, err := c.tagVersions(ctx, o.Tags)
	if err != nil {
		f.err = err
		return nil, err
	}
	var found []string
	f.value, found, f.err = fn(ctx)
	if f.err != nil {
		if errors.Is(f.err, errNoStore) {
			f.err = nil
		}
		return f.value, f.err
	}
	var extra []string
	for _, t := range found {
		if _, ok := versions[t]; !ok {
			extra = append(extra, t)
		}
	}
	if len(extra) > 0 {
		more, err := c.tagVersions(ctx, extra)
		if err != nil {
			return f.value, nil
		}
		if versions == nil {
			versions = map[string]int64{}
		}
		for t, v := range more {
			versions[t] = v
		}
	}
	if err := c.put(ctx, key, f.value, o, versions); err != nil {
		return f.value, nil // served, just not kept
	}
	return f.value, nil
}

// errNoStore, returned by fn with a value, serves the value without
// storing it.
var errNoStore = errors.New("cache: don't store")

// NoStore is what fn returns with a value to be served but not stored,
// such as an error page.
func NoStore() error { return errNoStore }

// Get returns the cached value of key, decoded from JSON, or calls fn,
// stores its result as JSON, and returns it. An error from fn is returned
// and nothing is stored.
func Get[T any](ctx context.Context, c *Cache, key string, o Options, fn func(ctx context.Context) (T, error)) (T, error) {
	var v T
	b, err := c.Fetch(ctx, key, o, func(ctx context.Context) ([]byte, error) {
		out, err := fn(ctx)
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	})
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return v, fmt.Errorf("cache: decoding %s: %w", key, err)
	}
	return v, nil
}

// encode is the meta's length, the meta as JSON, then the value.
func encode(meta entry, value []byte) []byte {
	m, _ := json.Marshal(meta)
	out := make([]byte, 4, 4+len(m)+len(value))
	binary.BigEndian.PutUint32(out, uint32(len(m)))
	out = append(out, m...)
	return append(out, value...)
}

func decode(raw []byte) (entry, []byte, error) {
	var meta entry
	if len(raw) < 4 {
		return meta, nil, errors.New("cache: short entry")
	}
	n := int(binary.BigEndian.Uint32(raw))
	if 4+n > len(raw) {
		return meta, nil, errors.New("cache: short entry")
	}
	if err := json.Unmarshal(raw[4:4+n], &meta); err != nil {
		return meta, nil, err
	}
	return meta, raw[4+n:], nil
}
