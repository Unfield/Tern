package svelte

import (
	"strings"

	"github.com/Unfield/Tern/native/js"
)

// Go versions of Svelte's attribute helpers, which a component calls for
// each element. Each takes a fast path only when it computes what the
// JavaScript computes without running other code, and calls the compiled
// function (orig) otherwise: for objects whose conversion to a string
// runs code, getters, non-ASCII class names, or a module whose constants
// aren't the ones these were written against.

// Attr is attr from svelte/src/internal/shared/attributes.js:
//
//	export function attr(name, value, is_boolean = false) {
//		if (name === 'hidden' && value !== 'until-found') {
//			is_boolean = true;
//		}
//		if (value == null || (is_boolean && !value && value !== '')) return '';
//		const normalized =
//			(has_own_property.call(replacements, name) && replacements[name].get(value)) || value;
//		const assignment = is_boolean ? `=""` : `="${escape_html(normalized, true)}"`;
//		return ` ${name}${assignment}`;
//	}
//
// replacements, hasOwn and escape read the module's bindings.
func Attr(rt *js.Realm, replacements, hasOwn, escape func() js.Value, orig js.Value) js.Value {
	origHasOwn := rt.ObjectPrototype.Get("hasOwnProperty")
	origCall := rt.FunctionPrototype.Get("call")
	var callIC, getIC js.IC
	return rt.NewFunction("attr", 3, func(this js.Value, args []js.Value) js.Value {
		name, ok := js.Arg(args, 0).(string)
		if !ok {
			return js.Call(orig, this, args...)
		}
		value := js.Arg(args, 1)
		isBool := js.Arg(args, 2)
		if isBool == js.Undefined {
			isBool = false
		}
		if name == "hidden" && !js.StrictEquals(value, "until-found") {
			isBool = true
		}
		if js.IsNullish(value) || (js.Truthy(isBool) && !js.Truthy(value) && !js.StrictEquals(value, "")) {
			return ""
		}
		// has_own_property.call(replacements, name), while both are the
		// built-ins.
		hop := hasOwn()
		repl, isObj := replacements().(*js.Object)
		if hop != origHasOwn || !isObj || rt.GetIC(&callIC, hop, "call") != origCall {
			return js.Call(orig, this, args...)
		}
		has, plain := repl.HasOwnString(name)
		if !plain {
			return js.Call(orig, this, args...)
		}
		normalized := value
		if has {
			if got := rt.CallIC(&getIC, rt.Get(repl, name), "get", value); js.Truthy(got) {
				normalized = got
			}
		}
		if js.Truthy(isBool) {
			return rt.Str(" " + name + `=""`)
		}
		return rt.Str(" " + name + `="` + js.ToString(callEscape(escape(), normalized, true)) + `"`)
	})
}

// whitespace is attributes.js's whitespace: [...' \t\n\r\f\u00a0\u000b\ufeff'].
var whitespace = []string{" ", "\t", "\n", "\r", "\f", "\u00a0", "\u000b", "\ufeff"}

// classes is to_class's Go side, which AttrClass calls directly.
type classes struct {
	rt         *js.Realm
	whitespace func() js.Value
	checked    js.Value // the whitespace array last found to be the expected one
}

// wsOK reports whether the module's whitespace array is the expected one.
func (c *classes) wsOK() bool {
	w := c.whitespace()
	if w == c.checked && w != nil {
		return true
	}
	o, ok := w.(*js.Object)
	if !ok || !js.IsArray(o) {
		return false
	}
	vals, ok := o.ArrayElements()
	if !ok || len(vals) != len(whitespace) {
		return false
	}
	for i, v := range vals {
		if s, ok := v.(string); !ok || s != whitespace[i] {
			return false
		}
	}
	c.checked = w
	return true
}

func isClassSpace(b byte) bool {
	// The ASCII ones of whitespace: the class names here are ASCII.
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == '\v'
}

func ascii(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// primitiveString is the empty string plus v, for a primitive v; false for objects,
// whose conversion may run code.
func primitiveString(v js.Value) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case *js.Object, *js.Symbol:
		return "", false // objects run code; symbols throw: the JavaScript does both
	}
	return js.ToString(v), true
}

// toClass computes to_class(value, hash, directives); ok is false when it
// leaves it to the JavaScript, having done nothing observable.
func (c *classes) toClass(value, hash, directives js.Value) (js.Value, bool) {
	if !c.wsOK() {
		return nil, false
	}
	classname := ""
	if !js.IsNullish(value) {
		s, ok := primitiveString(value)
		if !ok || !ascii(s) {
			return nil, false
		}
		classname = s
	}
	if js.Truthy(hash) {
		h, ok := hash.(string)
		if !ok || !ascii(h) {
			return nil, false
		}
		if classname != "" {
			classname = classname + " " + h
		} else {
			classname = h
		}
	}
	if js.Truthy(directives) {
		d, ok := directives.(*js.Object)
		if !ok {
			return nil, false
		}
		valid := true
		if !d.PlainEach(func(k string, _ js.Value) {
			if k == "" || !ascii(k) {
				valid = false // "" would loop forever in the JavaScript too: let it, where it can be stopped
			}
		}) || !valid {
			return nil, false
		}
		d.PlainEach(func(key string, v js.Value) {
			if js.Truthy(v) {
				if classname != "" {
					classname = classname + " " + key
				} else {
					classname = key
				}
				return
			}
			if classname == "" {
				return
			}
			// Remove each whole-word occurrence of key.
			a := 0
			for {
				j := strings.Index(classname[a:], key)
				if j < 0 {
					break
				}
				a += j
				b := a + len(key)
				if (a == 0 || isClassSpace(classname[a-1])) && (b == len(classname) || isClassSpace(classname[b])) {
					rest := ""
					if b+1 < len(classname) {
						rest = classname[b+1:]
					}
					classname = classname[:a] + rest
				} else {
					a = b
				}
			}
		})
	}
	if classname == "" {
		return js.Null, true
	}
	return classname, true
}

// ToClass is to_class from svelte/src/internal/shared/attributes.js.
func ToClass(rt *js.Realm, whitespace func() js.Value, orig js.Value) js.Value {
	c := &classes{rt: rt, whitespace: whitespace}
	f := js.AsObject(rt.NewFunction("to_class", 3, func(this js.Value, args []js.Value) js.Value {
		if v, ok := c.toClass(js.Arg(args, 0), js.Arg(args, 1), js.Arg(args, 2)); ok {
			return v
		}
		return js.Call(orig, this, args...)
	}))
	f.Internal = c
	return f
}

// AttrClass is attr_class from svelte/src/internal/server/index.js:
//
//	export function attr_class(value, hash, directives) {
//		var result = to_class(value, hash, directives);
//		return result ? ` class="${escape_html(result, true)}"` : '';
//	}
func AttrClass(rt *js.Realm, toClass, escape func() js.Value, orig js.Value) js.Value {
	return rt.NewFunction("attr_class", 3, func(this js.Value, args []js.Value) js.Value {
		tc := toClass()
		var result js.Value
		if o, ok := tc.(*js.Object); ok {
			if c, ok := o.Internal.(*classes); ok {
				if v, ok := c.toClass(js.Arg(args, 0), js.Arg(args, 1), js.Arg(args, 2)); ok {
					result = v
				}
			}
		}
		if result == nil {
			result = js.Call(tc, js.Undefined, js.Arg(args, 0), js.Arg(args, 1), js.Arg(args, 2))
		}
		if !js.Truthy(result) {
			return ""
		}
		return ` class="` + js.ToString(callEscape(escape(), result, true)) + `"`
	})
}

// Stringify is stringify from svelte/src/internal/server/index.js:
//
//	export function stringify(value) {
//		return typeof value === 'string' ? value : value == null ? '' : value + '';
//	}
func Stringify(rt *js.Realm) js.Value {
	return rt.NewFunction("stringify", 1, func(_ js.Value, args []js.Value) js.Value {
		v := js.Arg(args, 0)
		if s, ok := v.(string); ok {
			return s
		}
		if js.IsNullish(v) {
			return ""
		}
		return rt.Add(v, "")
	})
}

// EnsureArrayLike is ensure_array_like from svelte/src/internal/server/index.js:
//
//	export function ensure_array_like(array_like_or_iterator) {
//		if (array_like_or_iterator) {
//			return array_like_or_iterator.length !== undefined
//				? array_like_or_iterator
//				: Array.from(array_like_or_iterator);
//		}
//		return [];
//	}
//
// Arrays and strings, whose length is there without running code, come
// back as they are; the rest is left to the JavaScript.
func EnsureArrayLike(rt *js.Realm, orig js.Value) js.Value {
	return rt.NewFunction("ensure_array_like", 1, func(this js.Value, args []js.Value) js.Value {
		v := js.Arg(args, 0)
		if !js.Truthy(v) {
			return rt.NewArray(nil)
		}
		switch x := v.(type) {
		case string:
			return x
		case *js.Object:
			if js.IsPlainArray(x) {
				return x
			}
		}
		return js.Call(orig, this, args...)
	})
}
