package js

import "fmt"

// Throw is a JavaScript exception in flight: compiled code panics with it,
// and try/catch recovers it.
type Throw struct{ Value Value }

func (t *Throw) Error() string { return "js: uncaught " + ToDisplay(t.Value) }

// Unsupported is raised when compiled code meets something the compiler
// handles only partly; the request falls back to the JavaScript engine.
type Unsupported struct{ What string }

func (u *Unsupported) Error() string { return "js: not supported natively: " + u.What }

// NotSupported panics with an Unsupported.
func NotSupported(format string, args ...any) *Unsupported {
	return &Unsupported{What: fmt.Sprintf(format, args...)}
}

func newError(ctor *Object, msg string) *Throw {
	e := NewObject(ctor.Get("prototype").(*Object))
	e.DefineHidden("message", msg)
	e.DefineHidden("stack", ctor.Get("name").(string)+": "+msg)
	return &Throw{Value: e}
}

// TypeErrorf makes a TypeError exception.
func TypeErrorf(format string, args ...any) *Throw {
	return newError(TypeError, fmt.Sprintf(format, args...))
}

// RangeErrorf makes a RangeError exception.
func RangeErrorf(format string, args ...any) *Throw {
	return newError(RangeError, fmt.Sprintf(format, args...))
}

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
