// Package plugins lists the plugins tern add knows and tern.land shows:
// Tern's own, under plugins/, and the community's. To list a plugin of
// yours, add it to registry.json in a pull request.
package plugins

import (
	_ "embed"
	"encoding/json"
	"strings"
)

// Plugin is an entry of the registry.
type Plugin struct {
	// Name is what tern add takes, such as "compress".
	Name        string `json:"name"`
	Description string `json:"description"`
	// Import is the plugin's package.
	Import string `json:"import"`
	// Module is the Go module to go get, when it isn't Tern's own.
	Module string `json:"module,omitempty"`
	// Add is the expression that makes the plugin, which tern add puts in
	// the app's kit.WithPlugins; "" when it needs configuration first.
	Add string `json:"add,omitempty"`
	// Imports are other packages Add uses, such as "os".
	Imports []string `json:"imports,omitempty"`
	// Env are the environment variables it reads.
	Env []string `json:"env,omitempty"`
	// Notes say what to do after adding it.
	Notes  string `json:"notes,omitempty"`
	Docs   string `json:"docs"`
	Author string `json:"author"`
	// URL is the plugin's home, for community plugins.
	URL string `json:"url,omitempty"`
}

// Official reports whether the plugin is part of Tern.
func (p Plugin) Official() bool {
	return p.Module == "" && strings.HasPrefix(p.Import, "github.com/Unfield/Tern/")
}

//go:embed registry.json
var registry []byte

// All returns the registry, in its order.
func All() []Plugin {
	var out []Plugin
	if err := json.Unmarshal(registry, &out); err != nil {
		panic("plugins: registry.json: " + err.Error())
	}
	return out
}

// Find returns the plugin named name, or with the import path name.
func Find(name string) (Plugin, bool) {
	for _, p := range All() {
		if p.Name == name || p.Import == name {
			return p, true
		}
	}
	return Plugin{}, false
}
