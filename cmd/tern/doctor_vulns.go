package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/mod/semver"
	"golang.org/x/vuln/scan"
)

// govulnFinding is the part of govulncheck's JSON output doctor reads: a
// finding whose trace starts at a function is one the app's code calls.
type govulnMessage struct {
	Finding *struct {
		OSV          string `json:"osv"`
		FixedVersion string `json:"fixed_version"`
		Trace        []struct {
			Module   string `json:"module"`
			Function string `json:"function"`
		} `json:"trace"`
	} `json:"finding"`
}

// doctorGoVulns runs govulncheck on the app: known vulnerabilities in the
// Go modules and the Go version it builds with, counted only when the
// app's code reaches the vulnerable function.
func doctorGoVulns(root string) doctorCheck {
	name := "Go modules"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var out, errs bytes.Buffer
	cmd := scan.Command(ctx, "-C", root, "-json", "./...")
	cmd.Stdout, cmd.Stderr = &out, &errs
	if err := cmd.Start(); err == nil {
		_ = cmd.Wait() // the output says what was found
	}
	called, found := govulnFindings(&out)
	if found == 0 && errs.Len() > 0 {
		msg := strings.TrimSpace(errs.String())
		if i := strings.IndexByte(msg, '\n'); i > 0 {
			msg = msg[:i]
		}
		return doctorCheck{Name: name, Status: checkSkip, Detail: "govulncheck didn't run: " + msg}
	}
	if len(called) == 0 {
		detail := "no known vulnerabilities the app's code reaches"
		if found > 0 {
			detail += fmt.Sprintf(" (%d in code it doesn't call)", found)
		}
		return doctorCheck{Name: name, Status: checkOK, Detail: detail}
	}
	n := 0
	var parts, fixes []string
	for _, m := range called {
		n += m.count
		what := m.module
		if m.module == "stdlib" {
			what = "Go's standard library"
		}
		parts = append(parts, fmt.Sprintf("%d in %s", m.count, what))
		switch {
		case m.fixed == "":
		case m.module == "stdlib":
			fixes = append(fixes, "Go "+strings.TrimPrefix(m.fixed, "v"))
		default:
			fixes = append(fixes, m.module+"@"+m.fixed)
		}
	}
	noun := "known vulnerabilities"
	if n == 1 {
		noun = "known vulnerability"
	}
	fix := "go run golang.org/x/vuln/cmd/govulncheck@latest ./... shows them"
	if len(fixes) > 0 {
		fix = "Update to " + strings.Join(fixes, ", ") + "; " + fix
	}
	return doctorCheck{Name: name, Status: checkFail, Detail: fmt.Sprintf("%d %s the app's code reaches: %s", n, noun, strings.Join(parts, ", ")), Fix: fix}
}

// vulnModule is the vulnerabilities the app reaches in one module, and the
// version that fixes them all.
type vulnModule struct {
	module, fixed string
	count         int
}

// govulnFindings reads govulncheck's JSON stream: the vulnerabilities the
// code calls, by module, and how many were found in all.
func govulnFindings(r io.Reader) (called []vulnModule, found int) {
	dec := json.NewDecoder(r)
	all, reached := map[string]bool{}, map[string]bool{}
	for {
		var m govulnMessage
		if err := dec.Decode(&m); err != nil {
			break
		}
		f := m.Finding
		if f == nil || f.OSV == "" {
			continue
		}
		all[f.OSV] = true
		if len(f.Trace) == 0 || f.Trace[0].Function == "" || reached[f.OSV] {
			continue
		}
		reached[f.OSV] = true
		mod := f.Trace[0].Module
		i := slices.IndexFunc(called, func(v vulnModule) bool { return v.module == mod })
		if i < 0 {
			called = append(called, vulnModule{module: mod})
			i = len(called) - 1
		}
		called[i].count++
		if f.FixedVersion != "" && (called[i].fixed == "" || semver.Compare(f.FixedVersion, called[i].fixed) > 0) {
			called[i].fixed = f.FixedVersion
		}
	}
	return called, len(all)
}

// npmAudit is the part of npm's and pnpm's audit JSON doctor reads.
type npmAudit struct {
	Metadata struct {
		Vulnerabilities map[string]int `json:"vulnerabilities"`
	} `json:"metadata"`
}

// doctorNPMVulns audits the npm packages, which end up in the browser:
// high and critical advisories are warnings.
func doctorNPMVulns(root string) doctorCheck {
	name := "npm packages"
	var tool []string
	switch {
	case fileExists(filepath.Join(root, "pnpm-lock.yaml")) || workspaceLock(root, "pnpm-lock.yaml"):
		tool = []string{"pnpm", "audit", "--json"}
	case fileExists(filepath.Join(root, "package-lock.json")):
		tool = []string{"npm", "audit", "--json"}
	default:
		return doctorCheck{Name: name, Status: checkSkip, Detail: "no package-lock.json or pnpm-lock.yaml"}
	}
	if _, err := exec.LookPath(tool[0]); err != nil {
		return doctorCheck{Name: name, Status: checkSkip, Detail: tool[0] + " isn't installed"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool[0], tool[1:]...)
	cmd.Dir = root
	out, _ := cmd.Output() // exits 1 when it finds something
	var a npmAudit
	if err := json.Unmarshal(out, &a); err != nil || a.Metadata.Vulnerabilities == nil {
		return doctorCheck{Name: name, Status: checkSkip, Detail: strings.Join(tool[:2], " ") + " gave no report (offline?)"}
	}
	v := a.Metadata.Vulnerabilities
	serious, other := v["high"]+v["critical"], v["low"]+v["moderate"]+v["info"]
	switch {
	case serious > 0:
		return doctorCheck{Name: name, Status: checkWarn, Detail: fmt.Sprintf("%d high or critical advisories, %d lower", serious, other),
			Fix: strings.Join(tool[:2], " ") + " shows them; " + tool[0] + " update fixes most"}
	case other > 0:
		return doctorCheck{Name: name, Status: checkOK, Detail: fmt.Sprintf("no high or critical advisories (%d lower)", other)}
	}
	return doctorCheck{Name: name, Status: checkOK, Detail: "no known advisories"}
}

// workspaceLock reports whether a lockfile named name is in a parent of
// root, as in a pnpm workspace.
func workspaceLock(root, name string) bool {
	for dir := filepath.Dir(root); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if fileExists(filepath.Join(dir, name)) {
			return true
		}
		if fileExists(filepath.Join(dir, ".git")) {
			return false
		}
	}
	return false
}
