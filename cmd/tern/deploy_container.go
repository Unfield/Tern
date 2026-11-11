package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// The fly and container providers build the project's Dockerfile. A
// project without one gets the one tern new writes.

// ensureDockerfile writes the default Dockerfile and .dockerignore if the
// project has none.
func ensureDockerfile(d *deployment) error {
	path := filepath.Join(d.Root, "Dockerfile")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	b, err := templateFS.ReadFile("template/Dockerfile")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(d.Root, ".dockerignore")); errors.Is(err, os.ErrNotExist) {
		if ig, err := templateFS.ReadFile("template/_dockerignore"); err == nil {
			_ = os.WriteFile(filepath.Join(d.Root, ".dockerignore"), ig, 0o644)
		}
	}
	d.ui.Printf("  %s %s\n", d.ui.Green("+"), "Wrote a Dockerfile (commit it, and change it as you need)")
	return nil
}

// flyConfig is a Fly.io target.
type flyConfig struct {
	// App is the Fly app's name. The default is the app's name.
	App string `json:"app"`
	// Region is the primary region for a new app, such as "fra".
	Region string `json:"region"`
	// Org is the organization a new app is created in.
	Org string `json:"org"`
}

type flyProvider struct {
	c   flyConfig
	run runner
}

func (f *flyProvider) bin() (string, error) {
	for _, n := range []string{"fly", "flyctl"} {
		if p, err := exec.LookPath(n); err == nil {
			return p, nil
		}
	}
	return "", errors.New("the fly provider needs flyctl: https://fly.io/docs/flyctl/install/")
}

func (f *flyProvider) app(d *deployment) string {
	if f.c.App != "" {
		return f.c.App
	}
	return d.Name
}

const flyToml = `# fly.toml, written by tern deploy. See https://fly.io/docs/reference/configuration/
app = %q
primary_region = %q

[build]
  dockerfile = "Dockerfile"

[env]
  PORT = "3000"

[http_service]
  internal_port = 3000
  force_https = true
  auto_stop_machines = "stop"
  auto_start_machines = true
  min_machines_running = 0

  [[http_service.checks]]
    grace_period = "10s"
    interval = "30s"
    method = "GET"
    path = "/"
    timeout = "5s"
`

func (f *flyProvider) deploy(ctx context.Context, d *deployment) error {
	fly, err := f.bin()
	if err != nil {
		return err
	}
	if err := ensureDockerfile(d); err != nil {
		return err
	}
	app := f.app(d)
	if _, err := os.Stat(filepath.Join(d.Root, "fly.toml")); errors.Is(err, os.ErrNotExist) {
		region := f.c.Region
		if region == "" {
			region = "iad"
		}
		if err := os.WriteFile(filepath.Join(d.Root, "fly.toml"), fmt.Appendf(nil, flyToml, app, region), 0o644); err != nil {
			return err
		}
		d.ui.Printf("  %s %s\n", d.ui.Green("+"), "Wrote fly.toml")
	}
	if _, err := f.run.run(ctx, d.Root, nil, nil, fly, "status", "--app", app); err != nil {
		args := []string{"apps", "create", app}
		if f.c.Org != "" {
			args = append(args, "--org", f.c.Org)
		}
		if err := f.run.stream(ctx, d.Root, nil, d.stdout, d.stderr, fly, args...); err != nil {
			return err
		}
	}
	if len(d.Env) > 0 {
		// Staged: they take effect with this deploy.
		if err := f.run.stream(ctx, d.Root, strings.NewReader(envLines(d.Env)), d.stdout, d.stderr, fly, "secrets", "import", "--app", app, "--stage"); err != nil {
			return err
		}
	}
	if len(d.Domains) > 0 {
		d.ui.Printf("  %s %s\n", d.ui.Dim("i"), d.ui.Dim("Fly serves HTTPS; add the domains with fly certs add, and point them at the app"))
	}
	return f.run.stream(ctx, d.Root, nil, d.stdout, d.stderr, fly, "deploy", "--app", app, "--remote-only")
}

func (f *flyProvider) releases(ctx context.Context, d *deployment) error {
	fly, err := f.bin()
	if err != nil {
		return err
	}
	return f.run.stream(ctx, d.Root, nil, d.stdout, d.stderr, fly, "releases", "--app", f.app(d))
}

func (f *flyProvider) logs(ctx context.Context, d *deployment) error {
	fly, err := f.bin()
	if err != nil {
		return err
	}
	err = f.run.stream(ctx, d.Root, nil, d.stdout, d.stderr, fly, "logs", "--app", f.app(d))
	if ctx.Err() != nil {
		return nil //nolint:nilerr // stopped with ^C: not a failure
	}
	return err
}

// containerConfig is a target that builds an image and pushes it to a
// registry, for platforms that run containers: Cloud Run, ECS, Azure
// Container Apps, Kubernetes, Railway, Render and the like.
type containerConfig struct {
	// Image is the repository, such as "ghcr.io/you/app". It is tagged
	// with the release's version and with latest.
	Image string `json:"image"`
	// Tool is docker or podman. The default is whichever is installed.
	Tool string `json:"tool"`
	// Platform is the image's platform. The default is linux/amd64.
	Platform string `json:"platform"`
	// NoPush builds the image without pushing it.
	NoPush bool `json:"noPush"`
	// After are commands run once the image is pushed, with {image}
	// replaced by the image's tagged name: the platform's own deploy
	// command, such as "gcloud run deploy app --image {image}".
	After []string `json:"after"`
}

type containerProvider struct {
	c   containerConfig
	run runner
}

var tagRe = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func (c *containerProvider) deploy(ctx context.Context, d *deployment) error {
	tool := c.c.Tool
	if tool == "" {
		for _, t := range []string{"docker", "podman"} {
			if _, err := exec.LookPath(t); err == nil {
				tool = t
				break
			}
		}
		if tool == "" {
			return errors.New("the container provider needs docker or podman")
		}
	}
	if err := ensureDockerfile(d); err != nil {
		return err
	}
	platform := c.c.Platform
	if platform == "" {
		platform = "linux/amd64"
	}
	tag := strings.Trim(tagRe.ReplaceAllString(d.Version, "-"), "-.")
	if len(tag) > 120 {
		tag = tag[:120]
	}
	image := c.c.Image + ":" + tag
	latest := c.c.Image + ":latest"
	if err := c.run.stream(ctx, d.Root, nil, d.stdout, d.stderr, tool, "build", "--platform", platform, "-t", image, "-t", latest, "."); err != nil {
		return err
	}
	if !c.c.NoPush {
		for _, img := range []string{image, latest} {
			if err := c.run.stream(ctx, d.Root, nil, d.stdout, d.stderr, tool, "push", img); err != nil {
				return err
			}
		}
	}
	d.ui.Printf("  %s %s\n", d.ui.Green("✓"), "Image "+image)
	for _, cmd := range c.c.After {
		line := strings.ReplaceAll(cmd, "{image}", image)
		d.ui.Printf("  %s %s\n", d.ui.Dim("$"), line)
		shell, flag := "sh", "-c"
		if runtime.GOOS == "windows" {
			shell, flag = "cmd", "/C"
		}
		if err := c.run.stream(ctx, d.Root, nil, d.stdout, d.stderr, shell, flag, line); err != nil {
			return err
		}
	}
	return nil
}
