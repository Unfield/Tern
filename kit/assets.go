package kit

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
)

// immutablePrefix is where Vite writes content-hashed files.
const immutablePrefix = "/_app/immutable/"

// assets serves the built client and resolves the files a page needs.
type assets struct {
	fsys  fs.FS
	files map[string]assetFile // by URL path
	vite  map[string]viteChunk
	entry string
	// csp is the policy prerendered pages are sent, set by the app; the
	// header is cspName.
	csp, cspName string
}

type assetFile struct {
	name string // path in fsys
	etag string
	gzip string // path of a precompressed .gz sibling, or ""
	br   string // path of a precompressed .br sibling, or ""
	// page is set for a page Tern prerendered, whose inline scripts are
	// allowed by their hashes.
	page   bool
	hashes []string // 'sha256-…' of each inline script that runs
}

// ternPage marks the HTML of a page Tern rendered: its data script.
const ternPage = `id="__tern_data"`

// inlineScript matches a <script> element with its attributes and text.
var inlineScript = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script>`)

// scriptSrc matches a src attribute.
var scriptSrc = regexp.MustCompile(`(?i)\bsrc\s*=`)

// scriptHashes returns the CSP hash of each inline script in page that
// runs. A prerendered page has no nonce: the request it was rendered for is
// gone, so its scripts are allowed by what they say.
func scriptHashes(page []byte) []string {
	var out []string
	for _, m := range inlineScript.FindAllSubmatch(page, -1) {
		attrs := strings.ToLower(string(m[1]))
		if scriptSrc.MatchString(attrs) || !executable(attrs) {
			continue
		}
		sum := sha256.Sum256(m[2])
		h := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
		if !slices.Contains(out, h) {
			out = append(out, h)
		}
	}
	return out
}

// pageCSP is the policy a prerendered page is sent: the app's, with its
// scripts' hashes.
func (a *assets) pageCSP(f assetFile) string {
	if len(f.hashes) == 0 {
		return a.csp
	}
	return addSources(a.csp, "script-src", f.hashes)
}

// viteChunk is an entry in Vite's .vite/manifest.json.
type viteChunk struct {
	File    string   `json:"file"`
	Src     string   `json:"src"`
	IsEntry bool     `json:"isEntry"`
	Imports []string `json:"imports"`
	CSS     []string `json:"css"`
}

func loadAssets(fsys fs.FS, entry string) (*assets, error) {
	a := &assets{fsys: fsys, files: map[string]assetFile{}, entry: entry}

	raw, err := fs.ReadFile(fsys, ".vite/manifest.json")
	if err != nil {
		return nil, fmt.Errorf("kit: reading the client build manifest (run `tern build`): %w", err)
	}
	if err := json.Unmarshal(raw, &a.vite); err != nil {
		return nil, fmt.Errorf("kit: parsing .vite/manifest.json: %w", err)
	}
	if _, ok := a.vite[entry]; !ok {
		return nil, fmt.Errorf("kit: client entry %q is missing from .vite/manifest.json", entry)
	}

	err = fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name == ".vite" {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(name, ".gz") || strings.HasSuffix(name, ".br") {
			return nil
		}
		f := assetFile{name: name}
		if !strings.HasPrefix("/"+name, immutablePrefix) {
			sum, err := hashFile(fsys, name)
			if err != nil {
				return err
			}
			f.etag = `"` + sum + `"`
		}
		if _, err := fs.Stat(fsys, name+".gz"); err == nil {
			f.gzip = name + ".gz"
		}
		if _, err := fs.Stat(fsys, name+".br"); err == nil {
			f.br = name + ".br"
		}
		if strings.HasSuffix(name, ".html") {
			raw, err := fs.ReadFile(fsys, name)
			if err != nil {
				return err
			}
			if bytes.Contains(raw, []byte(ternPage)) {
				f.page, f.hashes = true, scriptHashes(raw)
			}
		}
		a.files["/"+name] = f
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("kit: indexing client assets: %w", err)
	}
	return a, nil
}

func hashFile(fsys fs.FS, name string) (string, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:20], nil
}

// serve writes a static asset and reports whether the path was one.
// Prerendered pages are stored as <path>/index.html.
func (a *assets) serve(w http.ResponseWriter, r *http.Request) bool {
	f, ok := a.files[r.URL.Path]
	if !ok && r.URL.RawQuery == "" {
		f, ok = a.files[strings.TrimSuffix(r.URL.Path, "/")+"/index.html"]
	}
	if !ok {
		if strings.HasPrefix(r.URL.Path, immutablePrefix) {
			http.NotFound(w, r)
			return true
		}
		return false
	}

	h := w.Header()
	if strings.HasPrefix(r.URL.Path, immutablePrefix) {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "public, max-age=0, must-revalidate")
		h.Set("ETag", f.etag)
	}
	if ct := mime.TypeByExtension(path.Ext(f.name)); ct != "" {
		h.Set("Content-Type", ct)
	}
	if f.page && a.csp != "" {
		h.Set(a.cspName, a.pageCSP(f))
	}

	name := f.name
	if f.gzip != "" || f.br != "" {
		h.Add("Vary", "Accept-Encoding")
		switch {
		case f.br != "" && accepts(r, "br"):
			name = f.br
			h.Set("Content-Encoding", "br")
		case f.gzip != "" && accepts(r, "gzip"):
			name = f.gzip
			h.Set("Content-Encoding", "gzip")
		}
	}

	file, err := a.fsys.Open(name)
	if err != nil {
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return true
	}
	defer func() { _ = file.Close() }()
	if rs, ok := file.(io.ReadSeeker); ok {
		http.ServeContent(w, r, "", time.Time{}, rs)
		return true
	}
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, file)
	}
	return true
}

// accepts reports whether the request accepts the content encoding.
func accepts(r *http.Request, encoding string) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		enc, q, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.EqualFold(strings.TrimSpace(enc), encoding) && strings.TrimSpace(q) != "q=0" {
			return true
		}
	}
	return false
}

// forPage returns the scripts (entry first, then modules to preload) and
// stylesheets for a page that renders the given Svelte components.
func (a *assets) forPage(components []string) (scripts, styles []string) {
	seen := map[string]bool{}
	seenCSS := map[string]bool{}
	var visit func(key string, isEntry bool)
	visit = func(key string, isEntry bool) {
		if seen[key] {
			return
		}
		seen[key] = true
		chunk, ok := a.vite[key]
		if !ok {
			return
		}
		if !isEntry {
			scripts = append(scripts, "/"+chunk.File)
		}
		for _, css := range chunk.CSS {
			if !seenCSS[css] {
				seenCSS[css] = true
				styles = append(styles, "/"+css)
			}
		}
		for _, imp := range chunk.Imports {
			visit(imp, false)
		}
	}
	scripts = append(scripts, "/"+a.vite[a.entry].File)
	visit(a.entry, true)
	for _, c := range components {
		visit(c, false)
	}
	return scripts, styles
}
