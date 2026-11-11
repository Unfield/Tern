package report

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Unfield/Tern/internal/batch"
	"github.com/Unfield/Tern/logging"
)

type memory struct {
	mu      sync.Mutex
	reports []Report
}

func (m *memory) Send(_ context.Context, r []Report) error {
	m.mu.Lock()
	m.reports = append(m.reports, r...)
	m.mu.Unlock()
	return nil
}

func newReporter(per int) (*Reporter, *memory) {
	m := &memory{}
	return New(Options{Sinks: []Sink{m}, PerMinute: per, Batch: batch.Options{Interval: time.Millisecond}}), m
}

func (m *memory) got(t *testing.T, r *Reporter) []Report {
	t.Helper()
	if err := r.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Report(nil), m.reports...)
}

func TestGroupsAndLimits(t *testing.T) {
	r, m := newReporter(2)
	ctx := logging.With(context.Background(), slog.String(logging.RequestIDKey, "r1"), slog.String("user", "u7"), slog.String("tenant", "acme"))
	for i := range 5 {
		r.Report(ctx, fmt.Errorf("loading order %d: %w", 1000+i, errors.New(`user "ann" not found`)), func(rep *Report) { rep.Route = "/orders/id.param" })
	}
	r.Report(ctx, errors.New("disk full"))
	reports := m.got(t, r)
	if len(reports) != 3 { // 2 of the 5, and the other one
		t.Fatalf("%d sent", len(reports))
	}
	if reports[0].Group != reports[1].Group || reports[0].Group == reports[2].Group {
		t.Error("grouping")
	}
	if reports[0].RequestID != "r1" || reports[0].User != "u7" || reports[0].Attrs["tenant"] != "acme" || reports[0].Type != "errors.errorString" {
		t.Errorf("report %+v", reports[0])
	}
	gs := r.Groups()
	if len(gs) != 2 || gs[0].Message != "disk full" || gs[1].Count != 5 {
		t.Errorf("groups %+v", gs)
	}
}

func TestNormalize(t *testing.T) {
	a := Normalize(`order 123 for "ann" (550e8400-e29b-41d4-a716-446655440000) at 0xc000123`)
	b := Normalize(`order 9 for "bob" (6ba7b810-9dad-11d1-80b4-00c04fd430c8) at 0xc0aa456`)
	if a != b {
		t.Errorf("%q != %q", a, b)
	}
}

func panicky() (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = testPanic(v)
		}
	}()
	var p *struct{ n int }
	p.n++ //nolint:staticcheck // the panic is the point: a nil dereference in this file
	return nil
}

type panicErr struct {
	v   any
	pcs []uintptr
}

func (p *panicErr) Error() string                { return fmt.Sprint(p.v) }
func (p *panicErr) PanicValue() (any, []uintptr) { return p.v, p.pcs }

func testPanic(v any) error {
	pcs := make([]uintptr, 32)
	n := runtimeCallers(4, pcs) // past runtimeCallers, testPanic and the deferred func
	return &panicErr{v: v, pcs: pcs[:n]}
}

func TestPanicsAndStacks(t *testing.T) {
	r, m := newReporter(10)
	r.Report(context.Background(), panicky())
	r.Report(context.Background(), WithStack(errors.New("wrapped")))
	r.Report(context.Background(), Errorf("made %d", 1))
	reports := m.got(t, r)
	p := reports[0]
	if !p.Panic || p.Type != "panic" || !strings.Contains(p.Message, "nil pointer") || len(p.Stack) == 0 {
		t.Fatalf("panic %+v", p)
	}
	if !strings.HasSuffix(p.Stack[0].Function, "report.panicky") {
		t.Errorf("innermost frame %+v", p.Stack[0])
	}
	if len(reports[1].Stack) == 0 || !strings.Contains(reports[1].Stack[0].Function, "TestPanicsAndStacks") || len(reports[2].Stack) == 0 {
		t.Errorf("WithStack %+v / Errorf %+v", reports[1].Stack, reports[2].Stack)
	}
	if WithStack(nil) != nil {
		t.Error("WithStack(nil)")
	}
}

func TestErrType(t *testing.T) {
	type myErr struct{ error }
	if got := errType(fmt.Errorf("x: %w", &myErr{errors.New("y")})); got != "report.myErr" {
		t.Error(got)
	}
}

func runtimeCallers(skip int, pcs []uintptr) int { return runtime.Callers(skip, pcs) }
