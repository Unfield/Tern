package goscript

import (
	"context"
	"runtime"
	"sync"
	"testing"
)

func benchComponent() {
	Register("bench", func(c *Component) {
		n := State(c, "n", 0)
		Derived(c, "doubled", func() int { return n.Get() * 2 })
		c.Func("inc", func() int { n.Update(func(v int) int { return v + 1 }); return n.Peek() })
	})
}

// BenchmarkLiveCall measures a call through a live instance: the Func, its
// state change and the emitted values, as the live endpoint drives it.
func BenchmarkLiveCall(b *testing.B) {
	benchComponent()
	inst, err := MountLive(context.Background(), "bench", LiveOptions{Emit: func(string, []byte) {}})
	if err != nil {
		b.Fatal(err)
	}
	defer inst.Destroy()
	ctx := context.Background()
	for b.Loop() {
		if _, err := inst.Call(ctx, "inc", nil); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLiveCallParallel runs calls on many instances at once.
func BenchmarkLiveCallParallel(b *testing.B) {
	benchComponent()
	b.RunParallel(func(pb *testing.PB) {
		inst, err := MountLive(context.Background(), "bench", LiveOptions{Emit: func(string, []byte) {}})
		if err != nil {
			b.Error(err)
			return
		}
		defer inst.Destroy()
		for pb.Next() {
			_, _ = inst.Call(context.Background(), "inc", nil)
		}
	})
}

// TestLiveMemory reports the memory a mounted instance holds.
func TestLiveMemory(t *testing.T) {
	if testing.Short() || !testing.Verbose() {
		t.Skip("run with -v to measure")
	}
	benchComponent()
	const n = 10000
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	insts := make([]*Instance, n)
	for i := range n {
		insts[i], _ = MountLive(context.Background(), "bench", LiveOptions{Emit: func(string, []byte) {}})
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	t.Logf("%d instances: %.1f KiB heap and %.1f KiB stack each", n, float64(after.HeapAlloc-before.HeapAlloc)/n/1024, float64(after.StackInuse-before.StackInuse)/n/1024)
	var wg sync.WaitGroup
	for _, inst := range insts {
		wg.Go(inst.Destroy)
	}
	wg.Wait()
}
