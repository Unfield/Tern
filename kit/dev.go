package kit

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// devServer is active when the app runs under `tern dev`. Requests for
// modules, assets and hot reloading are proxied to Vite, so the whole app
// is served from one port.
type devServer struct {
	vite  *url.URL
	proxy *httputil.ReverseProxy
	root  string
	entry string
	base  string
}

// vitePrefixes are request paths served by the Vite dev server.
var vitePrefixes = []string{"/@", "/node_modules/", "/src/", "/.tern/", "/__vite"}

func newDevServer(viteURL, root, entry, base string) (*devServer, error) {
	u, err := url.Parse(viteURL)
	if err != nil {
		return nil, fmt.Errorf("kit: invalid TERN_DEV_VITE_URL: %w", err)
	}
	if root == "" {
		if root, err = os.Getwd(); err != nil {
			return nil, err
		}
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "Vite dev server unavailable: "+err.Error(), http.StatusBadGateway)
	}
	return &devServer{vite: u, proxy: proxy, root: root, entry: entry, base: base}, nil
}

var devStylesClient = &http.Client{Timeout: 10 * time.Second}

// styles returns the CSS the page's components import, from Vite. Under
// Vite, stylesheets arrive as JavaScript after the page: without them in
// the HTML, the first paint is unstyled. The client removes this copy
// once Vite's own styles are in place, so edits still hot-reload.
func (d *devServer) styles(ctx context.Context, components []string) string {
	if len(components) == 0 {
		return ""
	}
	q := url.Values{"c": components}
	u := d.vite.JoinPath("/__tern/dev/styles")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ""
	}
	resp, err := devStylesClient.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return ""
	}
	// Nothing in a stylesheet can end the <style> element early.
	return strings.ReplaceAll(string(b), "</style", `<\/style`)
}

func (d *devServer) handles(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		// Vite's HMR connection uses the vite-hmr subprotocol; other
		// WebSockets are the app's own.
		return strings.Contains(r.Header.Get("Sec-WebSocket-Protocol"), "vite-") // vite-hmr, vite-ping
	}
	path := r.URL.Path
	if d.base != "" {
		if !strings.HasPrefix(path, d.base+"/") {
			return false
		}
		path = strings.TrimPrefix(path, d.base)
	}
	for _, p := range vitePrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	// Files in static/ are served by Vite as its public directory.
	clean := filepath.FromSlash(strings.TrimPrefix(path, "/"))
	if clean == "" || strings.Contains(clean, "..") {
		return false
	}
	info, err := os.Stat(filepath.Join(d.root, "static", clean))
	return err == nil && !info.IsDir()
}

// shell re-reads src/app.html so edits show up on reload.
func (d *devServer) shell() (*shell, error) {
	b, err := os.ReadFile(filepath.Join(d.root, "src", "app.html"))
	if err != nil {
		return nil, fmt.Errorf("kit: reading src/app.html: %w", err)
	}
	return parseShell(string(b))
}

func (d *devServer) scripts(base string) []string {
	// /__tern/dev/client.js is served by `tern dev` and reloads the page
	// after the Go server restarts.
	return []string{base + "/@vite/client", "/__tern/dev/client.js", base + "/" + d.entry}
}
