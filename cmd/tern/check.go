package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/Unfield/Tern/internal/codegen"
	"github.com/Unfield/Tern/internal/ui"
)

// checkDir is where tern check writes the copy of src it type-checks.
const checkDir = codegen.GenDir + "/check"

// cmdCheck regenerates code and type-checks the app with svelte-check.
// Components with <script lang="go"> are checked against their Go code's
// types: svelte-check sees a copy of src in which each Go script is
// replaced by TypeScript declarations of what the markup may use.
func cmdCheck(args []string, stdout, stderr io.Writer) int {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Fprintln(stdout, "usage: tern check [svelte-check options]\n\nGenerates code, then runs svelte-check, with the markup of <script lang=\"go\"> components checked against their Go types.")
			return 0
		}
	}
	root, _ := os.Getwd()
	if _, err := codegen.Generate(root); err != nil {
		ui.New(stderr).Error(err)
		return 1
	}
	bin, err := svelteCheckBin(root)
	if err != nil {
		ui.New(stderr).Error(err)
		return 1
	}
	shadow, err := buildCheckTree(root)
	if err != nil {
		ui.New(stderr).Error(err)
		return 1
	}
	argv := append([]string{}, args...)
	if shadow {
		shadowDir := filepath.Join(root, filepath.FromSlash(checkDir))
		argv = append([]string{"--workspace", shadowDir, "--tsconfig", filepath.Join(shadowDir, "tsconfig.json")}, argv...)
	} else {
		argv = append([]string{"--tsconfig", "./tsconfig.json"}, argv...)
	}
	cmd := exec.Command(bin, argv...)
	cmd.Dir = root
	cmd.Stdout = &pathFixer{w: stdout}
	cmd.Stderr = &pathFixer{w: stderr}
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		ui.New(stderr).Error(err)
		return 1
	}
	return 0
}

// pathFixer makes paths in svelte-check's output point at the real files.
type pathFixer struct{ w io.Writer }

func (p *pathFixer) Write(b []byte) (int, error) {
	s := strings.ReplaceAll(string(b), "/"+checkDir+"/", "/")
	s = strings.ReplaceAll(s, checkDir+"/", "")
	s = strings.ReplaceAll(s, filepath.FromSlash(checkDir)+string(filepath.Separator), "")
	if _, err := io.WriteString(p.w, s); err != nil {
		return 0, err
	}
	return len(b), nil
}

func svelteCheckBin(root string) (string, error) {
	name := "svelte-check"
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	for dir := root; ; dir = filepath.Dir(dir) {
		p := filepath.Join(dir, "node_modules", ".bin", name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return "", errors.New("svelte-check is not installed; add it with npm install -D svelte-check")
}

var (
	goScriptTag = regexp.MustCompile(`<script\b([^>]*\blang=["']go["'][^>]*)>([\s\S]*?)</script>`)
	liveAttr    = regexp.MustCompile(`(^|\s)live(\s|=|$)`)
	stateNameRe = regexp.MustCompile(`\b(?:State|Derived)(?:\[[^\]]*\])?\(\s*\w+\s*,\s*"([A-Za-z_$][\w$]*)"`)
	showNameRe  = regexp.MustCompile(`\.Show\(\s*\w+\s*,\s*"([A-Za-z_$][\w$]*)"`)
	// A room's Show takes the room's key first: room.Show(c, "board:"+id, "people").
	showKeyNameRe = regexp.MustCompile(`\.Show\(\s*\w+\s*,\s*(?:[^,()"]|"[^"]*"|\([^()]*\))+,\s*"([A-Za-z_$][\w$]*)"`)
	funcNameRe    = regexp.MustCompile(`\.Func\(\s*"([A-Za-z_$][\w$]*)"`)
	tsconfigPath  = regexp.MustCompile(`^\.\./`)
)

// goScriptNames matches exportedNames in packages/tern/src/vite/goscript.ts.
func goScriptNames(src string) (values, funcs []string) {
	seen := map[string]bool{}
	for _, re := range []*regexp.Regexp{stateNameRe, showNameRe, showKeyNameRe} {
		for _, m := range re.FindAllStringSubmatch(src, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				values = append(values, m[1])
			}
		}
	}
	for _, m := range funcNameRe.FindAllStringSubmatch(src, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			funcs = append(funcs, m[1])
		}
	}
	return values, funcs
}

// buildCheckTree writes the copy of src for svelte-check and reports
// whether one was needed.
func buildCheckTree(root string) (bool, error) {
	out := filepath.Join(root, filepath.FromSlash(checkDir))
	if err := os.RemoveAll(out); err != nil {
		return false, err
	}
	src := filepath.Join(root, "src")
	needed := false
	type file struct{ rel, text string }
	var rewritten []file
	var others []string
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if strings.HasSuffix(p, ".svelte") {
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if goScriptTag.Match(raw) {
				needed = true
				rewritten = append(rewritten, file{rel, typedScript(root, rel, string(raw))})
				return nil
			}
		}
		others = append(others, rel)
		return nil
	})
	if err != nil || !needed {
		return false, err
	}
	for _, f := range rewritten {
		dst := filepath.Join(out, f.rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return false, err
		}
		if err := os.WriteFile(dst, []byte(f.text), 0o644); err != nil {
			return false, err
		}
	}
	for _, rel := range others {
		dst := filepath.Join(out, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return false, err
		}
		if os.Link(filepath.Join(root, rel), dst) != nil {
			raw, err := os.ReadFile(filepath.Join(root, rel))
			if err != nil {
				return false, err
			}
			if err := os.WriteFile(dst, raw, 0o644); err != nil {
				return false, err
			}
		}
	}
	return true, writeCheckTSConfig(root, out)
}

// typedScript replaces a component's Go script with TypeScript that
// declares what its markup may use, keeping every line where it was.
func typedScript(root, rel, text string) string {
	m := goScriptTag.FindStringSubmatchIndex(text)
	attrs, code := text[m[2]:m[3]], text[m[4]:m[5]]
	live := liveAttr.MatchString(attrs)
	values, funcs := goScriptNames(code)
	id := codegen.GoScriptID(rel)

	var b strings.Builder
	b.WriteString(`let __goProps: Record<string, any> = $props();`)
	taken := map[string]bool{}
	for _, n := range append(append([]string{}, values...), funcs...) {
		taken[n] = true
	}
	if live {
		typesFile := filepath.Join(root, filepath.FromSlash(codegen.LiveTypesDir), id)
		from, _ := filepath.Rel(filepath.Dir(filepath.Join(root, filepath.FromSlash(checkDir), rel)), typesFile)
		fmt.Fprintf(&b, ` import type { State as __S, Funcs as __F } from %q; import type { LiveComponent as __L } from '@ternkit/tern/goscript';`, "./"+filepath.ToSlash(from))
		if !taken["live"] {
			b.WriteString(` const live = null as unknown as __L<__S>;`)
		}
		for _, v := range values {
			fmt.Fprintf(&b, ` const %s = null as unknown as __S[%q];`, v, v)
		}
		for _, f := range funcs {
			fmt.Fprintf(&b, ` const %s = null as unknown as __F[%q];`, f, f)
		}
	} else {
		for _, n := range append(append([]string{}, values...), funcs...) {
			fmt.Fprintf(&b, ` const %s: any = null;`, n)
		}
	}
	b.WriteString(strings.Repeat("\n", strings.Count(code, "\n")))
	tag := strings.TrimSpace(liveAttr.ReplaceAllString(regexp.MustCompile(`\blang=["']go["']`).ReplaceAllString(attrs, `lang="ts"`), "$1$2"))
	return text[:m[0]] + "<script " + tag + ">" + b.String() + "</script>" + text[m[1]:]
}

// writeCheckTSConfig writes a tsconfig for the copy: the app's, with the
// generated paths pointing into the copy.
func writeCheckTSConfig(root, out string) error {
	raw, err := os.ReadFile(filepath.Join(root, codegen.GenDir, "tsconfig.json"))
	if err != nil {
		return err
	}
	var gen struct {
		CompilerOptions struct {
			Paths    map[string][]string `json:"paths"`
			RootDirs []string            `json:"rootDirs"`
		} `json:"compilerOptions"`
		Include []string `json:"include"`
	}
	if err := json.Unmarshal(raw, &gen); err != nil {
		return err
	}
	// Paths in .tern/tsconfig.json are relative to .tern; the copy is one
	// level deeper, with its own src.
	move := func(p string) string {
		switch {
		case p == "..":
			return "."
		case strings.HasPrefix(p, "../src"):
			return "./" + strings.TrimPrefix(p, "../")
		case strings.HasPrefix(p, "./"):
			return "../" + strings.TrimPrefix(p, "./")
		case tsconfigPath.MatchString(p):
			return "../" + p
		}
		return p
	}
	paths := map[string][]string{}
	for k, v := range gen.CompilerOptions.Paths {
		for _, p := range v {
			paths[k] = append(paths[k], move(p))
		}
	}
	var rootDirs, include []string
	for _, p := range gen.CompilerOptions.RootDirs {
		rootDirs = append(rootDirs, move(p))
	}
	for _, p := range gen.Include {
		include = append(include, move(p))
	}
	cfg := map[string]any{
		"//":      "Written by tern check; DO NOT EDIT.",
		"extends": "../../tsconfig.json",
		"compilerOptions": map[string]any{
			"paths":    paths,
			"rootDirs": rootDirs,
		},
		"include": include,
		"exclude": []string{"../../node_modules/**"},
	}
	b, _ := json.MarshalIndent(cfg, "", "\t")
	return os.WriteFile(filepath.Join(out, "tsconfig.json"), b, 0o644)
}
