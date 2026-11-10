// Package svelte holds what compiled components need from Go beyond the
// JavaScript they were compiled from. Svelte's own server runtime is
// compiled from its source like any other module.
package svelte

import "github.com/Unfield/Tern/native/js"

// Namespaces for imports the IR doesn't record; tern records every module,
// so these are only reached by imports that failed to resolve.
var (
	Internal = js.Value(js.NewNamespace())
	Server   = js.Value(js.NewNamespace())
	Core     = js.Value(js.NewNamespace())
	EsmEnv   = js.Value(js.NewNamespace())
)
