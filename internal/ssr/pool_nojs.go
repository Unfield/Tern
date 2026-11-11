//go:build tern_nojs

package ssr

import (
	"context"
	"errors"
)

// Built with the tern_nojs tag, the binary has no JavaScript engine:
// tern build leaves it out when the compiler renders every page natively.
// A render that would need it fails, and the page renders in the browser.

// ErrNoEngine is what renders that need the JavaScript engine return in a
// binary built without it.
var ErrNoEngine = errors.New("ssr: this binary has no JavaScript engine (tern build left it out, as every page compiled to Go); rebuild with TERN_KEEP_ENGINE=1 to keep it")

// Pool stands in for the engine's pool; every call fails.
type Pool struct{}

// NewPool fails: there's no engine.
func NewPool(string, Options) (*Pool, error) { return nil, ErrNoEngine }

// Render implements Renderer.
func (*Pool) Render(context.Context, []byte, string) (Result, error) { return Result{}, ErrNoEngine }

// Email implements EmailRenderer.
func (*Pool) Email(context.Context, string, []byte) (EmailResult, []byte, error) {
	return EmailResult{}, nil, ErrNoEngine
}

// Close implements io.Closer.
func (*Pool) Close() error { return nil }
