package js

import "fmt"

// Throw is a JavaScript exception in flight: compiled code panics with it,
// and try/catch recovers it.
//
// The runtime's own errors, such as a TypeError, start with just a kind
// and a message: their error object is made in the realm that catches
// them, by In. Raising them needs no realm, so neither do the value
// operations that can fail.
type Throw struct {
	// Value is what was thrown; nil for a runtime error not yet made.
	Value   Value
	Kind    string // the error constructor's name, for a runtime error
	Message string
}

func (t *Throw) Error() string {
	if t.Value == nil {
		return "js: uncaught " + t.Kind + ": " + t.Message
	}
	return "js: uncaught " + ToDisplay(t.Value)
}

// In returns the thrown value, making a runtime error's object in r.
func (t *Throw) In(r *Realm) Value {
	if t.Value == nil {
		ctor, _ := r.Global.Get(t.Kind).(*Object)
		if ctor == nil || ctor.construct == nil {
			ctor = r.Error
		}
		t.Value = ctor.constructArgs([]Value{t.Message}, ctor)
	}
	return t.Value
}

func errorf(kind, format string, args []any) *Throw {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	return &Throw{Kind: kind, Message: msg}
}

// Unsupported is raised when compiled code meets something the compiler
// handles only partly; the request falls back to the JavaScript engine.
type Unsupported struct{ What string }

func (u *Unsupported) Error() string { return "js: not supported natively: " + u.What }

// NotSupported panics with an Unsupported.
func NotSupported(format string, args ...any) *Unsupported {
	return &Unsupported{What: fmt.Sprintf(format, args...)}
}

// TypeErrorf makes a TypeError exception.
func TypeErrorf(format string, args ...any) *Throw { return errorf("TypeError", format, args) }

// RangeErrorf makes a RangeError exception.
func RangeErrorf(format string, args ...any) *Throw { return errorf("RangeError", format, args) }

// SyntaxErrorf makes a SyntaxError exception.
func SyntaxErrorf(format string, args ...any) *Throw { return errorf("SyntaxError", format, args) }

// ReferenceErrorf makes a ReferenceError exception.
func ReferenceErrorf(format string, args ...any) *Throw {
	return errorf("ReferenceError", format, args)
}

// URIErrorf makes a URIError exception.
func URIErrorf(format string, args ...any) *Throw { return errorf("URIError", format, args) }

// ToDisplay describes a value for error messages, without running any code.
func ToDisplay(v Value) string {
	switch x := v.(type) {
	case string:
		return x
	case *Object:
		if m, ok := x.GetOwn("message"); ok {
			if s, ok := m.(string); ok {
				name := "Error"
				if n, ok := x.lookup("name"); ok {
					if s, ok := n.(string); ok {
						name = s
					}
				}
				return name + ": " + s
			}
		}
		if x.array != nil {
			return "[array]"
		}
		return "[object]"
	case *Symbol:
		return "Symbol(" + x.Description + ")"
	}
	return ToString(v)
}
