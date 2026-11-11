package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Unfield/Tern/internal/ui"
)

// fakeRunner records commands and answers them.
type fakeRunner struct {
	calls  []fakeCall
	answer func(script string) (string, error)
}

type fakeCall struct {
	name   string
	args   []string
	script string // the sh -c script, for ssh
	stdin  []byte
}

func (f *fakeRunner) record(stdin io.Reader, name string, args []string) fakeCall {
	c := fakeCall{name: name, args: args}
	if stdin != nil {
		c.stdin, _ = io.ReadAll(stdin)
	}
	if name == "ssh" {
		last := args[len(args)-1]
		if s, ok := strings.CutPrefix(last, "sh -c '"); ok {
			c.script = strings.ReplaceAll(strings.TrimSuffix(s, "'"), `'\''`, "'")
		}
	}
	f.calls = append(f.calls, c)
	return c
}

func (f *fakeRunner) run(_ context.Context, _ string, stdin io.Reader, _ io.Writer, name string, args ...string) ([]byte, error) {
	c := f.record(stdin, name, args)
	if f.answer != nil {
		out, err := f.answer(c.script)
		return []byte(out), err
	}
	return nil, nil
}

func (f *fakeRunner) stream(_ context.Context, _ string, stdin io.Reader, _, _ io.Writer, name string, args ...string) error {
	f.record(stdin, name, args)
	return nil
}

func testDeployment(t *testing.T) *deployment {
	var out bytes.Buffer
	return &deployment{Action: "deploy", Target: "production", Name: "shop", Root: t.TempDir(), Version: "v1.2.0-3-gabc", Env: map[string]string{"SECRET_KEY": "s3cr3t value"}, ui: ui.New(&out), stdout: &out, stderr: &out}
}

func TestSSHDeploy(t *testing.T) {
	d := testDeployment(t)
	d.Domains = []string{"shop.example.com"}
	bin := filepath.Join(d.Root, "app")
	if err := os.WriteFile(bin, []byte("the binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	fr := &fakeRunner{answer: func(script string) (string, error) {
		if strings.HasPrefix(script, "uname -m") {
			return "aarch64\n1000\nsystemd\n", nil
		}
		return "", nil
	}}
	var builtFor string
	p := &sshProvider{c: sshConfig{Host: "deploy@203.0.113.5", Port: 2222}, run: fr, build: func(_ *deployment, goos, goarch string) (string, error) {
		builtFor = goos + "/" + goarch
		return bin, nil
	}}
	if err := p.deploy(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if builtFor != "linux/arm64" {
		t.Errorf("built for %s", builtFor)
	}
	if len(fr.calls) != 5 {
		t.Fatalf("%d calls", len(fr.calls))
	}
	for _, c := range fr.calls {
		if c.name != "ssh" || !contains(c.args, "-p", "2222") || !contains(c.args, "deploy@203.0.113.5") {
			t.Fatalf("call %v", c.args)
		}
	}
	setup, env, upload, sw := fr.calls[1], fr.calls[2], fr.calls[3], fr.calls[4]
	for _, want := range []string{"sudo -n useradd --system --home-dir /srv/shop", "/etc/systemd/system/shop@.service", "Type=notify", "Environment=TERN_REUSEPORT=1", "ExecStart=/srv/shop/releases/%i/app", "sudo -n systemctl daemon-reload"} {
		if !strings.Contains(setup.script, want) {
			t.Errorf("setup lacks %q:\n%s", want, setup.script)
		}
	}
	if got := string(env.stdin); got != "SECRET_KEY=\"s3cr3t value\"\nTERN_TLS_DOMAINS=shop.example.com\n" {
		t.Errorf("env %q", got)
	}
	zr, err := gzip.NewReader(bytes.NewReader(upload.stdin))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(zr); string(b) != "the binary" {
		t.Errorf("uploaded %q", b)
	}
	if !strings.Contains(upload.script, "gzip -dc | sudo -n tee /srv/shop/releases/") {
		t.Errorf("upload:\n%s", upload.script)
	}
	for _, want := range []string{"sudo -n systemctl start shop@", "-v1.2.0-3-gabc", "systemctl stop \"$u\"", "ln -sfn releases/", "tail -n +6"} {
		if !strings.Contains(sw.script, want) {
			t.Errorf("switch lacks %q:\n%s", want, sw.script)
		}
	}
}

func TestSSHRollback(t *testing.T) {
	d := testDeployment(t)
	fr := &fakeRunner{answer: func(script string) (string, error) {
		switch {
		case strings.HasPrefix(script, "uname -m"):
			return "x86_64\n0\nsystemd\n", nil
		case strings.HasPrefix(script, "ls -1"):
			return "20261101-100000-a\n20261102-100000-b\n20261103-100000-c\n--\nreleases/20261102-100000-b\n", nil
		}
		return "", nil
	}}
	p := &sshProvider{c: sshConfig{Host: "root@x"}, run: fr}
	if err := p.rollback(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	last := fr.calls[len(fr.calls)-1].script
	if !strings.Contains(last, "systemctl start shop@20261101-100000-a") || strings.Contains(last, "sudo") {
		t.Errorf("rollback switch:\n%s", last)
	}
}

func TestSSHChecks(t *testing.T) {
	d := testDeployment(t)
	fr := &fakeRunner{answer: func(string) (string, error) { return "x86_64\n0\nno-systemd\n", nil }}
	p := &sshProvider{c: sshConfig{Host: "root@x"}, run: fr, build: func(*deployment, string, string) (string, error) { return "", nil }}
	if err := p.deploy(context.Background(), d); err == nil || !strings.Contains(err.Error(), "systemd") {
		t.Errorf("no systemd: %v", err)
	}
	d.Name = "Bad Name"
	if err := p.deploy(context.Background(), d); err == nil || !strings.Contains(err.Error(), "name") {
		t.Errorf("bad name: %v", err)
	}
}

func TestContainerDeploy(t *testing.T) {
	d := testDeployment(t)
	fr := &fakeRunner{}
	p := &containerProvider{c: containerConfig{Image: "ghcr.io/me/shop", Tool: "docker", After: []string{"gcloud run deploy shop --image {image}"}}, run: fr}
	if err := p.deploy(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d.Root, "Dockerfile")); err != nil {
		t.Error("no Dockerfile written")
	}
	want := []string{
		"docker build --platform linux/amd64 -t ghcr.io/me/shop:v1.2.0-3-gabc -t ghcr.io/me/shop:latest .",
		"docker push ghcr.io/me/shop:v1.2.0-3-gabc",
		"docker push ghcr.io/me/shop:latest",
	}
	for i, w := range want {
		if got := fr.calls[i].name + " " + strings.Join(fr.calls[i].args, " "); got != w {
			t.Errorf("call %d: %s", i, got)
		}
	}
	if runtime.GOOS != "windows" {
		if last := fr.calls[3]; strings.Join(last.args, " ") != "-c gcloud run deploy shop --image ghcr.io/me/shop:v1.2.0-3-gabc" {
			t.Errorf("after: %v", last.args)
		}
	}
}

func TestExternalProvider(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	dir := t.TempDir()
	got := filepath.Join(dir, "got.json")
	script := "#!/bin/sh\ncat > " + got + "\necho deployed\n"
	if err := os.WriteFile(filepath.Join(dir, "tern-deploy-acme"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	raw := json.RawMessage(`{"provider":"acme","project":"p1"}`)
	p, err := providerFor(target{Provider: "acme", raw: raw})
	if err != nil {
		t.Fatal(err)
	}
	d := testDeployment(t)
	d.Config = raw
	if err := p.deploy(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(got)
	var sent map[string]any
	if err := json.Unmarshal(b, &sent); err != nil {
		t.Fatal(string(b), err)
	}
	if sent["action"] != "deploy" || sent["name"] != "shop" || sent["config"].(map[string]any)["project"] != "p1" || sent["env"].(map[string]any)["SECRET_KEY"] != "s3cr3t value" {
		t.Errorf("sent %v", sent)
	}
	if _, err := providerFor(target{Provider: "nonesuch"}); err == nil || !strings.Contains(err.Error(), "tern-deploy-nonesuch") {
		t.Errorf("missing provider: %v", err)
	}
}

func TestLoadTargets(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "tern.json"), []byte(`{"basePath": "", "deploy": {"prod": {"provider": "ssh", "host": "root@x", "domains": ["a.example"], "envFile": ".env.prod", "env": {"B": "2"}}}}`), 0o644)
	_ = os.WriteFile(filepath.Join(root, ".env.prod"), []byte("A=1\nB=1\n"), 0o644)
	ts, err := loadTargets(root)
	if err != nil {
		t.Fatal(err)
	}
	tg := ts["prod"]
	if tg.Provider != "ssh" || len(tg.Domains) != 1 {
		t.Fatalf("%+v", tg)
	}
	env, err := targetEnv(root, tg)
	if err != nil || env["A"] != "1" || env["B"] != "2" {
		t.Fatalf("env %v %v", env, err)
	}
	p, err := providerFor(tg)
	if err != nil || p.(*sshProvider).c.Host != "root@x" {
		t.Fatalf("%v %v", p, err)
	}
}

func TestDeployHelpers(t *testing.T) {
	if got := releaseID(time.Date(2026, 11, 11, 15, 4, 5, 0, time.UTC), "v1.2.0 dirty/x"); got != "20261111-150405-v1.2.0-dirty-x" {
		t.Error(got)
	}
	if got := appName("/x/My Shop_2"); got != "my-shop-2" {
		t.Error(got)
	}
	if got := shQuote("it's"); got != `'it'\''s'` {
		t.Error(got)
	}
	if got := envLines(map[string]string{"A": "plain", "B": `a "b" $c`, "C": ""}); got != "A=plain\nB=\"a \\\"b\\\" \\$c\"\nC=\"\"\n" {
		t.Errorf("%q", got)
	}
}

func contains(list []string, seq ...string) bool {
	for i := 0; i+len(seq) <= len(list); i++ {
		match := true
		for j, s := range seq {
			if list[i+j] != s {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
