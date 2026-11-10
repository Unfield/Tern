package ssr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

const testBundle = `
globalThis.__tern_render = (payload, href) => {
	const p = JSON.parse(payload);
	if (p.loop) { for (;;) {} }
	if (p.throw) { throw new Error('render failed: ' + p.throw); }
	if (p.log) { __tern_log('warn', 'from js: ' + p.log); }
	if (p.skip) { return JSON.stringify({ skip: true }); }
	return JSON.stringify({ head: '<title>' + p.title + '</title>', body: '<p>' + href + '</p>' });
};
`

func newTestPool(t *testing.T, opts Options) *Pool {
	t.Helper()
	p, err := NewPool(testBundle, opts)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestRender(t *testing.T) {
	p := newTestPool(t, Options{Workers: 2})
	res, err := p.Render(context.Background(), []byte(`{"title":"Hi"}`), "https://example.com/x")
	if err != nil {
		t.Fatal(err)
	}
	if res.Head != "<title>Hi</title>" || res.Body != "<p>https://example.com/x</p>" || res.Skip {
		t.Errorf("result = %+v", res)
	}
	res, err = p.Render(context.Background(), []byte(`{"skip":true}`), "https://example.com/")
	if err != nil || !res.Skip {
		t.Errorf("skip result = %+v, %v", res, err)
	}
}

func TestRenderConcurrently(t *testing.T) {
	p := newTestPool(t, Options{Workers: 4})
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := range 64 {
		wg.Go(func() {
			title := fmt.Sprintf("page-%d", i)
			res, err := p.Render(context.Background(), []byte(`{"title":"`+title+`"}`), "https://example.com/")
			if err != nil {
				errs <- err
				return
			}
			if res.Head != "<title>"+title+"</title>" {
				errs <- fmt.Errorf("got %q for %s", res.Head, title)
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestTimeoutReplacesVM(t *testing.T) {
	p := newTestPool(t, Options{Workers: 1, Timeout: 100 * time.Millisecond})
	start := time.Now()
	_, err := p.Render(context.Background(), []byte(`{"loop":true}`), "https://example.com/")
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("timeout took %v", d)
	}
	// The pool recovers with a fresh VM.
	res, err := p.Render(context.Background(), []byte(`{"title":"after"}`), "https://example.com/")
	if err != nil || res.Head != "<title>after</title>" {
		t.Fatalf("after timeout: %+v, %v", res, err)
	}
}

func TestRenderError(t *testing.T) {
	p := newTestPool(t, Options{Workers: 1})
	_, err := p.Render(context.Background(), []byte(`{"throw":"boom"}`), "https://example.com/")
	if err == nil || !strings.Contains(err.Error(), "render failed: boom") {
		t.Fatalf("err = %v", err)
	}
	if _, err := p.Render(context.Background(), []byte(`{"title":"ok"}`), "https://example.com/"); err != nil {
		t.Fatalf("pool did not recover: %v", err)
	}
}

func TestConsoleLogging(t *testing.T) {
	var buf bytes.Buffer
	p := newTestPool(t, Options{Workers: 1, Logger: slog.New(slog.NewTextHandler(&buf, nil))})
	if _, err := p.Render(context.Background(), []byte(`{"title":"x","log":"hello"}`), "https://example.com/"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "from js: hello") || !strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("log = %q", buf.String())
	}
}

func TestContextCanceledWhileWaiting(t *testing.T) {
	p := newTestPool(t, Options{Workers: 1, Timeout: time.Second})
	go func() { _, _ = p.Render(context.Background(), []byte(`{"loop":true}`), "https://example.com/") }()
	time.Sleep(20 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := p.Render(ctx, []byte(`{"title":"x"}`), "https://example.com/"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want deadline exceeded", err)
	}
}

func TestBadBundles(t *testing.T) {
	if _, err := NewPool("this is not javascript (", Options{}); err == nil {
		t.Error("syntax error should fail")
	}
	if _, err := NewPool("globalThis.x = 1;", Options{}); err == nil || !strings.Contains(err.Error(), "__tern_render") {
		t.Errorf("missing render function: %v", err)
	}
}

func TestLoadErrorIncludesStack(t *testing.T) {
	_, err := NewPool("function boom() { undefinedThing(); }\nboom();\nglobalThis.__tern_render = () => '{}';", Options{})
	if err == nil || !strings.Contains(err.Error(), "at boom") {
		t.Fatalf("err = %v, want a JavaScript stack trace", err)
	}
}

func TestRetiresVMs(t *testing.T) {
	p := newTestPool(t, Options{Workers: 1, MaxRenders: 3})
	for i := range 10 {
		res, err := p.Render(context.Background(), []byte(`{"title":"x"}`), fmt.Sprintf("https://x/%d", i))
		if err != nil {
			t.Fatalf("render %d: %v", i, err)
		}
		if !strings.Contains(res.Body, fmt.Sprintf("/%d", i)) {
			t.Fatalf("render %d: %q", i, res.Body)
		}
	}
}
