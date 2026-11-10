package ssr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestCompat renders popular Svelte libraries in the server renderer. The
// bundle comes from tests/ssr-compat (pnpm --filter ssr-compat build); the
// test is skipped without it, and CI builds it.
func TestCompat(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	bundle, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "tests", "ssr-compat", ".tern", "output", "server", "server.js"))
	if err != nil {
		if os.Getenv("TERN_REQUIRE_COMPAT") != "" {
			t.Fatal("the compatibility bundle is missing: run pnpm --filter ssr-compat build")
		}
		t.Skip("build tests/ssr-compat first (pnpm --filter ssr-compat build)")
	}
	var logs bytes.Buffer
	p, err := NewPool(string(bundle), Options{Workers: 1, Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })

	res, err := p.Render(context.Background(), []byte(`{"list":true}`), "http://localhost/")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name   string   `json:"name"`
		Expect []string `json:"expect"`
	}
	if err := json.Unmarshal([]byte(res.Body), &cases); err != nil {
		t.Fatal(err)
	}
	// The same bundle rendered in Node, by tests/ssr-compat/node-render.js.
	var node map[string]string
	if b, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "tests", "ssr-compat", ".tern", "output", "node.json")); err == nil {
		if err := json.Unmarshal(b, &node); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			logs.Reset()
			payload, _ := json.Marshal(map[string]string{"case": c.Name})
			res, err := p.Render(context.Background(), payload, "http://localhost/")
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			for _, want := range c.Expect {
				if !strings.Contains(res.Body, want) {
					t.Errorf("missing %q in\n%s", want, truncate(res.Body, 2000))
				}
			}
			if want, ok := node[c.Name]; ok && random.ReplaceAllString(want, "#") != random.ReplaceAllString(res.Body, "#") {
				t.Errorf("differs from Node:\nnode:   %s\ntern:   %s", truncate(want, 1500), truncate(res.Body, 1500))
			}
			if strings.Contains(logs.String(), "level=ERROR") {
				t.Errorf("errors logged while rendering:\n%s", logs.String())
			}
		})
	}
}

// random matches generated IDs, which differ between any two renders.
var random = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|nanoid [0-9]+`)

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("… (%d more bytes)", len(s)-n)
}
