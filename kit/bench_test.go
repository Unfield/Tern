package kit

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func benchApp(b *testing.B, opts ...Option) *App {
	b.Helper()
	opts = append([]Option{WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))}, opts...)
	a, err := New(testManifest(), opts...)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = a.Close() })
	return a
}

func benchRequest(b *testing.B, h http.Handler, target string) {
	b.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				b.Fatalf("status %d", rec.Code)
			}
		}
	})
}

// BenchmarkDataRequest is a client-side navigation: two layout loads and a
// page load, encoded as JSON.
func BenchmarkDataRequest(b *testing.B) {
	benchRequest(b, benchApp(b), "http://example.com/blog/hello/__data.json")
}

// BenchmarkDocument is a full page load rendered on the client.
func BenchmarkDocument(b *testing.B) {
	benchRequest(b, benchApp(b), "http://example.com/blog/hello")
}

// BenchmarkDocumentSSR is a full page load with a (small) server render.
func BenchmarkDocumentSSR(b *testing.B) {
	m := testManifest()
	m.ServerBundle = fakeServerBundle
	a, err := New(m, WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = a.Close() })
	benchRequest(b, a, "http://example.com/blog/hello")
}
