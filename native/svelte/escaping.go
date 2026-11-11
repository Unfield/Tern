package svelte

import (
	"strings"

	"github.com/Unfield/Tern/native/js"
)

// Intrinsics: Go versions of Svelte's hottest server helpers. The compiler
// uses one in place of the compiled function only when the function's
// source is the one it was written against (internal/native/intrinsics.go
// checks), so they behave exactly the same.

// EscapeHTML is escape_html from svelte/src/escaping.js, for realm rt:
//
//	const str = String(value ?? '');
//	… escape & and <, and " too when is_attr …
func EscapeHTML(rt *js.Realm) js.Value {
	e := &escaper{rt: rt}
	f := js.AsObject(rt.NewFunction("escape_html", 2, func(_ js.Value, args []js.Value) js.Value {
		return e.escape(js.Arg(args, 0), js.Truthy(js.Arg(args, 1)))
	}))
	f.Internal = e
	return f
}

// escaper is escape_html's Go side, which other intrinsics call directly
// while the escape_html they import is this one.
type escaper struct {
	rt           *js.Realm
	stringGlobal js.GlobalRef
}

func (e *escaper) escape(v js.Value, attr bool) js.Value {
	if js.IsNullish(v) {
		v = ""
	}
	if _, ok := v.(string); !ok || e.rt.GlobalGetCached(&e.stringGlobal, "String") != js.Value(e.rt.StringCtor) {
		v = js.Call(e.rt.GlobalGetCached(&e.stringGlobal, "String"), js.Undefined, v)
	}
	s, ok := v.(string)
	if !ok {
		s = js.ToString(v)
	}
	return Escape(v, s, attr)
}

// callEscape is escape_html(v, attr) through the binding f: directly when
// f is the intrinsic.
func callEscape(f js.Value, v js.Value, attr bool) js.Value {
	if o, ok := f.(*js.Object); ok {
		if e, ok := o.Internal.(*escaper); ok {
			return e.escape(v, attr)
		}
	}
	return js.Call(f, js.Undefined, v, attr)
}

// Escape escapes s, whose Value is v: when nothing needs escaping, v comes
// back as it is, without allocating.
func Escape(v js.Value, s string, attr bool) js.Value {
	special := "&<"
	if attr {
		special = "&<\""
	}
	i := strings.IndexAny(s, special)
	if i < 0 {
		return v
	}
	var b strings.Builder
	b.Grow(len(s) + 16)
	for i >= 0 {
		b.WriteString(s[:i])
		switch s[i] {
		case '&':
			b.WriteString("&amp;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteString("&lt;")
		}
		s = s[i+1:]
		i = strings.IndexAny(s, special)
	}
	b.WriteString(s)
	return b.String()
}
