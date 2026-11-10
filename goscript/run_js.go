//go:build js && wasm

package goscript

import (
	"syscall/js"
)

// Run serves the registered components to the browser and blocks. It
// defines globalThis.__tern_go.mount(id, propsJSON, emit), which returns
// an object with call(name, argsJSON) and destroy().
func Run() {
	mountFn := js.FuncOf(func(_ js.Value, args []js.Value) any {
		id, props, emit := args[0].String(), args[1].String(), args[2]
		c, err := mount(id, []byte(props), func(name string, v []byte) { emit.Invoke(name, string(v)) })
		if err != nil {
			return js.Global().Get("Error").New(err.Error())
		}
		call := js.FuncOf(func(_ js.Value, a []js.Value) any {
			out, err := c.call(a[0].String(), []byte(a[1].String()))
			if err != nil {
				js.Global().Get("console").Call("error", err.Error())
				return js.Null()
			}
			return string(out)
		})
		var destroy js.Func
		destroy = js.FuncOf(func(js.Value, []js.Value) any {
			c.destroy()
			call.Release()
			destroy.Release()
			return nil
		})
		obj := js.Global().Get("Object").New()
		obj.Set("call", call)
		obj.Set("destroy", destroy)
		return obj
	})
	js.Global().Set("__tern_go", js.ValueOf(map[string]any{"mount": mountFn}))
	if ready := js.Global().Get("__tern_go_ready"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}
	select {}
}
