// Package ratelimit holds the token buckets behind kit's default rate limit
// and plugins/ratelimit: per key, up to burst requests at once, refilled at
// a steady rate.
package ratelimit

import (
	"math"
	"sync"
	"time"
)

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
func (l *Limiter) Take(k string) (time.Duration, bool) {
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
