package i18n

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The browser's copy of the texts must match.
func TestBuiltinMatchesTypeScript(t *testing.T) {
	src, err := os.ReadFile("../../packages/tern/src/runtime/builtin.ts")
	if err != nil {
		t.Fatal(err)
	}
	_, obj, ok := strings.Cut(string(src), "= ")
	if !ok {
		t.Fatal("no object in builtin.ts")
	}
	var ts map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimSpace(obj), ";")), &ts); err != nil {
		t.Fatal(err)
	}
	if len(ts) != len(Builtin) {
		t.Fatalf("builtin.ts has %d texts, builtin.json %d", len(ts), len(Builtin))
	}
	for k, v := range Builtin {
		if ts[k] != v {
			t.Errorf("%s: %q in TypeScript, %q in Go", k, ts[k], v)
		}
	}
}

func TestTB(t *testing.T) {
	var none *Catalog
	if got := none.TB("de", "tern.validate.required", map[string]any{"field": "Name"}); got != "Name is required." {
		t.Fatal(got)
	}
	c := &Catalog{Default: "en", messages: map[string]map[string]string{"de": {"tern.validate.required": "{field} fehlt."}, "en": {}}}
	if got := c.TB("de", "tern.validate.required", map[string]any{"field": "Name"}); got != "Name fehlt." {
		t.Fatal(got)
	}
	if got := c.TB("fr", "tern.validate.required", map[string]any{"field": "Nom"}); got != "Nom is required." {
		t.Fatal(got)
	}
}
