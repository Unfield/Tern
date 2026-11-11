package cache_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Unfield/Tern/cache"
	"github.com/Unfield/Tern/cache/redis"
)

// stores runs fn with each store: memory and disk always, and Redis when
// TERN_TEST_REDIS holds its URL.
func stores(t *testing.T, fn func(t *testing.T, s cache.Store)) {
	t.Run("memory", func(t *testing.T) { fn(t, cache.Memory(1<<20)) })
	t.Run("disk", func(t *testing.T) {
		s, err := cache.Disk(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		fn(t, s)
	})
	if u := os.Getenv("TERN_TEST_REDIS"); u != "" {
		t.Run("redis", func(t *testing.T) {
			s, err := redis.New(u)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			if err := s.Ping(context.Background()); err != nil {
				t.Fatal(err)
			}
			fn(t, s)
		})
	}
}

// unique keeps tests on a shared Redis apart.
func unique(t *testing.T, key string) string {
	return fmt.Sprintf("%s:%d:%s", t.Name(), time.Now().UnixNano(), key)
}

func TestStores(t *testing.T) {
	stores(t, func(t *testing.T, s cache.Store) {
		ctx := context.Background()
		k := unique(t, "k")
		if _, ok, err := s.Get(ctx, k); ok || err != nil {
			t.Fatalf("missing: %v %v", ok, err)
		}
		if err := s.Set(ctx, k, []byte("v\x00binary\r\n"), 0); err != nil {
			t.Fatal(err)
		}
		if v, ok, err := s.Get(ctx, k); !ok || err != nil || string(v) != "v\x00binary\r\n" {
			t.Fatalf("get: %q %v %v", v, ok, err)
		}
		short := unique(t, "short")
		_ = s.Set(ctx, short, []byte("x"), 30*time.Millisecond)
		time.Sleep(60 * time.Millisecond)
		if _, ok, _ := s.Get(ctx, short); ok {
			t.Error("an expired value is still there")
		}
		n := unique(t, "n")
		for want := int64(1); want <= 3; want++ {
			if got, err := s.Incr(ctx, n); err != nil || got != want {
				t.Fatalf("incr: %d %v", got, err)
			}
		}
		if err := s.Delete(ctx, k, n); err != nil {
			t.Fatal(err)
		}
		if _, ok, _ := s.Get(ctx, k); ok {
			t.Error("deleted value is still there")
		}
	})
}

func TestTagsAndTTL(t *testing.T) {
	stores(t, func(t *testing.T, s cache.Store) {
		ctx := context.Background()
		c := cache.New(s)
		tag := unique(t, "posts")
		k := unique(t, "recent")
		var calls atomic.Int32
		get := func() []string {
			v, err := cache.Get(ctx, c, k, cache.For(time.Hour, tag), func(context.Context) ([]string, error) {
				calls.Add(1)
				return []string{"a", fmt.Sprint(calls.Load())}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			return v
		}
		if v := get(); v[1] != "1" {
			t.Fatalf("first %v", v)
		}
		if v := get(); v[1] != "1" || calls.Load() != 1 {
			t.Fatalf("cached %v, %d calls", v, calls.Load())
		}
		if err := c.Invalidate(ctx, tag); err != nil {
			t.Fatal(err)
		}
		if v := get(); v[1] != "2" {
			t.Fatalf("after invalidating %v", v)
		}
		// Another Cache on the same store sees the invalidation: what
		// another process does.
		other := cache.New(s)
		if err := other.Invalidate(ctx, tag); err != nil {
			t.Fatal(err)
		}
		if e, ok, _ := c.Lookup(ctx, k); !ok || e.Fresh {
			t.Fatalf("invalidated elsewhere, still fresh: %v %v", ok, e.Fresh)
		}

		// A short TTL, and stale-while-revalidate.
		k2 := unique(t, "swr")
		var n atomic.Int32
		o := cache.Options{TTL: 40 * time.Millisecond, Stale: time.Minute}
		fetch := func() string {
			b, err := c.Fetch(ctx, k2, o, func(context.Context) ([]byte, error) {
				return fmt.Appendf(nil, "v%d", n.Add(1)), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
		if v := fetch(); v != "v1" {
			t.Fatal(v)
		}
		time.Sleep(60 * time.Millisecond)
		if v := fetch(); v != "v1" {
			t.Fatalf("stale is served at once: %s", v)
		}
		deadline := time.Now().Add(2 * time.Second)
		// Any newer value: with a 40ms TTL, a slow machine may refresh it
		// more than once before the loop looks.
		for fetch() == "v1" {
			if time.Now().After(deadline) {
				t.Fatal("the background refresh never landed")
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
}

func TestOneCallPerKey(t *testing.T) {
	c := cache.New(cache.Memory(1 << 20))
	var calls atomic.Int32
	release := make(chan struct{})
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			v, err := c.Fetch(context.Background(), "k", cache.For(time.Minute), func(context.Context) ([]byte, error) {
				calls.Add(1)
				<-release
				return []byte("v"), nil
			})
			if err != nil || string(v) != "v" {
				t.Errorf("%q %v", v, err)
			}
		})
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Fatalf("made %d times for 20 callers", n)
	}
}

func TestErrorsAndNoStore(t *testing.T) {
	ctx := context.Background()
	c := cache.New(cache.Memory(1 << 20))
	boom := errors.New("boom")
	if _, err := c.Fetch(ctx, "k", cache.For(time.Minute), func(context.Context) ([]byte, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if _, ok, _ := c.Lookup(ctx, "k"); ok {
		t.Error("a failure was stored")
	}
	v, err := c.Fetch(ctx, "k", cache.For(time.Minute), func(context.Context) ([]byte, error) { return []byte("once"), cache.NoStore() })
	if err != nil || string(v) != "once" {
		t.Fatalf("%q %v", v, err)
	}
	if _, ok, _ := c.Lookup(ctx, "k"); ok {
		t.Error("NoStore stored it")
	}
}

func TestMemoryEvicts(t *testing.T) {
	ctx := context.Background()
	s := cache.Memory(100)
	for i := range 10 {
		_ = s.Set(ctx, fmt.Sprint("k", i), make([]byte, 30), 0)
	}
	if _, ok, _ := s.Get(ctx, "k0"); ok {
		t.Error("the oldest wasn't evicted")
	}
	if _, ok, _ := s.Get(ctx, "k9"); !ok {
		t.Error("the newest was evicted")
	}
	for range 1000 {
		_, _ = s.Incr(ctx, "tag")
	}
	_ = s.Set(ctx, "big", make([]byte, 90), 0)
	if v, _ := s.Incr(ctx, "tag"); v != 1001 {
		t.Errorf("a counter was evicted: %d", v)
	}
}
