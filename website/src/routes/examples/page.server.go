// Package examples lists the example apps in the repository.
package examples

import (
	"github.com/Unfield/Tern/kit"

	"github.com/Unfield/Tern/website/internal/docs"
)

const Prerender = true

type Example struct {
	Name  string   `json:"name"`
	Dir   string   `json:"dir"`
	About string   `json:"about"`
	Uses  []string `json:"uses"`
	URL   string   `json:"url"`
}

type Examples struct {
	Examples []Example `json:"examples"`
}

var list = []Example{
	{Name: "Status", Dir: "status", About: "A self-hosted status page and uptime monitor: HTTP, TCP, DNS and certificate checks, a live dashboard over server-sent events, public status pages and incidents. One binary with SQLite.", Uses: []string{"Server-sent events", "Background jobs", "Migrations", "Tern UI charts"}},
	{Name: "Analytics", Dir: "analytics", About: "Privacy-friendly web analytics: a tracking script without cookies, rollups as jobs, retention, a live dashboard, weekly email reports and a stats API with OpenAPI.", Uses: []string{"Typed endpoints", "OpenAPI", "Email", "Jobs and cron"}},
	{Name: "Accounts", Dir: "accounts", About: "Every screen of user accounts: sign-up, sign-in with a password or a passkey, email verification, password resets and the devices a user is signed in on.", Uses: []string{"accounts", "Passkeys", "Tern UI auth screens"}},
	{Name: "Dashboard", Dir: "dashboard", About: "An admin dashboard built from shadcn-svelte components, rendered on the server: the benchmark app for server rendering.", Uses: []string{"shadcn-svelte", "Server rendering", "Layouts"}},
	{Name: "Tern UI", Dir: "ui", About: "A gallery of Tern UI: every component in light and dark mode, and a form validated by a Go action, with keyboard and accessibility tests.", Uses: []string{"Tern UI", "Form validation", "axe audits"}},
	{Name: "Todos", Dir: "todos", About: "The smallest complete app: sessions, form actions with progressive enhancement, a JSON API, a Markdown blog and prerendered pages.", Uses: []string{"Form actions", "Sessions", "Content collections"}},
	{Name: "Go script", Dir: "goscript", About: "Tern’s experimental <script lang=\"go\">: Svelte components whose logic is Go, compiled to WebAssembly, with runes.", Uses: []string{"Experimental", "WebAssembly"}},
}

func Load(ctx *kit.Ctx) (Examples, error) {
	out := make([]Example, len(list))
	for i, e := range list {
		e.URL = docs.RepoURL + "/tree/main/examples/" + e.Dir
		out[i] = e
	}
	return Examples{Examples: out}, nil
}
