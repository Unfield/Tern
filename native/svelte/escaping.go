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
	var stringGlobal js.GlobalRef
	return rt.NewFunction("escape_html", 2, func(_ js.Value, args []js.Value) js.Value {
		v := js.Arg(args, 0)
		if js.IsNullish(v) {
			v = ""
		}
		if _, ok := v.(string); !ok || rt.GlobalGetCached(&stringGlobal, "String") != js.Value(rt.StringCtor) {
			v = js.Call(rt.GlobalGetCached(&stringGlobal, "String"), js.Undefined, v)
		}
		s, ok := v.(string)
		if !ok {
			s = js.ToString(v)
		}
		return Escape(v, s, js.Truthy(js.Arg(args, 1)))
	})
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
