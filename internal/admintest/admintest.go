// Package admintest serves admin panels in tests, as the route an app
// gives them, and reads their screens and actions.
package admintest

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Unfield/Tern/admin"
	"github.com/Unfield/Tern/kit"
)

// App serves panel at /admin, with these other plugins first.
func App(t *testing.T, panel *admin.Admin, plugins ...kit.Plugin) http.Handler {
	t.Helper()
	a, err := kit.New(&kit.Manifest{AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`, Routes: []kit.RouteDef{
		{ID: "/admin/path.rest", Page: &kit.PageDef{Component: "src/routes/admin/+page.svelte", Load: kit.LoadOf(admin.Load), Actions: admin.Actions}},
	}}, kit.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))), kit.WithPlugins(append(plugins, panel)...))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Start(); err != nil {
		t.Fatal(err)
	}
	return a
}

// Page is a screen's data.
type Page struct {
	Status   int
	View     string `json:"view"`
	Resource struct {
		Actions []admin.ActionInfo `json:"actions"`
		Edit    bool               `json:"edit"`
	} `json:"resource"`
	List struct {
		Rows []struct {
			Key    string            `json:"key"`
			Values map[string]string `json:"values"`
		} `json:"rows"`
		Total int `json:"total"`
	} `json:"list"`
	Record struct {
		Values map[string]string `json:"values"`
	} `json:"record"`
	Notice string `json:"notice"`
}

// Get loads a screen.
func Get(t *testing.T, h http.Handler, path string) Page {
	t.Helper()
	u, _ := url.Parse(path)
	u.Path = strings.TrimSuffix(u.Path, "/") + "/__data.json"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, u.String(), nil))
	var r struct {
		Type   string            `json:"type"`
		Status int               `json:"status"`
		Nodes  []json.RawMessage `json:"nodes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("%s: %v: %s", path, err, rec.Body)
	}
	var p Page
	if len(r.Nodes) > 0 {
		_ = json.Unmarshal(r.Nodes[len(r.Nodes)-1], &p)
	}
	p.Status = r.Status
	if r.Type == "data" && p.Status == 0 {
		p.Status = 200
	}
	return p
}

// Post runs an action and returns where it redirects, or the failure's message.
func Post(t *testing.T, h http.Handler, path string, form url.Values) (location, message string, status int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Tern-Action", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var r struct {
		Type     string `json:"type"`
		Status   int    `json:"status"`
		Location string `json:"location"`
		Data     struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("%s: %v: %s", path, err, rec.Body)
	}
	return r.Location, r.Data.Message, r.Status
}
