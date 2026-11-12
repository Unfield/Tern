// Package plugins lists the plugins tern add knows, from the registry in
// the Tern module.
package plugins

import (
	"github.com/Unfield/Tern/kit"
	registry "github.com/Unfield/Tern/plugins"

	"github.com/Unfield/Tern/website/internal/docs"
)

const Prerender = true

type Plugin struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Import      string   `json:"import"`
	Add         bool     `json:"add"`
	Env         []string `json:"env"`
	Notes       string   `json:"notes"`
	Docs        string   `json:"docs"`
	Author      string   `json:"author"`
	URL         string   `json:"url"`
}

type Data struct {
	Official  []Plugin `json:"official"`
	Community []Plugin `json:"community"`
	// Edit is the registry on GitHub, to add a plugin to it.
	Edit string `json:"edit"`
}

func Load(ctx *kit.Ctx) (Data, error) {
	d := Data{Official: []Plugin{}, Community: []Plugin{}, Edit: docs.RepoURL + "/edit/main/plugins/registry.json"}
	for _, p := range registry.All() {
		item := Plugin{Name: p.Name, Description: p.Description, Import: p.Import, Add: p.Add != "", Env: p.Env, Notes: p.Notes, Docs: p.Docs, Author: p.Author, URL: p.URL}
		if item.Env == nil {
			item.Env = []string{}
		}
		if p.Official() {
			d.Official = append(d.Official, item)
		} else {
			d.Community = append(d.Community, item)
		}
	}
	return d, nil
}
