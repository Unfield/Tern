package ssr

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// BenchmarkRenderDashboard renders the dashboard example's login page,
// built from real shadcn-svelte components. Build the example first
// (tern build in examples/dashboard); the benchmark is skipped otherwise.
func BenchmarkRenderDashboard(b *testing.B) {
	_, here, _, _ := runtime.Caller(0)
	bundle, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "examples", "dashboard", ".tern", "output", "server", "server.js"))
	if err != nil {
		b.Skip("build examples/dashboard first")
	}
	p, err := NewPool(string(bundle), Options{})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = p.Close() })
	payload := []byte(`{"type":"data","route":"/login","status":200,"nodes":[{},{}]}`)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			res, err := p.Render(context.Background(), payload, "http://localhost/login")
			if err != nil || res.Body == "" {
				b.Fatalf("render: %v", err)
			}
		}
	})
}

// BenchmarkRenderSmall measures the pool's own overhead with a trivial bundle.
func BenchmarkRenderSmall(b *testing.B) {
	p, err := NewPool(testBundle, Options{})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = p.Close() })
	payload := []byte(`{"title":"x"}`)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := p.Render(context.Background(), payload, "http://localhost/"); err != nil {
				b.Fatal(err)
			}
		}
	})
}
