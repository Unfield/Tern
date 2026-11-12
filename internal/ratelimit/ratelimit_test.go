package ratelimit

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	now := time.Unix(0, 0)
	l := New(1, 2) // one a second, two at once
	l.Now = func() time.Time { return now }
	for i := range 2 {
		if _, ok := l.Take("a"); !ok {
			t.Fatalf("take %d refused", i+1)
		}
	}
	wait, ok := l.Take("a")
	if ok || wait != time.Second {
		t.Fatalf("third take: %v, %v; want refused for 1s", wait, ok)
	}
	if _, ok := l.Take("b"); !ok {
		t.Fatal("another key was refused")
	}
	now = now.Add(time.Second)
	if _, ok := l.Take("a"); !ok {
		t.Fatal("refused after refilling")
	}
	now = now.Add(time.Minute)
	l.sweep()
	if len(l.buckets) != 0 {
		t.Errorf("full buckets kept: %d", len(l.buckets))
	}
}
