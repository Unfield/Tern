package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// The ssh provider deploys to a Linux server with systemd, over the
// system's ssh, so ~/.ssh/config, keys and agents work as they do in a
// terminal. On the server:
//
//	/srv/<name>/releases/<id>/app   each release's binary
//	/srv/<name>/releases/<id>/.env  and its environment, from tern.json
//	/srv/<name>/shared              the working directory: databases and
//	                                certificates, kept across releases
//	/srv/<name>/shared/.env         variables of your own, if you add it
//	/srv/<name>/current             → the running release
//	/etc/systemd/system/<name>@.service
//
// A deploy starts <name>@<new id> next to the running release. Both listen
// on the same port (SO_REUSEPORT), and systemd waits for the new one to say
// it's ready (sd_notify) before the old one is stopped, letting its
// requests finish: no request is refused. A release that doesn't start is
// stopped, and the old one keeps serving.

type sshConfig struct {
	// Host is where to connect, as ssh takes it: "root@203.0.113.5" or a
	// Host from ~/.ssh/config.
	Host string `json:"host"`
	// Port and Identity are ssh's -p and -i; SSHArgs are more arguments.
	Port     int      `json:"port"`
	Identity string   `json:"identity"`
	SSHArgs  []string `json:"sshArgs"`
	// Dir is where the app lives on the server. The default is /srv/<name>.
	Dir string `json:"dir"`
	// User runs the app. It is created if need be. The default is the
	// app's name.
	User string `json:"user"`
	// AppPort is the port the app listens on without domains. The default
	// is 3000; with domains, it is 443 and 80.
	AppPort int `json:"appPort"`
	// Keep is how many releases are kept for rollbacks. The default is 5.
	Keep int `json:"keep"`
}

type sshProvider struct {
	c   sshConfig
	run runner
	// build makes the binary; tests replace it.
	build func(d *deployment, goos, goarch string) (string, error)
}

var safeName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func (s *sshProvider) settings(d *deployment) (dir, user string, err error) {
	if !safeName.MatchString(d.Name) {
		return "", "", fmt.Errorf("the app's name %q must be lowercase letters, digits, dots and dashes; set \"name\" in the target", d.Name)
	}
	dir, user = s.c.Dir, s.c.User
	if dir == "" {
		dir = "/srv/" + d.Name
	}
	if user == "" {
		user = d.Name
	}
	if !strings.HasPrefix(dir, "/") || strings.ContainsAny(dir, "'\"\n ") {
		return "", "", fmt.Errorf("dir %q must be an absolute path without spaces or quotes", dir)
	}
	if !safeName.MatchString(user) {
		return "", "", fmt.Errorf("user %q must be lowercase letters, digits, dots and dashes", user)
	}
	return dir, user, nil
}

func (s *sshProvider) sshArgs() []string {
	args := []string{"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-o", "ServerAliveInterval=15"}
	if runtime.GOOS != "windows" {
		// One connection for the whole deploy.
		args = append(args, "-o", "ControlMaster=auto", "-o", "ControlPath=/tmp/tern-ssh-%C", "-o", "ControlPersist=60")
	}
	if s.c.Port != 0 {
		args = append(args, "-p", strconv.Itoa(s.c.Port))
	}
	if s.c.Identity != "" {
		args = append(args, "-i", expandHome(s.c.Identity))
	}
	return append(args, s.c.SSHArgs...)
}

func expandHome(p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, rest)
		}
	}
	return p
}

// sh runs script on the server with sh, with stdin.
func (s *sshProvider) sh(ctx context.Context, d *deployment, script string, stdin io.Reader) ([]byte, error) {
	args := append(s.sshArgs(), s.c.Host, "sh -c "+shQuote(script))
	return s.run.run(ctx, d.Root, stdin, nil, "ssh", args...)
}

// shQuote quotes s for a POSIX shell.
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// server is what a probe found.
type server struct {
	arch string // GOARCH
	sudo string // "sudo -n " unless root
}

func (s *sshProvider) probe(ctx context.Context, d *deployment) (server, error) {
	out, err := s.sh(ctx, d, `uname -m; id -u; command -v systemctl >/dev/null 2>&1 && echo systemd || echo no-systemd`, nil)
	if err != nil {
		return server{}, fmt.Errorf("connecting to %s: %w", s.c.Host, err)
	}
	f := strings.Fields(string(out))
	if len(f) < 3 {
		return server{}, fmt.Errorf("unexpected answer from %s: %q", s.c.Host, out)
	}
	var srv server
	switch f[0] {
	case "x86_64", "amd64":
		srv.arch = "amd64"
	case "aarch64", "arm64":
		srv.arch = "arm64"
	case "armv7l", "armv6l":
		srv.arch = "arm"
	case "riscv64":
		srv.arch = "riscv64"
	default:
		return server{}, fmt.Errorf("%s runs on %s, which tern deploy doesn't build for", s.c.Host, f[0])
	}
	if f[1] != "0" {
		srv.sudo = "sudo -n "
	}
	if f[2] != "systemd" {
		return server{}, fmt.Errorf("%s has no systemd, which the ssh provider runs the app with", s.c.Host)
	}
	return srv, nil
}

// unit is the systemd template unit, an instance per release.
func unit(name, dir, user string) string {
	return `[Unit]
Description=` + name + ` (Tern app), release %i
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
NotifyAccess=main
User=` + user + `
WorkingDirectory=` + dir + `/shared
EnvironmentFile=-` + dir + `/shared/.env
EnvironmentFile=` + dir + `/releases/%i/.env
Environment=TERN_REUSEPORT=1
ExecStart=` + dir + `/releases/%i/app
Restart=on-failure
RestartSec=2
TimeoutStartSec=180
TimeoutStopSec=30
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=` + dir + `/shared

[Install]
WantedBy=multi-user.target
`
}

func (s *sshProvider) deploy(ctx context.Context, d *deployment) error {
	p := d.ui
	dir, user, err := s.settings(d)
	if err != nil {
		return err
	}
	srv, err := s.probe(ctx, d)
	if err != nil {
		return err
	}
	p.Printf("  %s %s %s\n", p.Green("✓"), "Connected to "+s.c.Host, p.Dim("linux/"+srv.arch))

	build := s.build
	if build == nil {
		build = buildFor
	}
	bin, err := build(d, "linux", srv.arch)
	if err != nil {
		return err
	}

	id := releaseID(time.Now(), d.Version)
	S := srv.sudo
	setup := fmt.Sprintf(`set -e
if ! id -u %[2]s >/dev/null 2>&1; then
	if command -v useradd >/dev/null 2>&1; then %[1]suseradd --system --home-dir %[3]s --no-create-home --shell /usr/sbin/nologin %[2]s
	else %[1]sadduser -S -D -H -h %[3]s %[2]s; fi
fi
%[1]smkdir -p %[3]s/releases %[3]s/shared
%[1]schown %[2]s: %[3]s/shared
%[1]schmod 750 %[3]s/shared
printf '%%s' %[4]s | %[1]stee /etc/systemd/system/%[5]s@.service >/dev/null
%[1]ssystemctl daemon-reload
`, S, user, dir, shQuote(unit(d.Name, dir, user)), d.Name)
	if _, err := s.sh(ctx, d, setup, nil); err != nil {
		return sudoHint(err, srv)
	}

	env := map[string]string{}
	for k, v := range d.Env {
		env[k] = v
	}
	if len(d.Domains) > 0 {
		env["TERN_TLS_DOMAINS"] = strings.Join(d.Domains, ",")
	} else if _, ok := env["PORT"]; !ok {
		port := s.c.AppPort
		if port == 0 {
			port = 3000
		}
		env["PORT"] = strconv.Itoa(port)
	}
	f, err := os.Open(bin)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, _ := f.Stat()
	pr, pw := io.Pipe()
	go func() {
		zw, _ := gzip.NewWriterLevel(pw, gzip.BestSpeed)
		_, err := io.Copy(zw, f)
		if err == nil {
			err = zw.Close()
		}
		_ = pw.CloseWithError(err)
	}()
	upload := fmt.Sprintf(`set -e
%[1]smkdir -p %[2]s/releases/%[3]s
gzip -dc | %[1]stee %[2]s/releases/%[3]s/app.new >/dev/null
%[1]schmod 755 %[2]s/releases/%[3]s/app.new
%[1]smv %[2]s/releases/%[3]s/app.new %[2]s/releases/%[3]s/app
`, S, dir, id)
	began := time.Now()
	if _, err := s.sh(ctx, d, upload, pr); err != nil {
		return err
	}
	// The release's environment is part of it: a release that fails to
	// start, or a restart of the running one, never sees another's.
	envScript := fmt.Sprintf(`set -e
%[1]stee %[2]s/releases/%[3]s/.env >/dev/null
%[1]schmod 600 %[2]s/releases/%[3]s/.env
`, S, dir, id)
	if _, err := s.sh(ctx, d, envScript, strings.NewReader(envLines(env))); err != nil {
		return err
	}
	p.Printf("  %s %s %s\n", p.Green("✓"), "Uploaded release "+id, p.Dim(sizeOf(info)+" in "+time.Since(began).Round(100*time.Millisecond).String()))

	if err := s.switchTo(ctx, d, srv, dir, id, true); err != nil {
		return err
	}
	if len(d.Domains) > 0 {
		for _, dom := range d.Domains {
			p.Printf("    %s\n", p.Cyan("https://"+dom+"/"))
		}
	} else {
		host := s.c.Host
		if i := strings.LastIndexByte(host, '@'); i >= 0 {
			host = host[i+1:]
		}
		p.Printf("    %s\n", p.Cyan("http://"+host+":"+env["PORT"]+"/"))
	}
	return nil
}

func sizeOf(info os.FileInfo) string {
	if info == nil {
		return ""
	}
	return strconv.FormatFloat(float64(info.Size())/(1<<20), 'f', 1, 64) + " MB"
}

func sudoHint(err error, srv server) error {
	if srv.sudo != "" && strings.Contains(err.Error(), "sudo") {
		return fmt.Errorf("%w\n\nthe ssh provider needs root, or sudo without a password, to install the service", err)
	}
	return err
}

// errNotStarted is the exit status of the switch script when the new
// release didn't start.
const errNotStarted = "exit status 3"

// switchTo starts release id, and once it is ready stops the others. A new
// release that doesn't start is removed when discard is set.
func (s *sshProvider) switchTo(ctx context.Context, d *deployment, srv server, dir, id string, discard bool) error {
	p := d.ui
	keep := s.c.Keep
	if keep <= 0 {
		keep = 5
	}
	script := fmt.Sprintf(`set -e
# The running releases. Listing them can fail without root (no access to
# the system bus): then stop, rather than leave the old release running.
units=$(%[1]ssystemctl list-units --type=service --state=active --plain --no-legend '%[2]s@*') || { echo "tern: can't list the running releases" >&2; exit 4; }
old=$(printf '%%s\n' "$units" | awk 'NF {print $1}')
if ! %[1]ssystemctl start %[2]s@%[4]s; then
	echo "tern: release %[4]s didn't start; the running release keeps serving" >&2
	%[1]sjournalctl -u %[2]s@%[4]s -n 40 --no-pager -o cat >&2 || true
	%[1]ssystemctl stop %[2]s@%[4]s >/dev/null 2>&1 || true
	%[6]s
	exit 3
fi
%[1]ssystemctl enable %[2]s@%[4]s >/dev/null 2>&1
for u in $old; do
	[ "$u" = "%[2]s@%[4]s.service" ] && continue
	%[1]ssystemctl stop "$u"
	%[1]ssystemctl disable "$u" >/dev/null 2>&1 || true
	echo "stopped $u"
done
%[1]sln -sfn releases/%[4]s %[3]s/current
cd %[3]s/releases
for r in $(ls -1 | sort -r | tail -n +%[5]d); do
	[ "$r" = "%[4]s" ] && continue
	%[1]ssystemctl is-active --quiet "%[2]s@$r" && continue
	%[1]srm -rf "%[3]s/releases/$r"
done
`, srv.sudo, d.Name, dir, id, keep+1, discardCmd(srv, dir, id, discard))
	s0 := p.Spin("Starting release " + id)
	out, err := s.sh(ctx, d, script, nil)
	if err != nil {
		s0.Fail()
		if msg, ok := strings.CutPrefix(err.Error(), "ssh: "+errNotStarted+"\n"); ok {
			// The script printed the release's log: show it under the error.
			lines := strings.Split(strings.TrimSpace(msg), "\n")
			var log []string
			for _, l := range lines {
				if !strings.HasPrefix(l, "tern: ") && !strings.HasPrefix(l, "Job for ") && !strings.HasPrefix(l, "See \"systemctl") {
					log = append(log, "  "+l)
				}
			}
			kept := "the running release keeps serving"
			if discard {
				kept += "; the new one was removed"
			}
			return fmt.Errorf("release %s didn't start, so %s. Its log:\n%s", id, kept, strings.Join(log, "\n"))
		}
		return err
	}
	stopped := 0
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(l, "stopped ") {
			stopped++
		}
	}
	if stopped > 0 {
		s0.Done("ready; the old release stopped after its requests")
	} else {
		s0.Done("ready")
	}
	return nil
}

func discardCmd(srv server, dir, id string, discard bool) string {
	if !discard {
		return ":"
	}
	return srv.sudo + "rm -rf " + dir + "/releases/" + id
}

// releaseID names a release by its time, then its version, so names sort
// by time.
func releaseID(t time.Time, version string) string {
	v := regexp.MustCompile(`[^A-Za-z0-9._-]+`).ReplaceAllString(version, "-")
	if len(v) > 40 {
		v = v[:40]
	}
	id := t.UTC().Format("20060102-150405")
	if v != "" {
		id += "-" + strings.Trim(v, "-.")
	}
	return id
}

// releaseList reads the releases on the server, oldest first, and the
// current one.
func (s *sshProvider) releaseList(ctx context.Context, d *deployment, dir string) (list []string, current string, err error) {
	out, err := s.sh(ctx, d, fmt.Sprintf(`ls -1 %[1]s/releases 2>/dev/null | sort; echo "--"; readlink %[1]s/current || true`, dir), nil)
	if err != nil {
		return nil, "", err
	}
	before, after, _ := bytes.Cut(out, []byte("--\n"))
	list = strings.Fields(string(before))
	current = strings.TrimPrefix(strings.TrimSpace(string(after)), "releases/")
	return list, current, nil
}

func (s *sshProvider) rollback(ctx context.Context, d *deployment) error {
	dir, _, err := s.settings(d)
	if err != nil {
		return err
	}
	srv, err := s.probe(ctx, d)
	if err != nil {
		return err
	}
	list, current, err := s.releaseList(ctx, d, dir)
	if err != nil {
		return err
	}
	i := -1
	for j, r := range list {
		if r == current {
			i = j
		}
	}
	if i <= 0 {
		return errors.New("there is no release before the current one to go back to")
	}
	d.ui.Printf("\n  %s %s → %s\n\n", d.ui.Brand(d.ui.Bold("tern deploy")), d.ui.Dim(current), d.ui.Bold(list[i-1]))
	return s.switchTo(ctx, d, srv, dir, list[i-1], false)
}

func (s *sshProvider) releases(ctx context.Context, d *deployment) error {
	dir, _, err := s.settings(d)
	if err != nil {
		return err
	}
	list, current, err := s.releaseList(ctx, d, dir)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		d.ui.Println("  no releases yet")
		return nil
	}
	for i := len(list) - 1; i >= 0; i-- {
		if list[i] == current {
			d.ui.Printf("  %s %s %s\n", d.ui.Green("●"), d.ui.Bold(list[i]), d.ui.Dim("current"))
		} else {
			d.ui.Printf("  %s %s\n", d.ui.Dim("○"), list[i])
		}
	}
	return nil
}

func (s *sshProvider) logs(ctx context.Context, d *deployment) error {
	if _, _, err := s.settings(d); err != nil {
		return err
	}
	// Reading the system journal needs root, or sudo.
	cmd := `S=; [ "$(id -u)" = 0 ] || S="sudo -n"; $S journalctl -u ` + shQuote(d.Name+"@*") + ` -n 100 -f --no-pager -o short-iso`
	args := append(s.sshArgs(), s.c.Host, "sh -c "+shQuote(cmd))
	err := s.run.stream(ctx, d.Root, nil, d.stdout, d.stderr, "ssh", args...)
	if ctx.Err() != nil {
		return nil //nolint:nilerr // stopped with ^C: not a failure
	}
	return err
}

// buildFor runs tern build for goos/goarch into build/deploy.
func buildFor(d *deployment, goos, goarch string) (string, error) {
	out := filepath.Join("build", "deploy", d.Name+"-"+goos+"-"+goarch)
	restore := setEnv(map[string]string{"GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "0"})
	defer restore()
	buildRunHint = false
	defer func() { buildRunHint = true }()
	if code := cmdBuild([]string{"-o", out}, d.stdout, d.stderr); code != 0 {
		return "", errors.New("the build failed")
	}
	return filepath.Join(d.Root, out), nil
}

// setEnv sets variables for this process until restore is called.
func setEnv(vars map[string]string) (restore func()) {
	type old struct {
		v  string
		ok bool
	}
	prev := map[string]old{}
	for k, v := range vars {
		ov, ok := os.LookupEnv(k)
		prev[k] = old{ov, ok}
		_ = os.Setenv(k, v)
	}
	return func() {
		for k, o := range prev {
			if o.ok {
				_ = os.Setenv(k, o.v)
			} else {
				_ = os.Unsetenv(k)
			}
		}
	}
}
