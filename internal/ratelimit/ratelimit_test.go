package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/Unfield/Tern/cache"
)

func TestLimiter(t *testing.T) {
	now := time.Unix(0, 0)
	l := New(1, 2) // one a second, two at once
	l.Now = func() time.Time { return now }
	for i := range 2 {
		if _, ok := l.Take(context.Background(), "a"); !ok {
			t.Fatalf("take %d refused", i+1)
		}
	}
	wait, ok := l.Take(context.Background(), "a")
	if ok || wait != time.Second {
		t.Fatalf("third take: %v, %v; want refused for 1s", wait, ok)
	}
	if _, ok := l.Take(context.Background(), "b"); !ok {
		t.Fatal("another key was refused")
	}
	now = now.Add(time.Second)
	if _, ok := l.Take(context.Background(), "a"); !ok {
		t.Fatal("refused after refilling")
	}
	now = now.Add(time.Minute)
	l.sweep()
	if len(l.buckets) != 0 {
		t.Errorf("full buckets kept: %d", len(l.buckets))
	}
}

func TestStore(t *testing.T) {
	ctx := context.Background()
	store := cache.Memory(1 << 20)
	now := time.Unix(960_000, 0)          // on a minute, where a window starts
	l := NewStore(store, "t:", 3.0/60, 3) // 3 a minute
	l.Now = func() time.Time { return now }
	for i := range 3 {
		if _, ok := l.Take(ctx, "a"); !ok {
			t.Fatalf("take %d refused", i+1)
		}
	}
	now = now.Add(20 * time.Second)
	wait, ok := l.Take(ctx, "a")
	if ok || wait != 40*time.Second {
		t.Fatalf("the fourth in the minute: ok %v, wait %v", ok, wait)
	}
	if _, ok := l.Take(ctx, "b"); !ok {
		t.Fatal("another key has its own count")
	}
	// Another instance on the same store shares the count.
	other := NewStore(store, "t:", 3.0/60, 3)
	other.Now = l.Now
	if _, ok := other.Take(ctx, "a"); ok {
		t.Fatal("a second instance didn't see the count")
	}
	now = now.Add(40 * time.Second)
	if _, ok := l.Take(ctx, "a"); !ok {
		t.Fatal("the next window starts afresh")
	}
}
