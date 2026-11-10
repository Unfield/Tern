package native

import (
	"fmt"
	"strings"
)

type writer struct {
	b      strings.Builder
	indent int
}

func (w *writer) line(format string, args ...any) {
	w.b.WriteString(strings.Repeat("\t", w.indent))
	if len(args) == 0 {
		w.b.WriteString(format)
	} else {
		fmt.Fprintf(&w.b, format, args...)
	}
	w.b.WriteByte('\n')
}

func (w *writer) open(format string, args ...any) {
	w.line(format, args...)
	w.indent++
}

func (w *writer) close(suffix string) {
	w.indent--
	w.line("%s", "}"+suffix)
}

func (w *writer) String() string { return w.b.String() }

// sub makes a writer at the same indent, to compile a part before deciding
// how to introduce it.
func (w *writer) sub() *writer { return &writer{indent: w.indent} }

func (w *writer) raw(s string) { w.b.WriteString(s) }

// target is a loop, switch or labeled statement that break or continue
// can reach.
type target struct {
	label    string // the Go label
	jsLabels []string
	loop     bool // continue works
	used     bool
	tryDepth int // the function's try depth where the target is
	id       int
}

// fnCtx is what a function body compiles in.
type fnCtx struct {
	parent   *fnCtx
	this     string // Go expression of this
	args     string // Go name of the arguments slice
	arrow    bool
	module   bool
	tryDepth int
	targets  []*target
	// usesArguments is set when the body reads arguments.
	usesArguments bool
	argumentsVar  string
	// For class constructors and methods: what super means.
	superProto string // the parent prototype, for super.x
	superCtor  string // the parent constructor, for super(...)
	thisVar    string // in a derived constructor, this is set by super()
	afterSuper string // field initializers to run after super()
	generator  string // the Go name of the generator's *js.Gen, in a generator
}

func (f *fnCtx) push(t *target) {
	t.tryDepth = f.tryDepth
	f.targets = append(f.targets, t)
}

func (f *fnCtx) pop() { f.targets = f.targets[:len(f.targets)-1] }
