package kit

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type query struct {
	Search string     `json:"q"`
	Limit  int        `json:"limit,omitempty"`
	Tags   []string   `query:"tag"`
	Since  *time.Time `json:"since"`
	Done   bool
	Skip   string `json:"-"`
}

func TestDecodeQuery(t *testing.T) {
	var q query
	err := DecodeQuery(url.Values{"q": {"go"}, "limit": {"5"}, "tag": {"a", "b"}, "since": {"2026-11-10T00:00:00Z"}, "Done": {"true"}, "-": {"x"}}, &q)
	if err != nil || q.Search != "go" || q.Limit != 5 || len(q.Tags) != 2 || q.Since == nil || !q.Done || q.Skip != "" {
		t.Errorf("q = %+v, %v", q, err)
	}
	if err := DecodeQuery(url.Values{"limit": {"many"}}, &q); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Errorf("bad number: %v", err)
	}
}

type create struct {
	ID   int64  `path:"id" json:"-"`
	Name string `json:"name"`
}

func (c create) Validate() error {
	if c.Name == "" {
		return errors.New("name is required")
	}
	if c.Name == "forbidden" {
		return Error(http.StatusForbidden, "no")
	}
	return nil
}

func TestTypedEndpoints(t *testing.T) {
	m := testManifest()
	m.Routes = append(m.Routes,
		RouteDef{ID: "/items/id.param", Endpoint: map[string]HandlerFunc{
			"POST": TypedEndpoint("POST", func(c *Ctx, in create) (map[string]any, error) {
				return map[string]any{"id": in.ID, "name": in.Name}, nil
			}),
			"GET": TypedEndpoint("GET", func(c *Ctx, in query) ([]string, error) { return in.Tags, nil }),
		}},
		RouteDef{ID: "/count", Endpoint: map[string]HandlerFunc{"GET": OutputEndpoint(func(c *Ctx) (int, error) { return 42, nil })}},
	)
	a, _ := newTestApp(t, m)
	post := func(body, ct string) (int, string) {
		w := do(t, a, "POST", "/items/7", strings.NewReader(body), "Content-Type", ct)
		return w.Code, strings.TrimSpace(w.Body.String())
	}
	if code, body := post(`{"name":"x"}`, "application/json"); code != 200 || body != `{"id":7,"name":"x"}` {
		t.Errorf("post: %d %s", code, body)
	}
	if code, body := post(`{"name":""}`, "application/json"); code != 400 || !strings.Contains(body, "name is required") {
		t.Errorf("invalid: %d %s", code, body)
	}
	if code, _ := post(`{"name":"forbidden"}`, "application/json"); code != 403 {
		t.Errorf("HTTP errors from Validate pass through: %d", code)
	}
	if code, body := post(`{"name": 3}`, "application/json"); code != 400 || !strings.Contains(body, "name must be string") {
		t.Errorf("wrong type: %d %s", code, body)
	}
	if code, _ := post(`name=x`, "application/x-www-form-urlencoded"); code != http.StatusUnsupportedMediaType {
		t.Errorf("form body: %d", code)
	}
	if w := do(t, a, "GET", "/items/1?tag=a&tag=b", nil); strings.TrimSpace(w.Body.String()) != `["a","b"]` {
		t.Errorf("get: %s", w.Body)
	}
	if w := do(t, a, "GET", "/count", nil); strings.TrimSpace(w.Body.String()) != "42" {
		t.Errorf("output only: %s", w.Body)
	}
}

func TestOpenAPIServing(t *testing.T) {
	m := testManifest()
	m.OpenAPI = []byte(`{"openapi":"3.1.0","info":{"title":"API","version":"1.0.0"},"paths":{}}`)
	a, _ := newTestApp(t, m, WithOpenAPI("/openapi.json", "Shop", "2.1.0"))
	w := do(t, a, "GET", "/openapi.json", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"title":"Shop"`) || !strings.Contains(w.Body.String(), `"version":"2.1.0"`) {
		t.Errorf("openapi: %d %s", w.Code, w.Body)
	}
	b, _ := newTestApp(t, m)
	if w := do(t, b, "GET", "/openapi.json", nil); w.Code != 404 {
		t.Errorf("served without WithOpenAPI: %d", w.Code)
	}
}
