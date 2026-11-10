package ssr

import (
	"container/list"
	"context"
	"crypto/sha256"
	"sync"

	"golang.org/x/sync/singleflight"
)

// Cache remembers renders by payload and URL. A render is a pure function
// of both: the browser hydrates from the same payload and must produce the
// same HTML, so a component whose output depends on anything else (the
// clock, randomness) would not hydrate cleanly either. Pages that look the
// same for many visitors, such as anonymous pages, then render once.
//
// Concurrent renders of the same page are coalesced into one.
type Cache struct {
	next     Renderer
	maxBytes int

	mu    sync.Mutex
	size  int
	lru   *list.List // of *entry, most recently used first
	items map[[32]byte]*list.Element
	group singleflight.Group

	hits, misses uint64
}

type entry struct {
	key  [32]byte
	res  Result
	size int
}

// NewCache wraps next with a cache of at most maxBytes of HTML.
func NewCache(next Renderer, maxBytes int) *Cache {
	return &Cache{next: next, maxBytes: maxBytes, lru: list.New(), items: map[[32]byte]*list.Element{}}
}

// Render returns a cached render, or renders and caches it.
func (c *Cache) Render(ctx context.Context, payload []byte, href string) (Result, error) {
	h := sha256.New()
	h.Write([]byte(href))
	h.Write([]byte{0})
	h.Write(payload)
	var key [32]byte
	h.Sum(key[:0])

	c.mu.Lock()
	if el, ok := c.items[key]; ok {
		c.lru.MoveToFront(el)
		c.hits++
		res := el.Value.(*entry).res
		c.mu.Unlock()
		return res, nil
	}
	c.misses++
	c.mu.Unlock()

	v, err, _ := c.group.Do(string(key[:]), func() (any, error) {
		// Shared by every waiting request, so one leaving does not cancel it.
		res, err := c.next.Render(context.WithoutCancel(ctx), payload, href)
		if err != nil {
			return Result{}, err
		}
		c.add(key, res)
		return res, nil
	})
	if err != nil {
		return Result{}, err
	}
	return v.(Result), nil
}

func (c *Cache) add(key [32]byte, res Result) {
	size := len(res.Head) + len(res.Body) + 128
	if size > c.maxBytes/8 {
		return // one huge page should not evict everything else
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[key]; ok {
		return
	}
	c.items[key] = c.lru.PushFront(&entry{key: key, res: res, size: size})
	c.size += size
	for c.size > c.maxBytes {
		last := c.lru.Back()
		e := last.Value.(*entry)
		c.lru.Remove(last)
		delete(c.items, e.key)
		c.size -= e.size
	}
}

// Next returns the renderer the cache wraps.
func (c *Cache) Next() Renderer { return c.next }

// Stats reports cache hits, misses, and the bytes cached.
func (c *Cache) Stats() (hits, misses uint64, bytes int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses, c.size
}
