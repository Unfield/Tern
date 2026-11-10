package ssr

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type countingRenderer struct {
	calls atomic.Int32
	delay time.Duration
	fail  bool
}

func (r *countingRenderer) Render(ctx context.Context, payload []byte, href string) (Result, error) {
	r.calls.Add(1)
	time.Sleep(r.delay)
	if r.fail {
		return Result{}, errors.New("boom")
	}
	return Result{Head: "h", Body: string(payload) + href}, nil
}

func TestCacheHitsAndKeys(t *testing.T) {
	r := &countingRenderer{}
	c := NewCache(r, 1<<20)
	ctx := context.Background()
	for range 3 {
		res, err := c.Render(ctx, []byte(`{"a":1}`), "http://x/")
		if err != nil || res.Body != `{"a":1}http://x/` {
			t.Fatalf("got %+v, %v", res, err)
		}
	}
	if _, err := c.Render(ctx, []byte(`{"a":1}`), "http://x/?q"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Render(ctx, []byte(`{"a":2}`), "http://x/"); err != nil {
		t.Fatal(err)
	}
	if n := r.calls.Load(); n != 3 {
		t.Errorf("renders = %d, want 3 (one per distinct payload and URL)", n)
	}
	if hits, misses, _ := c.Stats(); hits != 2 || misses != 3 {
		t.Errorf("hits, misses = %d, %d", hits, misses)
	}
}

func TestCacheCoalescesAndSkipsErrors(t *testing.T) {
	r := &countingRenderer{delay: 50 * time.Millisecond}
	c := NewCache(r, 1<<20)
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if _, err := c.Render(context.Background(), []byte("p"), "u"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if n := r.calls.Load(); n != 1 {
		t.Errorf("concurrent renders = %d, want 1", n)
	}

	f := &countingRenderer{fail: true}
	fc := NewCache(f, 1<<20)
	for range 2 {
		if _, err := fc.Render(context.Background(), []byte("p"), "u"); err == nil {
			t.Fatal("want an error")
		}
	}
	if n := f.calls.Load(); n != 2 {
		t.Errorf("failed renders are not cached: calls = %d", n)
	}
}

func TestCacheEvicts(t *testing.T) {
	r := &countingRenderer{}
	c := NewCache(r, 3200)
	big := strings.Repeat("x", 200)
	for i := range 20 {
		if _, err := c.Render(context.Background(), []byte(big), string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, size := c.Stats(); size > 3200 {
		t.Errorf("size %d over the budget", size)
	}
	// The oldest entry was evicted, the newest is still there.
	before := r.calls.Load()
	_, _ = c.Render(context.Background(), []byte(big), string(rune('a'+19)))
	_, _ = c.Render(context.Background(), []byte(big), "a")
	if got := r.calls.Load() - before; got != 1 {
		t.Errorf("renders after eviction = %d, want 1", got)
	}
}
