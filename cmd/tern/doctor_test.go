package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDoctorFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDoctorTarget(t *testing.T) {
	root := t.TempDir()
	writeDoctorFiles(t, root, map[string]string{
		".env.example": "DATABASE_URL=\nSESSION_SECRET=\n",
		"tern.json":    `{"deploy": {"production": {"provider": "ssh", "host": "root@example.com", "env": {"DATABASE_URL": "postgres://x"}}, "staging": {"provider": "ssh", "host": "root@example.com", "domains": ["staging.example.com"], "env": {"DATABASE_URL": "a", "SESSION_SECRET": "b"}}}}`,
	})
	t.Setenv("SESSION_SECRET", "")
	byName := func(cs []doctorCheck) map[string]doctorCheck {
		m := map[string]doctorCheck{}
		for _, c := range cs {
			m[c.Name] = c
		}
		return m
	}
	prod := byName(doctorTarget(root, ""))
	if c := prod["Environment"]; c.Status != checkWarn || !strings.Contains(c.Detail, "SESSION_SECRET") || strings.Contains(c.Detail, "DATABASE_URL") {
		t.Errorf("production env: %+v", c)
	}
	if c := prod["HTTPS"]; c.Status != checkWarn {
		t.Errorf("production HTTPS: %+v", c)
	}
	staging := byName(doctorTarget(root, "staging"))
	if staging["Environment"].Status != checkOK || staging["HTTPS"].Status != checkOK {
		t.Errorf("staging: %+v", staging)
	}
	if c := byName(doctorTarget(root, "nope"))["Deployment"]; c.Status != checkFail {
		t.Errorf("unknown target: %+v", c)
	}
	if c := byName(doctorTarget(t.TempDir(), ""))["Deployment"]; c.Status != checkSkip {
		t.Errorf("no tern.json: %+v", c)
	}
}

func TestDoctorChecks(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	report := ".tern/native/report.txt"
	header := "Native rendering (ADR 0011): what the compiler left to the JavaScript engine.\nA page renders with the engine when it runs one of these.\n\n"
	writeDoctorFiles(t, root, map[string]string{report: header + "intrinsic not used: attr\n"})
	if c := doctorNative(root); c.Status != checkOK {
		t.Errorf("a clean report: %+v", c)
	}
	writeDoctorFiles(t, root, map[string]string{report: header + "src/lib/Chart.svelte: BigInt typed arrays\n"})
	if c := doctorNative(root); c.Status != checkWarn || !strings.Contains(c.Detail, "1 module") {
		t.Errorf("a fallback: %+v", c)
	}
	writeDoctorFiles(t, root, map[string]string{
		"migrations/0001_a.sql": "CREATE TABLE a (id INTEGER);\n",
		"main.go":               "package main\n\nfunc main() { ternApp() }\n",
	})
	if c := doctorMigrations(root); c.Status != checkOK || !strings.Contains(c.Detail, "1 migration") {
		t.Errorf("migrations: %+v", c)
	}
	if c := doctorSecure(root); c.Status != checkWarn || c.Fix != "go tool tern add secure" {
		t.Errorf("secure: %+v", c)
	}
	if !versionAtLeast("go1.27.0", "1.26") || versionAtLeast("go1.25.3", "1.26") || !versionAtLeast("go1.26", "1.26.0") {
		t.Error("versionAtLeast")
	}
}
