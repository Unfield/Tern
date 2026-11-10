package kit

import (
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
)

func i18nManifest() *Manifest {
	m := testManifest()
	m.AppHTML = `<html lang="%tern.lang%"><head>%tern.head%</head><body>%tern.body%</body></html>`
	m.Locales = fstest.MapFS{
		"en.json": {Data: []byte(`{"hello": "Hello, {name}!", "count": "{n, plural, one {# todo} other {# todos}}"}`)},
		"de.json": {Data: []byte(`{"hello": "Hallo, {name}!", "count": "{n, plural, one {# Aufgabe} other {# Aufgaben}}"}`)},
	}
	m.Routes = append(m.Routes,
		RouteDef{ID: "/greet", Page: &PageDef{Load: LoadOf(func(c *Ctx) (map[string]string, error) {
			return map[string]string{"text": c.T("hello", map[string]any{"name": "Ada"}), "count": c.T("count", map[string]any{"n": 2}), "locale": c.Locale()}, nil
		})}},
		RouteDef{ID: "/lang.param/greet", Page: &PageDef{Load: LoadOf(func(c *Ctx) (map[string]string, error) {
			return map[string]string{"text": c.T("hello", map[string]any{"name": "Ada"})}, nil
		})}},
	)
	return m
}

func TestLocaleResolution(t *testing.T) {
	a, _ := newTestApp(t, i18nManifest())
	cases := []struct {
		name, path, header, value, want string
	}{
		{"default", "/greet", "", "", `"text":"Hello, Ada!"`},
		{"accept-language", "/greet", "Accept-Language", "de-DE,de;q=0.9", `"text":"Hallo, Ada!"`},
		{"cookie beats header", "/greet", "Cookie", "locale=en", `"text":"Hello, Ada!"`},
		{"route parameter", "/de/greet", "", "", `"text":"Hallo, Ada!"`},
		{"unknown cookie ignored", "/greet", "Cookie", "locale=xx", `"text":"Hello, Ada!"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			headers := []string{}
			if tc.header != "" {
				headers = append(headers, tc.header, tc.value)
			}
			if tc.name == "cookie beats header" {
				headers = append(headers, "Accept-Language", "de")
			}
			body := do(t, a, "GET", tc.path, nil, headers...).Body.String()
			if !strings.Contains(body, tc.want) {
				t.Errorf("body missing %s:\n%s", tc.want, body)
			}
		})
	}

	body := do(t, a, "GET", "/greet", nil, "Accept-Language", "de").Body.String()
	for _, want := range []string{`<html lang="de">`, `"count":"2 Aufgaben"`, `"locale":"de"`} {
		if !strings.Contains(body, want) {
			t.Errorf("German page missing %s", want)
		}
	}
	if p := jsonPayload(t, do(t, a, "GET", "/greet/__data.json", nil, "Accept-Language", "de")); p.Locale != "de" {
		t.Errorf("data payload locale = %q", p.Locale)
	}
}

func TestSetLocaleFromHook(t *testing.T) {
	a, _ := newTestApp(t, i18nManifest(), WithHook(func(c *Ctx) error {
		if c.Header("X-User-Lang") != "" {
			c.SetLocale(c.Header("X-User-Lang"))
		}
		return nil
	}))
	if body := do(t, a, "GET", "/greet", nil, "X-User-Lang", "de").Body.String(); !strings.Contains(body, "Hallo") {
		t.Error("SetLocale in a hook should win")
	}
	if body := do(t, a, "GET", "/greet", nil, "X-User-Lang", "fr").Body.String(); !strings.Contains(body, "Hello") {
		t.Error("unsupported locales are ignored")
	}
}

func TestNoI18n(t *testing.T) {
	m := testManifest()
	m.Routes = append(m.Routes, RouteDef{ID: "/t", Endpoint: map[string]HandlerFunc{"GET": func(c *Ctx) error {
		return c.Text(http.StatusOK, c.T("some.key")+"|"+c.Locale())
	}}})
	a, _ := newTestApp(t, m)
	if body := do(t, a, "GET", "/t", nil).Body.String(); body != "some.key|" {
		t.Errorf("without locales T returns the key: %q", body)
	}
}

func TestTranslatedPaths(t *testing.T) {
	m := testManifest()
	m.Locales = fstest.MapFS{
		"en.json": {Data: []byte(`{"title":"Projects"}`)},
		"de.json": {Data: []byte(`{"title":"Projekte","$routes":{"projects":"projekte"}}`)},
	}
	m.Routes = append(m.Routes, RouteDef{ID: "/lang.optional/projects", Endpoint: map[string]HandlerFunc{"GET": func(c *Ctx) error {
		return c.Text(200, c.Locale()+" "+c.T("title"))
	}}})
	a, _ := newTestApp(t, m)
	for path, want := range map[string]string{"/de/projekte": "de Projekte", "/projects": "en Projects", "/de/projects": "de Projekte"} {
		if w := do(t, a, "GET", path, nil); w.Code != 200 || w.Body.String() != want {
			t.Errorf("%s: %d %q, want %q", path, w.Code, w.Body, want)
		}
	}
}
