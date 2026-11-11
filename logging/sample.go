package logging

import (
	"hash/maphash"
	"log/slog"
	"sync/atomic"
	"time"
)

// Sampling keeps a burst of log records from drowning the rest and from
// costing what they cost to ship. Within each Tick, the first First records
// with the same level and message are kept, then every Thereafter-th one.
// Records above MaxLevel are always kept.
//
// The default, when [Options.Sampling] is set with zero fields: 100 a
// second, then 1 in 100, for Info and below; warnings and errors are
// never sampled.
type Sampling struct {
	First      int
	Thereafter int // 0 keeps none after the first First
	Tick       time.Duration
	// MaxLevel is the highest level sampled. The zero value is Info.
	MaxLevel slog.Level
}

const sampleSlots = 4096

type sampler struct {
	first, thereafter int64
	tick              int64
	max               slog.Level
	seed              maphash.Seed
	slots             [sampleSlots]sampleCounter
	dropped           atomic.Uint64
}

type sampleCounter struct {
	resetAt atomic.Int64
	n       atomic.Int64
}

func newSampler(s *Sampling) *sampler {
	if s == nil {
		return nil
	}
	first, thereafter, tick := s.First, s.Thereafter, s.Tick
	if first == 0 && thereafter == 0 && tick == 0 {
		first, thereafter = 100, 100
	}
	if tick <= 0 {
		tick = time.Second
	}
	return &sampler{first: int64(first), thereafter: int64(thereafter), tick: int64(tick), max: s.MaxLevel, seed: maphash.MakeSeed()}
}

// keep reports whether to log a record. Records with different messages
// can share a counter (the table has a fixed size); they are then sampled
// together, which keeps memory bounded whatever the messages.
func (s *sampler) keep(level slog.Level, msg string, t time.Time) bool {
	if s == nil || level > s.max {
		return true
	}
	var h maphash.Hash
	h.SetSeed(s.seed)
	_, _ = h.WriteString(msg)
	_ = h.WriteByte(byte(level))
	c := &s.slots[h.Sum64()%sampleSlots]
	n := c.inc(t.UnixNano(), s.tick)
	if n <= s.first || (s.thereafter > 0 && (n-s.first)%s.thereafter == 0) {
		return true
	}
	s.dropped.Add(1)
	return false
}

func (c *sampleCounter) inc(now, tick int64) int64 {
	reset := c.resetAt.Load()
	if reset > now {
		return c.n.Add(1)
	}
	if c.resetAt.CompareAndSwap(reset, now+tick) {
		c.n.Store(1)
		return 1
	}
	return c.n.Add(1)
}
