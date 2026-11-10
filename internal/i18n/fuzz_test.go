package i18n

import (
	"strings"
	"unicode/utf8"
	"testing"
	"testing/fstest"
)

// FuzzFormat checks that formatting any message never panics, and that text
// without ICU syntax comes back unchanged.
func FuzzFormat(f *testing.F) {
	for _, s := range []string{"Hello {name}", "{n, plural, =0 {none} one {# item} other {# items}}", "{g, select, a {A} other {O}}", "'{'", "it''s", "{", "}", "{n, plural, other {{n, plural, other {#}}}}", "#", "{a,}", "{ , plural, other {x}}"} {
		f.Add(s, "en", 3.0)
	}
	f.Fuzz(func(t *testing.T, msg, locale string, n float64) {
		out := Format(locale, msg, map[string]any{"name": "Ada", "n": n, "g": "a"})
		// Locale files are JSON, so messages are valid UTF-8.
		if utf8.ValidString(msg) && !strings.ContainsAny(msg, "{}'#") && out != msg {
			t.Fatalf("plain text changed: %q -> %q", msg, out)
		}
	})
}

// FuzzFlatten checks that reading any locale file never panics.
func FuzzFlatten(f *testing.F) {
	f.Add([]byte(`{"a":{"b":"c"},"d":"e"}`))
	f.Add([]byte(`{"a":1}`))
	f.Add([]byte(`[]`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		_, _ = Flatten(raw)
	})
}

// FuzzMatch checks that any Accept-Language header picks a supported locale,
// or "" for none (the caller then uses the default locale).
func FuzzMatch(f *testing.F) {
	for _, s := range []string{"de-DE,de;q=0.9,en;q=0.8", "*", "", "fr;q=abc", "en-GB;q=0.5, de;q=1.0", ";;;,,,"} {
		f.Add(s)
	}
	c, err := Load(fstest.MapFS{"en.json": {Data: []byte(`{"a":"A"}`)}, "de.json": {Data: []byte(`{"a":"B"}`)}, "pt-BR.json": {Data: []byte(`{"a":"C"}`)}}, "")
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, header string) {
		got := c.Match(header)
		if got != "" && !c.Has(got) {
			t.Fatalf("Match(%q) = %q, not a supported locale", header, got)
		}
	})
}
