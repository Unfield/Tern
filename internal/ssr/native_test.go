package ssr

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Unfield/Tern/native/js"
)

// engineBundle is what the QuickJS fallback renders.
const engineBundle = `globalThis.__tern_render = (p, href) => JSON.stringify({ head: "", body: "engine " + p })`

// fakeRealm stands for compiled modules: __tern_render is a Go function.
func fakeRealm(render func(rt *js.Realm, payload string) js.Value) NewRealm {
	return func(setup func(*js.Realm)) *js.Realm {
		rt := js.NewRealm()
		setup(rt)
		rt.Global.DefineHidden("__tern_render", rt.NewFunction("__tern_render", 2, func(_ js.Value, args []js.Value) js.Value {
			return render(rt, js.ToString(js.Arg(args, 0)))
		}))
		return rt
	}
}

func TestNativeRenders(t *testing.T) {
	r, err := NewNative(fakeRealm(func(rt *js.Realm, p string) js.Value {
		if p == "unsupported" {
			panic(js.NotSupported("an async function"))
		}
		if p == "throws" {
			panic(js.TypeErrorf("boom"))
		}
		// Host functions are there, as in the engine's VMs.
		js.Call(rt.Global.Get("__tern_log"), js.Undefined, "info", "hello from "+p)
		return `{"head":"","body":"native ` + p + `"}`
	}), engineBundle, Options{Workers: 2, Logger: slog.New(slog.DiscardHandler)}, NativeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.(*Native).Close() }()
	for payload, want := range map[string]string{"a": "native a", "unsupported": "engine unsupported", "throws": "engine throws"} {
		res, err := r.Render(context.Background(), []byte(payload), "http://x/"+payload)
		if err != nil || res.Body != want {
			t.Errorf("%s: got %q, %v; want %q", payload, res.Body, err, want)
		}
	}
	// A path that fell back goes to the engine from then on.
	if _, ok := r.(*Native).engineOnly.Load("/unsupported"); !ok {
		t.Error("the path that fell back isn't remembered")
	}
}

func TestNativeTimeout(t *testing.T) {
	r, err := NewNative(fakeRealm(func(rt *js.Realm, p string) js.Value {
		if p == "loop" {
			for {
				rt.Tick()
			}
		}
		return `{"head":"","body":"ok"}`
	}), engineBundle, Options{Workers: 1, Timeout: 50 * time.Millisecond, Logger: slog.New(slog.DiscardHandler)}, NativeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Render(context.Background(), []byte("loop"), "http://x/"); !errors.Is(err, ErrTimeout) {
		t.Fatalf("got %v, want ErrTimeout", err)
	}
	// The interrupted realm was replaced.
	if res, err := r.Render(context.Background(), []byte("x"), "http://x/"); err != nil || res.Body != "ok" {
		t.Fatalf("after a timeout: %q, %v", res.Body, err)
	}
}

func TestNativeVerify(t *testing.T) {
	var logs bytes.Buffer
	r, err := NewNative(fakeRealm(func(_ *js.Realm, p string) js.Value {
		return `{"head":"","body":"engine ` + strings.ToUpper(p) + `"}`
	}), engineBundle, Options{Workers: 1, Logger: slog.New(slog.NewTextHandler(&logs, nil))}, NativeOptions{Verify: true})
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.Render(context.Background(), []byte("x"), "http://x/")
	if err != nil || res.Body != "engine x" {
		t.Fatalf("verify mode returns the engine's result: %q, %v", res.Body, err)
	}
	if !strings.Contains(logs.String(), "differs") {
		t.Fatalf("no difference logged: %s", logs.String())
	}
}

func TestNativeBrokenModules(t *testing.T) {
	_, err := NewNative(func(func(*js.Realm)) *js.Realm { panic(js.TypeErrorf("module init failed")) }, engineBundle, Options{}, NativeOptions{})
	if err == nil || !strings.Contains(err.Error(), "module init failed") {
		t.Fatalf("got %v", err)
	}
}
