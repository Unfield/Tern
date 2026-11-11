package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/Unfield/Tern/internal/codegen"
	"github.com/Unfield/Tern/internal/ui"
)

// tern deploy pushes the app to a target named in tern.json's "deploy":
//
//	"deploy": {
//		"production": {"provider": "ssh", "host": "root@203.0.113.5", "domains": ["example.com"]}
//	}
//
// Each target names a provider: ssh, fly or container, built in, or any
// other name, which runs tern-deploy-<name> from the PATH with the
// deployment as JSON on its standard input. Platforms can ship their own
// provider that way.

// target is a deployment target from tern.json.
type target struct {
	// Provider is "ssh", "fly", "container", or the name of an external
	// provider.
	Provider string `json:"provider"`
	// Name is the app's name on the target. The default is the project
	// directory's.
	Name string `json:"name"`
	// Env is set in the app's environment, after EnvFile's.
	Env     map[string]string `json:"env"`
	EnvFile string            `json:"envFile"`
	// Domains are served over HTTPS by the app itself, with certificates
	// from Let's Encrypt, where the provider supports it.
	Domains []string `json:"domains"`
	// raw is the whole entry, for the provider's own fields.
	raw json.RawMessage
}

// deployment is what a provider is given.
type deployment struct {
	Action  string            `json:"action"` // deploy, rollback, releases or logs
	Target  string            `json:"target"`
	Name    string            `json:"name"`
	Root    string            `json:"root"`
	Version string            `json:"version"`
	Env     map[string]string `json:"env"`
	Domains []string          `json:"domains"`
	Config  json.RawMessage   `json:"config"`

	ui     *ui.Printer
	stdout io.Writer
	stderr io.Writer
}

// provider deploys to one kind of target.
type provider interface {
	deploy(ctx context.Context, d *deployment) error
}

// rollbacker can go back to the previous release; lister lists releases;
// logger follows the app's logs.
type (
	rollbacker interface {
		rollback(ctx context.Context, d *deployment) error
	}
	lister interface {
		releases(ctx context.Context, d *deployment) error
	}
	logFollower interface {
		logs(ctx context.Context, d *deployment) error
	}
)

func providerFor(t target) (provider, error) {
	switch t.Provider {
	case "ssh":
		var c sshConfig
		if err := json.Unmarshal(t.raw, &c); err != nil {
			return nil, fmt.Errorf("ssh target: %w", err)
		}
		if c.Host == "" {
			return nil, errors.New(`ssh target: "host" is missing, such as "root@203.0.113.5"`)
		}
		return &sshProvider{c: c, run: execRunner{}}, nil
	case "fly":
		var c flyConfig
		if err := json.Unmarshal(t.raw, &c); err != nil {
			return nil, fmt.Errorf("fly target: %w", err)
		}
		return &flyProvider{c: c, run: execRunner{}}, nil
	case "container":
		var c containerConfig
		if err := json.Unmarshal(t.raw, &c); err != nil {
			return nil, fmt.Errorf("container target: %w", err)
		}
		if c.Image == "" {
			return nil, errors.New(`container target: "image" is missing, such as "ghcr.io/you/app"`)
		}
		return &containerProvider{c: c, run: execRunner{}}, nil
	case "":
		return nil, errors.New(`the target has no "provider": ssh, fly, container, or the name of a tern-deploy-<name> program`)
	}
	bin := "tern-deploy-" + t.Provider
	path, err := exec.LookPath(bin)
	if err != nil {
		return nil, fmt.Errorf("no provider %q: Tern has ssh, fly and container built in, and found no %s program on the PATH", t.Provider, bin)
	}
	return &externalProvider{path: path}, nil
}

func loadTargets(root string) (map[string]target, error) {
	b, err := os.ReadFile(filepath.Join(root, codegen.ConfigFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Deploy map[string]json.RawMessage `json:"deploy"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", codegen.ConfigFile, err)
	}
	out := map[string]target{}
	for name, raw := range cfg.Deploy {
		var t target
		if err := json.Unmarshal(raw, &t); err != nil {
			return nil, fmt.Errorf("%s: deploy.%s: %w", codegen.ConfigFile, name, err)
		}
		t.raw = raw
		out[name] = t
	}
	return out, nil
}

func cmdDeploy(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("deploy", flag.ContinueOnError)
	fl.SetOutput(stderr)
	rollback := fl.Bool("rollback", false, "go back to the release before the current one")
	releases := fl.Bool("releases", false, "list the target's releases")
	logs := fl.Bool("logs", false, "follow the app's logs on the target")
	host := fl.String("host", "", "deploy over SSH to this host, such as root@203.0.113.5, without a target in tern.json")
	var domains stringList
	fl.Var(&domains, "domain", "with -host: a domain to serve over HTTPS (repeatable)")
	fl.Usage = func() {
		fmt.Fprintln(stderr, "usage: tern deploy [target] [-rollback | -releases | -logs]\n       tern deploy -host user@server [-domain example.com]")
		fl.PrintDefaults()
	}
	// Flags may come after the target's name.
	var rest []string
	for len(args) > 0 {
		if err := fl.Parse(args); err != nil {
			return 2
		}
		args = fl.Args()
		if len(args) > 0 {
			rest = append(rest, args[0])
			args = args[1:]
		}
	}
	e := ui.New(stderr)
	root, err := os.Getwd()
	if err != nil {
		e.Error(err)
		return 1
	}
	targets, err := loadTargets(root)
	if err != nil {
		e.Error(err)
		return 1
	}
	if len(rest) > 1 {
		fl.Usage()
		return 2
	}

	var name string
	var t target
	switch {
	case *host != "":
		raw, _ := json.Marshal(map[string]any{"provider": "ssh", "host": *host, "domains": []string(domains)})
		t = target{Provider: "ssh", Domains: domains, raw: raw}
		name = *host
	case len(rest) == 1:
		name = rest[0]
		var ok bool
		if t, ok = targets[name]; !ok {
			e.Error(fmt.Errorf("no deploy target %q in %s%s", name, codegen.ConfigFile, targetList(targets)))
			return 1
		}
	case len(targets) == 1:
		for n, tt := range targets {
			name, t = n, tt
		}
	case len(targets) == 0:
		e.Error(fmt.Errorf("no deploy targets: add one to %s, such as\n\n%s\n\nor deploy to a server with tern deploy -host root@your-server", codegen.ConfigFile, exampleTarget))
		return 1
	default:
		e.Error(fmt.Errorf("which target?%s", targetList(targets)))
		return 1
	}

	p, err := providerFor(t)
	if err != nil {
		e.Error(err)
		return 1
	}
	d := &deployment{Action: "deploy", Target: name, Name: t.Name, Root: root, Domains: t.Domains, Config: t.raw, ui: ui.New(stdout), stdout: stdout, stderr: stderr}
	if d.Name == "" {
		d.Name = appName(root)
	}
	if d.Env, err = targetEnv(root, t); err != nil {
		e.Error(err)
		return 1
	}
	d.Version = releaseVersion(root)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pr := d.ui
	switch {
	case *rollback:
		d.Action = "rollback"
		r, ok := p.(rollbacker)
		if !ok {
			e.Error(fmt.Errorf("the %s provider can't roll back; deploy the release you want again", t.Provider))
			return 1
		}
		err = r.rollback(ctx, d)
	case *releases:
		d.Action = "releases"
		l, ok := p.(lister)
		if !ok {
			e.Error(fmt.Errorf("the %s provider doesn't list releases", t.Provider))
			return 1
		}
		err = l.releases(ctx, d)
	case *logs:
		d.Action = "logs"
		l, ok := p.(logFollower)
		if !ok {
			e.Error(fmt.Errorf("the %s provider doesn't show logs", t.Provider))
			return 1
		}
		err = l.logs(ctx, d)
	default:
		start := time.Now()
		pr.Printf("\n  %s %s %s\n\n", pr.Brand(pr.Bold("tern deploy")), pr.Bold(name), pr.Dim(t.Provider+" · "+d.Version))
		if err = p.deploy(ctx, d); err == nil {
			pr.Printf("\n  %s %s %s\n\n", pr.Green("✓"), pr.Bold("Deployed "+d.Name), pr.Dim("in "+ui.Duration(time.Since(start))))
		}
	}
	if err != nil {
		e.Println()
		e.Error(err)
		return 1
	}
	return 0
}

const exampleTarget = `  "deploy": {
    "production": { "provider": "ssh", "host": "root@203.0.113.5", "domains": ["example.com"] }
  }`

func targetList(targets map[string]target) string {
	if len(targets) == 0 {
		return ""
	}
	names := make([]string, 0, len(targets))
	for n := range targets {
		names = append(names, n)
	}
	sort.Strings(names)
	return "; the targets are " + strings.Join(names, ", ")
}

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

// appName is a name for the app made of the directory's: lowercase
// letters, digits and dashes.
func appName(root string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(filepath.Base(root)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "app"
	}
	return name
}

// targetEnv is the target's environment: EnvFile's variables, then Env's.
func targetEnv(root string, t target) (map[string]string, error) {
	env := map[string]string{}
	if t.EnvFile != "" {
		vars, err := readDotEnv(filepath.Join(root, t.EnvFile))
		if err != nil {
			return nil, err
		}
		if vars == nil {
			return nil, fmt.Errorf("envFile %s doesn't exist", t.EnvFile)
		}
		for _, kv := range vars {
			k, v, _ := strings.Cut(kv, "=")
			env[k] = v
		}
	}
	for k, v := range t.Env {
		env[k] = v
	}
	return env, nil
}

// releaseVersion names the release: the commit, from git describe, or the time.
func releaseVersion(root string) string {
	cmd := exec.Command("git", "describe", "--tags", "--always", "--dirty")
	cmd.Dir = root
	if out, err := cmd.Output(); err == nil {
		if v := strings.TrimSpace(string(out)); v != "" {
			return v
		}
	}
	return time.Now().UTC().Format("20060102-150405")
}

// envLines writes env as KEY=VALUE lines, sorted, with values quoted where
// they need it: the format of .env files and of systemd's EnvironmentFile.
func envLines(env map[string]string) string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var b strings.Builder
	for _, k := range keys {
		v := env[k]
		if strings.ContainsAny(v, " \t\"'\\#$\n") || v == "" {
			v = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, `$`, `\$`).Replace(v) + `"`
		}
		b.WriteString(k + "=" + v + "\n")
	}
	return b.String()
}

// runner runs commands; tests replace it.
type runner interface {
	// run runs name with args in dir, with stdin, and returns its
	// standard output. Its standard error goes to stderr when set.
	run(ctx context.Context, dir string, stdin io.Reader, stderr io.Writer, name string, args ...string) ([]byte, error)
	// stream runs it with its output on out and err.
	stream(ctx context.Context, dir string, stdin io.Reader, out, err io.Writer, name string, args ...string) error
}

type execRunner struct{}

func (execRunner) run(ctx context.Context, dir string, stdin io.Reader, stderr io.Writer, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Stdin = dir, stdin
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if stderr != nil {
		cmd.Stderr = io.MultiWriter(&errBuf, stderr)
	}
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(errBuf.String()); msg != "" {
			return out, fmt.Errorf("%s: %w\n%s", name, err, msg)
		}
		return out, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

func (execRunner) stream(ctx context.Context, dir string, stdin io.Reader, out, errw io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Stdin, cmd.Stdout, cmd.Stderr = dir, stdin, out, errw
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// externalProvider runs tern-deploy-<name> with the deployment as JSON on
// its standard input, and its output on tern's.
type externalProvider struct{ path string }

func (x *externalProvider) call(ctx context.Context, d *deployment) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return execRunner{}.stream(ctx, d.Root, bytes.NewReader(b), d.stdout, d.stderr, x.path)
}

func (x *externalProvider) deploy(ctx context.Context, d *deployment) error   { return x.call(ctx, d) }
func (x *externalProvider) rollback(ctx context.Context, d *deployment) error { return x.call(ctx, d) }
func (x *externalProvider) releases(ctx context.Context, d *deployment) error { return x.call(ctx, d) }
func (x *externalProvider) logs(ctx context.Context, d *deployment) error     { return x.call(ctx, d) }
