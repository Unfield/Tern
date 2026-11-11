package file

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Unfield/Tern/logging/internal/sinktest"
)

func TestWriteAndRotate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs", "app.log")
	s := New(Options{Path: path, MaxSize: 300, Keep: 2, Compress: true})
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	for range 6 {
		if err := s.Export(context.Background(), sinktest.Entries()); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var e map[string]any
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			t.Fatalf("%q: %v", l, err)
		}
		if e["msg"] == nil || e["level"] == nil {
			t.Fatalf("entry %v", e)
		}
	}
	rotated, _ := filepath.Glob(filepath.Join(dir, "logs", "app-*.log.gz"))
	if len(rotated) != 2 {
		t.Fatalf("rotated files %v", rotated)
	}
	// The kept ones are the newest, and are gzipped JSON lines.
	if !strings.HasSuffix(rotated[1], "app-20261111T120000.010.log.gz") {
		t.Fatalf("names %v", rotated)
	}
	f, _ := os.Open(rotated[1])
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(zr)
	if !strings.Contains(string(data), `"msg":"order placed"`) {
		t.Fatalf("rotated content %q", data)
	}
}

func TestReopenAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	for range 2 {
		s := New(Options{Path: path})
		if err := s.Export(context.Background(), sinktest.Entries()[:1]); err != nil {
			t.Fatal(err)
		}
		_ = s.Close()
	}
	b, _ := os.ReadFile(path)
	if n := strings.Count(string(b), "\n"); n != 2 {
		t.Fatalf("%d lines", n)
	}
}
