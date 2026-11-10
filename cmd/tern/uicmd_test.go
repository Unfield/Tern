package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUIAdd(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	dist := filepath.Join(root, "node_modules/@ternkit/tern/dist/ui")
	files := map[string]string{
		"index.js":        "export { default as Button } from './Button.svelte';\nexport { default as Form } from './Form.svelte';\n",
		"Button.svelte":   "<script lang=\"ts\">import { getField } from './field.js';</script>",
		"Form.svelte":     "<script lang=\"ts\">import { enhance } from '../runtime/forms.js';\nimport Button from './Button.svelte';\nimport { page } from '../runtime/state.svelte.js';</script>",
		"field.js":        "export const getField = () => {};",
		"field.d.ts":      "import type { FormField } from '../runtime/validate.js';\nexport declare const getField: () => void;",
		"MenuList.svelte": "<!-- internal -->",
	}
	if err := os.MkdirAll(dist, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dist, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var out, errOut bytes.Buffer
	if code := run([]string{"ui", "list"}, &out, &errOut); code != 0 || out.String() != "Button\nForm\n" {
		t.Fatalf("list: %d %q %s", code, out.String(), errOut.String())
	}
	out.Reset()
	if code := run([]string{"ui", "add", "form"}, &out, &errOut); code != 0 {
		t.Fatalf("add: %d %s", code, errOut.String())
	}
	for _, f := range []string{"Form.svelte", "Button.svelte", "field.js", "field.d.ts"} {
		if _, err := os.Stat(filepath.Join(root, "src/lib/ui", f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	form, _ := os.ReadFile(filepath.Join(root, "src/lib/ui/Form.svelte"))
	if strings.Contains(string(form), "../runtime") || strings.Count(string(form), "from '@ternkit/tern'") != 2 {
		t.Errorf("runtime imports not rewritten:\n%s", form)
	}
	types, _ := os.ReadFile(filepath.Join(root, "src/lib/ui/field.d.ts"))
	if !strings.Contains(string(types), "from '@ternkit/tern'") {
		t.Errorf("type imports not rewritten:\n%s", types)
	}
	if !strings.Contains(out.String(), "import Form from '$lib/ui/Form.svelte';") {
		t.Errorf("no import hint:\n%s", out.String())
	}

	// Changed files are kept unless -force.
	if err := os.WriteFile(filepath.Join(root, "src/lib/ui/Button.svelte"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	run([]string{"ui", "add", "Button"}, &out, &errOut)
	if b, _ := os.ReadFile(filepath.Join(root, "src/lib/ui/Button.svelte")); string(b) != "mine" || !strings.Contains(out.String(), "already there") {
		t.Errorf("overwrote a changed file: %q %s", b, out.String())
	}
	run([]string{"ui", "add", "-force", "Button"}, &out, &errOut)
	if b, _ := os.ReadFile(filepath.Join(root, "src/lib/ui/Button.svelte")); string(b) == "mine" {
		t.Error("-force didn't replace it")
	}
	errOut.Reset()
	if code := run([]string{"ui", "add", "MenuList"}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "no Tern UI component") {
		t.Errorf("internal components can't be added: %d %s", code, errOut.String())
	}
}
