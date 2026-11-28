// Package ratelimit holds the token buckets behind kit's default rate limit
// and plugins/ratelimit: per key, up to burst requests at once, refilled at
// a steady rate.
package ratelimit

import (
	"context"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/Unfield/Tern/cache"
)

// A Taker limits how often each key may act.
type Taker interface {
	// Take spends one action for k, or reports how long until k may act.
	Take(ctx context.Context, k string) (time.Duration, bool)
}

// Limiter limits how often each key may act. It's safe for concurrent use.
type Limiter struct {
	rate, burst float64

	mu      sync.Mutex
	buckets map[string]*bucket
	// Now is the clock; tests replace it.
	Now func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// New returns a Limiter allowing perSecond actions a second, in bursts of
// up to burst.
func New(perSecond, burst float64) *Limiter {
	return &Limiter{rate: perSecond, burst: burst, buckets: map[string]*bucket{}, Now: time.Now}
}

// Take spends a token for k, or reports how long until one is available.
func (l *Limiter) Take(_ context.Context, k string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.Now()
	b := l.buckets[k]
	if b == nil {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[k] = b
	}
	b.tokens = math.Min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return 0, true
	}
	return time.Duration((1 - b.tokens) / l.rate * float64(time.Second)), false
}

// Sweep drops buckets that have refilled completely, every minute until
// stop is closed, so memory stays bounded.
func (l *Limiter) Sweep(stop <-chan struct{}) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			l.sweep()
		}
	}
}

func (l *Limiter) sweep() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.Now()
	for k, b := range l.buckets {
		if b.tokens+now.Sub(b.last).Seconds()*l.rate >= l.burst {
			delete(l.buckets, k)
		}
	}
}

// Store counts in a cache.Store, so that every instance of an app shares
// the counts and they outlast a restart. It allows burst actions per key in
// each window of burst/perSecond seconds: a fixed window, since a store
// only has to count.
type Store struct {
	store  cache.Store
	prefix string
	limit  int64
	window time.Duration
	// Now is the clock; tests replace it.
	Now func() time.Time
	// OnError is told when the store fails; the action is then allowed.
	OnError func(error)
}

// NewStore returns a limiter counting in store under keys that start with
// prefix.
func NewStore(store cache.Store, prefix string, perSecond, burst float64) *Store {
	window := time.Duration(math.Max(1, math.Round(burst/perSecond))) * time.Second
	return &Store{store: store, prefix: prefix, limit: int64(math.Max(1, burst)), window: window, Now: time.Now}
}

// Take implements Taker.
func (s *Store) Take(ctx context.Context, k string) (time.Duration, bool) {
	now := s.Now()
	win := now.UnixNano() / int64(s.window)
	n, err := cache.IncrTTL(ctx, s.store, s.prefix+k+":"+strconv.FormatInt(win, 10), s.window+time.Second)
	if err != nil {
		if s.OnError != nil {
			s.OnError(err)
		}
		return 0, true
	}
	if n <= s.limit {
		return 0, true
	}
	return time.Unix(0, (win+1)*int64(s.window)).Sub(now), false
}
