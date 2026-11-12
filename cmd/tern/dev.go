package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/Unfield/Tern/internal/codegen"
	"github.com/Unfield/Tern/internal/routes"
	"github.com/Unfield/Tern/internal/ui"
	"github.com/Unfield/Tern/internal/version"
)

// cmdDev runs the development server: Vite for the client, the Go app
// rebuilt on every change, and a proxy in front of both on one port.
func cmdDev(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("dev", flag.ContinueOnError)
	fl.SetOutput(stderr)
	defaultPort := 5173
	if p, err := strconv.Atoi(os.Getenv("PORT")); err == nil {
		defaultPort = p
	}
	port := fl.Int("port", defaultPort, "port to listen on")
	host := fl.String("host", "localhost", "host to listen on; use 0.0.0.0 to expose the server on your network")
	if err := fl.Parse(args); err != nil {
		return 2
	}

	root, err := os.Getwd()
	if err != nil {
		ui.New(stderr).Error(err)
		return 1
	}
	d := &devRunner{root: root, host: *host, port: *port, stdout: stdout, stderr: stderr, ui: ui.New(stdout), clients: map[chan struct{}]bool{}, started: time.Now()}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := d.run(ctx); err != nil {
		ui.New(stderr).Error(err)
		return 1
	}
	return 0
}

type devRunner struct {
	// routes are the route patterns of the last generation, nil before it.
	routes []string
	root   string
	host   string
	port   int
	stdout io.Writer
	stderr io.Writer
	ui     *ui.Printer
	// started is when tern dev began, for the startup time in the banner.
	started time.Time

	vitePort   int
	appPort    int
	viteExited chan struct{}
	viteErr    error
	proxy      *httputil.ReverseProxy

	mu       sync.Mutex
	app      *exec.Cmd
	appDone  chan struct{}
	ready    chan struct{} // closed once the app accepts connections
	problem  string        // build or runtime error shown in the browser
	clients  map[chan struct{}]bool
	logTail  *tailBuffer
	building sync.Mutex
	// logs is the log view at /__tern/dev/logs.
	logs *devLogs
}

func (d *devRunner) run(ctx context.Context) error {
	unlock, err := lockDev(d.root)
	if err != nil {
		return err
	}
	defer unlock()

	if d.vitePort, err = freePort(); err != nil {
		return err
	}
	if d.appPort, err = freePort(); err != nil {
		return err
	}
	target, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(d.appPort))
	d.proxy = httputil.NewSingleHostReverseProxy(target)
	d.proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "The app is restarting; try again in a moment.", http.StatusBadGateway)
	}
	d.ready = make(chan struct{})
	d.logs = newDevLogs()
	stopLogs, err := d.logs.listen()
	if err != nil {
		return err
	}
	defer stopLogs()

	vite, err := d.startVite()
	if err != nil {
		return err
	}
	defer d.stopVite(vite)

	ln, err := net.Listen("tcp", net.JoinHostPort(d.host, strconv.Itoa(d.port)))
	if err != nil {
		return fmt.Errorf("listening on port %d: %w (use -port to pick another)", d.port, err)
	}
	srv := &http.Server{Handler: d, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	d.rebuild(false)
	d.banner()
	defer d.stopApp()
	return d.watch(ctx)
}

// banner prints the startup message with the URLs to open.
func (d *devRunner) banner() {
	p := d.ui
	port := strconv.Itoa(d.port)
	p.Printf("\n  %s %s  %s\n\n", p.Brand(p.Bold("tern")), p.Dim(version.String()), p.Dim("ready in "+ui.Duration(time.Since(d.started))))
	exposed := d.host == "" || d.host == "0.0.0.0" || d.host == "::"
	local := d.host
	if exposed {
		local = "localhost"
	}
	p.Printf("  %s %s %s\n", p.Green("➜"), p.Bold("Local:  "), p.Cyan("http://"+net.JoinHostPort(local, port)+"/"))
	if exposed {
		for _, ip := range lanAddresses() {
			p.Printf("  %s %s %s\n", p.Green("➜"), p.Bold("Network:"), p.Cyan("http://"+net.JoinHostPort(ip, port)+"/"))
		}
	} else {
		p.Printf("  %s %s %s\n", p.Dim("➜"), p.Dim("Network:"), p.Dim("use -host 0.0.0.0 to expose"))
	}
	p.Printf("  %s %s %s\n", p.Green("➜"), p.Bold("Logs:   "), p.Cyan("http://"+net.JoinHostPort(local, port)+"/__tern/dev/logs"))
	if problem := d.currentProblem(); problem != "" {
		p.Println()
		p.Failure("the app does not build yet; the error is shown in the browser too")
	}
	p.Println()
}

// lanAddresses returns this machine's non-loopback IPv4 addresses.
func lanAddresses() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []string
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() && ipn.IP.To4() != nil {
			out = append(out, ipn.IP.String())
		}
	}
	return out
}

func (d *devRunner) startVite() (*exec.Cmd, error) {
	bin, err := viteBin()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, "--clearScreen", "false", "--logLevel", "warn")
	cmd.Env = append(os.Environ(),
		"TERN_VITE_PORT="+strconv.Itoa(d.vitePort),
		"TERN_APP_PORT="+strconv.Itoa(d.port),
	)
	cmd.Stdout, cmd.Stderr = d.stdout, d.stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting vite: %w", err)
	}
	// Wait until Vite accepts connections, so the first page can be
	// server-rendered through it.
	exited := make(chan struct{})
	go func() {
		d.viteErr = cmd.Wait()
		close(exited)
	}()
	deadline := time.Now().Add(60 * time.Second)
	for {
		select {
		case <-exited:
			return nil, fmt.Errorf("vite exited during startup: %w", d.viteErr)
		default:
		}
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(d.vitePort), 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			stopProcess(cmd)
			return nil, errors.New("vite did not start within 60 seconds")
		}
		time.Sleep(25 * time.Millisecond)
	}
	d.viteExited = exited
	return cmd, nil
}

// ServeHTTP serves dev endpoints and proxies everything else to the app,
// holding requests while it rebuilds.
func (d *devRunner) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/__tern/dev/logs" || strings.HasPrefix(r.URL.Path, "/__tern/dev/logs/") {
		d.logs.serve(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/__tern/dev/api/") {
		d.serveAPI(w, r)
		return
	}
	switch r.URL.Path {
	case "/__tern/dev/events":
		d.events(w, r)
		return
	case "/__tern/dev/client.js":
		w.Header().Set("Content-Type", "text/javascript")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, devClientJS)
		_, _ = io.WriteString(w, devOverlayJS)
		return
	}

	d.mu.Lock()
	ready, problem := d.ready, d.problem
	d.mu.Unlock()
	if problem == "" {
		select {
		case <-ready:
		case <-time.After(60 * time.Second):
			http.Error(w, "The app did not start in time.", http.StatusServiceUnavailable)
			return
		case <-r.Context().Done():
			return
		}
		d.mu.Lock()
		problem = d.problem
		d.mu.Unlock()
	}
	if problem != "" {
		d.showProblem(w, r, problem)
		return
	}
	d.logs.proxy(d.proxy, w, r)
}

const devClientJS = `const events = new EventSource('/__tern/dev/events');
events.addEventListener('reload', () => location.reload());
`

func (d *devRunner) showProblem(w http.ResponseWriter, r *http.Request, problem string) {
	if !strings.Contains(r.Header.Get("Accept"), "text/html") {
		http.Error(w, problem, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Build error · Tern</title>
<style>
body { margin: 0; font: 15px/1.5 system-ui, sans-serif; background: #1c1917; color: #f5f5f4; }
main { max-width: 64rem; margin: 3rem auto; padding: 0 1.5rem; }
h1 { color: #fca5a5; font-size: 1.25rem; }
pre { background: #0c0a09; padding: 1rem 1.25rem; border-radius: 8px; overflow-x: auto; white-space: pre-wrap; font: 13px/1.6 ui-monospace, monospace; }
p { color: #a8a29e; }
</style></head>
<body data-tern-problem><main>
<h1>The app could not be built or started</h1>
<pre>%s</pre>
<p>Fix the problem and save. This page reloads by itself.</p>
</main>
<script type="module" src="/__tern/dev/client.js"></script>
</body></html>`, html.EscapeString(problem))
}

// events streams reload notifications to the browser.
func (d *devRunner) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch := make(chan struct{}, 1)
	d.mu.Lock()
	d.clients[ch] = true
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		delete(d.clients, ch)
		d.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()
	for {
		select {
		case <-ch:
			_, _ = io.WriteString(w, "event: reload\ndata: {}\n\n")
			flusher.Flush()
		case <-time.After(30 * time.Second):
			_, _ = io.WriteString(w, ": ping\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (d *devRunner) broadcastReload() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for ch := range d.clients {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (d *devRunner) setProblem(p string) {
	d.mu.Lock()
	d.problem = p
	select {
	case <-d.ready:
	default:
		close(d.ready) // release waiting requests so they see the problem
	}
	d.mu.Unlock()
	first, _, _ := strings.Cut(strings.TrimSpace(p), "\n")
	d.logs.event("build failed: " + first)
	d.ui.Event(d.ui.Red("✗"), "%s", d.ui.Red("build failed"))
	for _, line := range strings.Split(strings.TrimRight(p, "\n"), "\n") {
		d.ui.Printf("    %s\n", line)
	}
	d.broadcastReload()
}

// rebuild regenerates code, rebuilds the Go binary and restarts the app.
func (d *devRunner) rebuild(reload bool) {
	d.building.Lock()
	defer d.building.Unlock()
	began := time.Now()

	d.mu.Lock()
	select {
	case <-d.ready:
		d.ready = make(chan struct{})
	default:
	}
	d.mu.Unlock()
	d.stopApp()

	res, err := codegen.Generate(d.root)
	if err != nil {
		d.setProblem(err.Error())
		return
	}
	d.noteRoutes(res)
	bin := filepath.Join(d.root, codegen.GenDir, "dev", "app")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	var out bytes.Buffer
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = d.root
	build.Stdout, build.Stderr = &out, &out
	if err := build.Run(); err != nil {
		d.setProblem(strings.TrimSpace(out.String()))
		return
	}
	if err := d.startApp(bin); err != nil {
		d.setProblem(err.Error())
		return
	}

	d.mu.Lock()
	d.problem = ""
	select {
	case <-d.ready:
	default:
		close(d.ready)
	}
	d.mu.Unlock()
	if reload {
		d.logs.event("server restarted in " + ui.Duration(time.Since(began)))
		d.ui.Event(d.ui.Green("↻"), "server restarted %s", d.ui.Dim("in "+ui.Duration(time.Since(began))))
		d.broadcastReload()
	}
}

func (d *devRunner) startApp(bin string) error {
	d.logTail = &tailBuffer{max: 8 << 10}
	cmd := exec.Command(bin)
	cmd.Dir = d.root
	file, err := readDotEnv(filepath.Join(d.root, ".env"))
	if err != nil {
		d.ui.Event(d.ui.Yellow("!"), ".env: %v", err)
	}
	cmd.Env = append(append(file, os.Environ()...), // set variables win over .env
		"HOST=127.0.0.1",
		"PORT="+strconv.Itoa(d.appPort),
		"TERN_DEV_VITE_URL=http://127.0.0.1:"+strconv.Itoa(d.vitePort),
		"TERN_ROOT="+d.root,
	)
	cmd.Env = append(cmd.Env, d.logs.env()...)
	cmd.Stdout = d.stdout
	cmd.Stderr = io.MultiWriter(d.stderr, d.logTail)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting the app: %w", err)
	}
	done := make(chan struct{})
	go func() {
		err := cmd.Wait()
		close(done)
		d.mu.Lock()
		stillCurrent := d.app == cmd
		d.mu.Unlock()
		if stillCurrent && err != nil {
			d.setProblem(fmt.Sprintf("The app exited: %v\n\n%s", err, d.logTail.String()))
		}
	}()

	d.mu.Lock()
	d.app, d.appDone = cmd, done
	d.mu.Unlock()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-done:
			return fmt.Errorf("the app exited during startup\n\n%s", d.logTail.String())
		default:
		}
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(d.appPort), 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return errors.New("the app did not start listening within 30 seconds; does main.go call ListenAndServe?")
}

func (d *devRunner) stopApp() {
	d.mu.Lock()
	cmd, done := d.app, d.appDone
	d.app = nil
	d.mu.Unlock()
	if cmd == nil {
		return
	}
	if runtime.GOOS == "windows" {
		_ = cmd.Process.Kill()
	} else {
		_ = cmd.Process.Signal(os.Interrupt)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
}

// watch reacts to file changes until ctx is canceled.
func (d *devRunner) watch(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer func() { _ = w.Close() }()
	if err := addTree(w, d.root); err != nil {
		return err
	}

	var timer *time.Timer
	var pendingBuild, pendingGen bool
	fire := make(chan struct{}, 1)
	for {
		select {
		case <-ctx.Done():
			d.ui.Printf("\n  %s\n", d.ui.Dim("stopping…"))
			return nil
		case err := <-w.Errors:
			d.ui.Event(d.ui.Yellow("!"), "file watcher: %v", err)
		case ev := <-w.Events:
			rel, err := filepath.Rel(d.root, ev.Name)
			if err != nil || ignored(rel) {
				continue
			}
			if ev.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					_ = addTree(w, ev.Name)
				}
			}
			kind := classify(filepath.ToSlash(rel), ev.Op)
			if kind == changeNone && strings.HasSuffix(rel, ".svelte") && hasLiveGoScript(ev.Name) {
				kind = changeGen // a live component's Go code is part of the server; gen reports whether it changed
			}
			switch kind {
			case changeBuild:
				pendingBuild = true
			case changeGen:
				pendingGen = true
			default:
				continue
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(100*time.Millisecond, func() {
				select {
				case fire <- struct{}{}:
				default:
				}
			})
		case <-fire:
			build, gen := pendingBuild, pendingGen
			pendingBuild, pendingGen = false, false
			switch {
			case build:
				d.rebuild(true)
			case gen:
				res, err := codegen.Generate(d.root)
				if err == nil {
					d.noteRoutes(res)
				}
				switch {
				case err != nil:
					d.setProblem(err.Error())
				case res.GoChanged:
					d.rebuild(true)
				case d.currentProblem() != "":
					d.rebuild(true) // a route fix may resolve the shown problem
				}
			}
		}
	}
}

// noteRoutes says which routes a generation added and removed.
func (d *devRunner) noteRoutes(res *codegen.Result) {
	if res == nil || res.Manifest == nil {
		return
	}
	now := make([]string, 0, len(res.Manifest.Routes))
	for _, r := range res.Manifest.Routes {
		now = append(now, r.Pattern())
	}
	if d.routes != nil {
		added, removed := diffRoutes(d.routes, now)
		for _, p := range added {
			d.ui.Event(d.ui.Green("+"), "route %s", p)
		}
		for _, p := range removed {
			d.ui.Event(d.ui.Red("-"), "route %s", p)
		}
	}
	d.routes = now
}

// diffRoutes returns the patterns in now but not in old, and the other
// way around, each sorted.
func diffRoutes(old, now []string) (added, removed []string) {
	in := func(list []string) map[string]bool {
		m := make(map[string]bool, len(list))
		for _, p := range list {
			m[p] = true
		}
		return m
	}
	o, n := in(old), in(now)
	for p := range n {
		if !o[p] {
			added = append(added, p)
		}
	}
	for p := range o {
		if !n[p] {
			removed = append(removed, p)
		}
	}
	slices.Sort(added)
	slices.Sort(removed)
	return added, removed
}

func (d *devRunner) currentProblem() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.problem
}

type change int

const (
	changeNone  change = iota
	changeGen          // route structure changed: regenerate
	changeBuild        // Go code changed: regenerate and rebuild
)

// classify decides what a file change requires. Svelte, CSS and other
// client files are handled by Vite's hot module replacement.
func classify(rel string, op fsnotify.Op) change {
	base := filepath.Base(rel)
	switch {
	case base == codegen.GoFile:
		return changeNone
	case strings.HasSuffix(base, ".go") && !strings.HasSuffix(base, "_test.go"), base == "go.mod", base == "go.sum":
		return changeBuild
	case rel == codegen.AppHTML:
		return changeNone // re-read on every request
	case strings.HasPrefix(rel, codegen.RoutesDir+"/") && base == routes.FilePageMarkdown:
		return changeGen // regenerates the page's component, which Vite reloads
	case strings.HasPrefix(rel, codegen.RoutesDir+"/"):
		structural := op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0
		if structural && (strings.HasPrefix(base, "+") || filepath.Ext(base) == "") {
			return changeGen
		}
	}
	return changeNone
}

// hasLiveGoScript reports whether a .svelte file has <script lang="go" live>.
func hasLiveGoScript(file string) bool {
	raw, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	s := string(raw)
	i := strings.Index(s, `lang="go"`)
	if i < 0 {
		return false
	}
	end := strings.Index(s[i:], ">")
	return end > 0 && strings.Contains(s[i:i+end], "live")
}

func ignored(rel string) bool {
	first, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
	switch first {
	case "node_modules", codegen.GenDir, ".git", "build", "static":
		return true
	}
	return strings.HasPrefix(filepath.Base(rel), ".") || strings.HasSuffix(rel, "~")
}

func addTree(w *fsnotify.Watcher, dir string) error {
	root := dir
	return filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil //nolint:nilerr // unreadable entries are skipped
		}
		if rel, _ := filepath.Rel(root, p); rel != "." && ignored(rel) {
			return filepath.SkipDir
		}
		return w.Add(p)
	})
}

// stopVite stops the Vite process, whose Wait already runs in startVite.
func (d *devRunner) stopVite(cmd *exec.Cmd) {
	if runtime.GOOS == "windows" {
		_ = cmd.Process.Kill()
	} else {
		_ = cmd.Process.Signal(os.Interrupt)
	}
	select {
	case <-d.viteExited:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		<-d.viteExited
	}
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func stopProcess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if runtime.GOOS == "windows" {
		_ = cmd.Process.Kill()
		return
	}
	_ = cmd.Process.Signal(os.Interrupt)
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
	}
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
