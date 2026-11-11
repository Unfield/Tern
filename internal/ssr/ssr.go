// Package ssr renders Svelte pages on the server by running the app's
// server bundle in embedded QuickJS VMs. It is pure Go: no cgo and no Node.
package ssr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Result is a rendered page.
type Result struct {
	Head string `json:"head"`
	Body string `json:"body"`
	// Skip reports that the payload is not something the server renders.
	Skip bool `json:"skip"`
}

// Renderer renders a page payload. href is the page's absolute URL.
type Renderer interface {
	Render(ctx context.Context, payload []byte, href string) (Result, error)
}

// Options configure a Pool.
type Options struct {
	// Workers is the number of VMs, and so the number of concurrent
	// renders. The default is GOMAXPROCS, at most DefaultMaxWorkers.
	Workers int
	// Timeout bounds a single render. The default is 5 seconds.
	Timeout time.Duration
	// MemoryLimit caps each VM's heap, in bytes. The default is 256 MiB.
	MemoryLimit uintptr
	// Logger receives console output from server-side code.
	Logger *slog.Logger
	// Funcs are Go functions the bundle can call while rendering, by name;
	// ctx is the render's context.
	Funcs map[string]func(ctx context.Context, args []any) (any, error)
	// MaxRenders retires a VM after this many renders and replaces it with
	// a fresh one, built in the background. It bounds memory that libraries
	// keep across renders. The default is 10,000; a negative value never
	// retires VMs.
	MaxRenders int
}

// DefaultMaxWorkers caps the default number of VMs. The VMs share the C
// runtime's allocator, which takes a global lock on every JavaScript
// function call, so throughput peaks around four concurrent renders and
// falls with more (see docs/benchmarks.md).
const DefaultMaxWorkers = 4

// ErrTimeout is returned when a render exceeds the pool's timeout.
var ErrTimeout = errors.New("ssr: render timed out")

// EmailResult is a rendered email template.
type EmailResult struct {
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	Error   string `json:"error,omitempty"`
}

// EmailRenderer renders the app's email templates (src/emails). An empty
// name returns the template names with their preview props, as JSON.
type EmailRenderer interface {
	Email(ctx context.Context, name string, props []byte) (EmailResult, []byte, error)
}

// ErrNoEmails is returned when the app has no src/emails.
var ErrNoEmails = errors.New("ssr: the app has no email templates (src/emails)")

func decodeEmail(name, out string) (EmailResult, []byte, error) {
	if name == "" {
		return EmailResult{}, []byte(out), nil
	}
	var res EmailResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return EmailResult{}, nil, fmt.Errorf("ssr: decoding the email: %w", err)
	}
	if res.Error != "" {
		return EmailResult{}, nil, errors.New("ssr: " + res.Error)
	}
	return res, nil, nil
}
