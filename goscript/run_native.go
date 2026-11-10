//go:build !(js && wasm)

package goscript

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Run mounts every component with empty props, outside the browser, and
// prints each one's initial state as JSON. tern uses it at build time so
// server rendering and the first paint show real values.
func Run() {
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := map[string]map[string]json.RawMessage{}
	for _, id := range ids {
		state := map[string]json.RawMessage{}
		c, err := mountComponent(id, []byte("{}"), nil, hooks{emit: func(name string, v []byte) { state[name] = v }}, true)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		c.destroy()
		out[id] = state
	}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
