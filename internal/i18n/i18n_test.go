package i18n

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

type testCase struct {
	Locale  string         `json:"locale"`
	Message string         `json:"message"`
	Params  map[string]any `json:"params"`
	Want    string         `json:"want"`
}

// TestSharedCases runs the cases the client's formatter is tested against too.
func TestSharedCases(t *testing.T) {
	raw, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []testCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if got := Format(c.Locale, c.Message, c.Params); got != c.Want {
			t.Errorf("Format(%s, %q, %v) = %q, want %q", c.Locale, c.Message, c.Params, got, c.Want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, msg := range []string{"{", "{name", "a}b", "{n, plural, one {x}}", "{n, number}", "{, plural, other {x}}", "{n, plural, other {x}"} {
		if _, err := parse(msg); err == nil {
			t.Errorf("parse(%q) should fail", msg)
		}
	}
}

func TestCatalog(t *testing.T) {
	fsys := fstest.MapFS{
		"en.json":   {Data: []byte(`{"nav": {"home": "Home", "items": "{n, plural, one {# item} other {# items}}"}, "only": "English only"}`)},
		"de.json":   {Data: []byte(`{"nav": {"home": "Startseite", "items": "{n, plural, one {# Element} other {# Elemente}}"}}`)},
		"notes.txt": {Data: []byte("ignored")},
	}
	c, err := Load(fsys, "")
	if err != nil {
		t.Fatal(err)
	}
	if c.Default != "en" || strings.Join(c.Locales, ",") != "de,en" {
		t.Errorf("catalog = %s %v", c.Default, c.Locales)
	}
	if got := c.T("de", "nav.items", map[string]any{"n": 3}); got != "3 Elemente" {
		t.Errorf("T = %q", got)
	}
	if got := c.T("de", "only", nil); got != "English only" {
		t.Errorf("falls back to the default locale: %q", got)
	}
	if got := c.T("de", "nope", nil); got != "nope" {
		t.Errorf("falls back to the key: %q", got)
	}
	if m := c.Missing(); strings.Join(m["de"], ",") != "only" {
		t.Errorf("missing = %v", m)
	}
	for header, want := range map[string]string{
		"de-AT,de;q=0.9,en;q=0.8": "de",
		"fr-FR,fr;q=0.9":          "",
		"en-US":                   "en",
		"fr;q=1, DE;q=0.5":        "de",
		"":                        "",
	} {
		if got := c.Match(header); got != want {
			t.Errorf("Match(%q) = %q, want %q", header, got, want)
		}
	}
	if _, err := Load(fstest.MapFS{"en.json": {Data: []byte(`{"bad": "{n, plural, one {x}}"}`)}}, ""); err == nil {
		t.Error("invalid messages must fail to load")
	}
	if _, err := Load(fsys, "fr"); err == nil {
		t.Error("an unknown default locale must fail")
	}
}
