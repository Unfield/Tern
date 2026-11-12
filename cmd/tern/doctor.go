package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Unfield/Tern/internal/codegen"
	"github.com/Unfield/Tern/internal/routes"
	"github.com/Unfield/Tern/internal/ui"
	"github.com/Unfield/Tern/migrate"
)

// tern doctor checks a project before it's deployed: the tools, the
// versions of Tern's two halves, code generation, native rendering, the
// migrations and the deployment's settings.

type checkStatus int

const (
	checkOK checkStatus = iota
	checkWarn
	checkFail
	checkSkip
)

type doctorCheck struct {
	Name   string      `json:"name"`
	Status checkStatus `json:"-"`
	State  string      `json:"status"` // ok, warning, failure or skipped
	Detail string      `json:"detail"`
	// Fix says what to do about a warning or failure.
	Fix string `json:"fix,omitempty"`
}

func cmdDoctor(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "print the checks as JSON")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: tern doctor [-json] [target]\n\nChecks the project before a deploy: tools and versions, code generation, native rendering, migrations, and the target's settings in tern.json (the only one, or \"production\", by default). It exits 1 if a check fails.")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	root, _ := os.Getwd()
	checks := runDoctor(root, fs.Arg(0))
	failed := slices.ContainsFunc(checks, func(c doctorCheck) bool { return c.Status == checkFail })
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(checks)
	} else {
		printDoctor(stdout, checks)
	}
	if failed {
		return 1
	}
	return 0
}

func printDoctor(w io.Writer, checks []doctorCheck) {
	p := ui.New(w)
	p.Println()
	warns, fails := 0, 0
	for _, c := range checks {
		mark := p.Green("✓")
		switch c.Status {
		case checkWarn:
			mark, warns = p.Yellow("!"), warns+1
		case checkFail:
			mark, fails = p.Red("✗"), fails+1
		case checkSkip:
			mark = p.Dim("–")
		}
		detail := c.Detail
		if c.Status == checkSkip || c.Status == checkOK {
			detail = p.Dim(detail)
		}
		p.Printf("  %s %s%s %s\n", mark, p.Bold(c.Name), pad(c.Name, 18), detail)
		if c.Fix != "" && (c.Status == checkWarn || c.Status == checkFail) {
			p.Printf("    %s %s\n", strings.Repeat(" ", 18), p.Cyan(c.Fix))
		}
	}
	p.Println()
	switch {
	case fails > 0:
		p.Printf("  %s\n\n", p.Red(plural(fails, "check")+" failed"))
	case warns > 0:
		p.Printf("  %s\n\n", p.Yellow(plural(warns, "warning")))
	default:
		p.Printf("  %s\n\n", p.Green("Ready to deploy"))
	}
}

func runDoctor(root, targetName string) []doctorCheck {
	var checks []doctorCheck
	checks = append(checks, doctorGo(root), doctorNode(), doctorVersions(root), doctorGen(root), doctorNative(root), doctorMigrations(root))
	checks = append(checks, doctorTarget(root, targetName)...)
	checks = append(checks, doctorSecure(root), doctorGit(root))
	for i := range checks {
		checks[i].State = map[checkStatus]string{checkOK: "ok", checkWarn: "warning", checkFail: "failure", checkSkip: "skipped"}[checks[i].Status]
	}
	return checks
}

var goVersionRe = regexp.MustCompile(`go(\d+)\.(\d+)`)

func versionAtLeast(have, want string) bool {
	h, w := goVersionRe.FindStringSubmatch("go"+strings.TrimPrefix(have, "go")), goVersionRe.FindStringSubmatch("go"+strings.TrimPrefix(want, "go"))
	if h == nil || w == nil {
		return true
	}
	hm, _ := strconv.Atoi(h[1])
	hn, _ := strconv.Atoi(h[2])
	wm, _ := strconv.Atoi(w[1])
	wn, _ := strconv.Atoi(w[2])
	return hm > wm || hm == wm && hn >= wn
}

func doctorGo(root string) doctorCheck {
	out, err := exec.Command("go", "env", "GOVERSION").Output()
	if err != nil {
		return doctorCheck{Name: "Go", Status: checkFail, Detail: "go isn't on the PATH", Fix: "Install Go from https://go.dev/dl"}
	}
	have := strings.TrimSpace(string(out))
	mod, _ := os.ReadFile(filepath.Join(root, "go.mod"))
	want := ""
	if m := regexp.MustCompile(`(?m)^go (\S+)`).FindSubmatch(mod); m != nil {
		want = string(m[1])
	}
	if want != "" && !versionAtLeast(have, want) {
		return doctorCheck{Name: "Go", Status: checkFail, Detail: fmt.Sprintf("%s, and go.mod wants %s", have, want), Fix: "Install Go " + want + " or later"}
	}
	return doctorCheck{Name: "Go", Status: checkOK, Detail: have}
}

func doctorNode() doctorCheck {
	out, err := exec.Command("node", "--version").Output()
	if err != nil {
		return doctorCheck{Name: "Node", Status: checkFail, Detail: "node isn't on the PATH: builds need it, the app doesn't", Fix: "Install Node 22 or later from https://nodejs.org"}
	}
	v := strings.TrimSpace(string(out))
	major, _ := strconv.Atoi(strings.TrimPrefix(strings.SplitN(v, ".", 2)[0], "v"))
	if major < 22 {
		return doctorCheck{Name: "Node", Status: checkFail, Detail: v + ", and Tern needs 22 or later", Fix: "Install Node 22 or later"}
	}
	return doctorCheck{Name: "Node", Status: checkOK, Detail: v}
}

// doctorVersions compares the Go module's version of Tern with the npm
// package's: they're released together and must match.
func doctorVersions(root string) doctorCheck {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Version}}{{if .Replace}} {{.Replace.Path}}{{end}}", "github.com/Unfield/Tern")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return doctorCheck{Name: "Tern", Status: checkFail, Detail: "github.com/Unfield/Tern isn't in go.mod", Fix: "go get github.com/Unfield/Tern@latest"}
	}
	goV, replaced, _ := strings.Cut(strings.TrimSpace(string(out)), " ")
	pkg, err := os.ReadFile(filepath.Join(root, "node_modules", "@ternkit", "tern", "package.json"))
	if err != nil {
		return doctorCheck{Name: "Tern", Status: checkFail, Detail: "@ternkit/tern isn't installed", Fix: "npm install"}
	}
	var p struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal(pkg, &p)
	if replaced != "" || goV == "" {
		return doctorCheck{Name: "Tern", Status: checkOK, Detail: fmt.Sprintf("npm %s, Go module replaced with %s", p.Version, replaced)}
	}
	if strings.TrimPrefix(goV, "v") != p.Version {
		return doctorCheck{Name: "Tern", Status: checkFail, Detail: fmt.Sprintf("the Go module is %s and the npm package %s", goV, p.Version), Fix: fmt.Sprintf("npm install @ternkit/tern@%s, or go get github.com/Unfield/Tern@v%s", strings.TrimPrefix(goV, "v"), p.Version)}
	}
	return doctorCheck{Name: "Tern", Status: checkOK, Detail: goV + " on both sides"}
}

func doctorGen(root string) doctorCheck {
	if _, err := codegen.Generate(root); err != nil {
		first, _, _ := strings.Cut(err.Error(), "\n")
		return doctorCheck{Name: "Routes", Status: checkFail, Detail: first, Fix: "Fix it, then run go tool tern gen"}
	}
	m, err := routes.Scan(os.DirFS(root), defaultRoutesDir)
	if err != nil {
		return doctorCheck{Name: "Routes", Status: checkFail, Detail: err.Error(), Fix: "Fix the routes"}
	}
	return doctorCheck{Name: "Routes", Status: checkOK, Detail: plural(len(m.Routes), "route") + ", generated"}
}

// doctorNative reads the last build's native rendering report: what it
// lists renders with the JavaScript engine.
func doctorNative(root string) doctorCheck {
	b, err := os.ReadFile(filepath.Join(root, ".tern", "native", "report.txt"))
	if err != nil {
		return doctorCheck{Name: "Native rendering", Status: checkSkip, Detail: "no build yet: run go tool tern build to check it"}
	}
	// writeReport's format: two lines about the report, a blank one, then
	// what falls back, one a line.
	_, body, _ := strings.Cut(string(b), "\n\n")
	var problems []string
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "The compiler failed") {
			return doctorCheck{Name: "Native rendering", Status: checkWarn, Detail: "the compiler failed, so every page renders with the JavaScript engine", Fix: "See .tern/native/report.txt"}
		}
		if line != "" && !strings.HasPrefix(line, "intrinsic not used:") {
			problems = append(problems, line)
		}
	}
	if len(problems) == 0 {
		return doctorCheck{Name: "Native rendering", Status: checkOK, Detail: "every page renders in Go"}
	}
	return doctorCheck{Name: "Native rendering", Status: checkWarn, Detail: fmt.Sprintf("%s fall back to the JavaScript engine, such as %s", plural(len(problems), "module"), problems[0]), Fix: "See .tern/native/report.txt"}
}

func doctorMigrations(root string) doctorCheck {
	dir := filepath.Join(root, "migrations")
	if _, err := os.Stat(dir); err != nil {
		return doctorCheck{Name: "Migrations", Status: checkSkip, Detail: "no migrations folder"}
	}
	ms, err := migrate.Load(os.DirFS(dir))
	if err != nil {
		return doctorCheck{Name: "Migrations", Status: checkFail, Detail: err.Error(), Fix: "Fix the migration files"}
	}
	return doctorCheck{Name: "Migrations", Status: checkOK, Detail: plural(len(ms), "migration") + ", applied when the app starts"}
}

// doctorTarget checks the deployment target: its environment against
// .env.example, and HTTPS.
func doctorTarget(root, name string) []doctorCheck {
	skip := func(detail string) []doctorCheck {
		return []doctorCheck{{Name: "Deployment", Status: checkSkip, Detail: detail}}
	}
	targets, err := loadTargets(root)
	if err != nil {
		return []doctorCheck{{Name: "Deployment", Status: checkFail, Detail: err.Error(), Fix: "Fix tern.json"}}
	}
	if len(targets) == 0 {
		return skip("no deploy targets in tern.json")
	}
	if name == "" {
		if _, ok := targets["production"]; ok {
			name = "production"
		} else if len(targets) == 1 {
			for n := range targets {
				name = n
			}
		} else {
			return skip("several targets: name one, such as tern doctor staging")
		}
	}
	t, ok := targets[name]
	if !ok {
		return []doctorCheck{{Name: "Deployment", Status: checkFail, Detail: fmt.Sprintf("no target %q in tern.json", name)}}
	}
	var out []doctorCheck
	env, err := targetEnv(root, t)
	switch {
	case err != nil:
		out = append(out, doctorCheck{Name: "Environment", Status: checkFail, Detail: err.Error(), Fix: "Fix envFile in tern.json"})
	default:
		example := envKeys(filepath.Join(root, ".env.example"))
		var missing []string
		for _, k := range example {
			if _, ok := env[k]; !ok && os.Getenv(k) == "" {
				missing = append(missing, k)
			}
		}
		switch {
		case example == nil:
			out = append(out, doctorCheck{Name: "Environment", Status: checkSkip, Detail: "no .env.example to check " + name + "'s environment against"})
		case len(missing) > 0:
			out = append(out, doctorCheck{Name: "Environment", Status: checkWarn, Detail: fmt.Sprintf("%s doesn't set %s, from .env.example", name, strings.Join(missing, ", ")), Fix: "Set them in tern.json's env or envFile for " + name})
		default:
			out = append(out, doctorCheck{Name: "Environment", Status: checkOK, Detail: fmt.Sprintf("%s sets everything in .env.example", name)})
		}
	}
	if len(t.Domains) > 0 {
		out = append(out, doctorCheck{Name: "HTTPS", Status: checkOK, Detail: "served by the app for " + strings.Join(t.Domains, ", ")})
	} else {
		out = append(out, doctorCheck{Name: "HTTPS", Status: checkWarn, Detail: name + " has no domains, so the app doesn't serve HTTPS itself", Fix: "Add domains to the target, or serve HTTPS from a proxy in front of it"})
	}
	return out
}

// envKeys lists the variables a .env file names, with values or without,
// such as .env.example's; nil if there's no file.
func envKeys(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	keys := []string{}
	for line := range strings.SplitSeq(string(b), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		if k, _, ok := strings.Cut(line, "="); ok && line[0] != '#' && strings.TrimSpace(k) != "" {
			keys = append(keys, strings.TrimSpace(k))
		}
	}
	return keys
}

// securityOptOuts are the options that turn off a protection Tern has on
// by default (ADR 0027), and what each one costs.
var securityOptOuts = []struct{ option, cost string }{
	{"WithoutCSRFProtection", "other sites can post forms to the app"},
	{"WithoutContentSecurityPolicy", "injected markup can run scripts"},
	{"WithoutSecurityHeaders", "pages can be framed by other sites"},
	{"WithoutRateLimit", "nothing slows down a script hammering forms"},
}

// doctorSecure looks in the app's own Go files at the root for options that
// turn protections off.
func doctorSecure(root string) doctorCheck {
	files, _ := filepath.Glob(filepath.Join(root, "*.go"))
	var found []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || strings.HasSuffix(f, ".gen.go") {
			continue
		}
		b, _ := os.ReadFile(f)
		for _, o := range securityOptOuts {
			if bytes.Contains(b, []byte("kit."+o.option+"(")) && !slices.Contains(found, o.option) {
				found = append(found, o.option)
			}
		}
	}
	if len(found) == 0 {
		return doctorCheck{Name: "Security", Status: checkOK, Detail: "headers, Content-Security-Policy, CSRF protection and rate limits on"}
	}
	var costs []string
	for _, o := range securityOptOuts {
		if slices.Contains(found, o.option) {
			costs = append(costs, "kit."+o.option+": "+o.cost)
		}
	}
	return doctorCheck{Name: "Security", Status: checkWarn, Detail: strings.Join(costs, "; "), Fix: "Turn protections off only where needed; see docs/security.md"}
}

func doctorGit(root string) doctorCheck {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return doctorCheck{Name: "Git", Status: checkSkip, Detail: "not a git repository"}
	}
	if n := len(strings.Split(strings.TrimSpace(string(out)), "\n")); strings.TrimSpace(string(out)) != "" {
		return doctorCheck{Name: "Git", Status: checkWarn, Detail: plural(n, "uncommitted change") + ": the release is named after a dirty commit", Fix: "Commit before deploying"}
	}
	return doctorCheck{Name: "Git", Status: checkOK, Detail: "everything committed"}
}
