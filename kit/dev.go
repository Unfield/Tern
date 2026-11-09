package kit

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// devServer is active when the app runs under `tern dev`. Requests for
// modules, assets and hot reloading are proxied to Vite, so the whole app
// is served from one port.
type devServer struct {
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
	return &devServer{proxy: proxy, root: root, entry: entry, base: base}, nil
}

func (d *devServer) handles(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return true // Vite's HMR connection
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
