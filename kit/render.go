package kit

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"maps"
	"net/http"
	"strings"
)

// payload is the data sent to the client, both inline in the HTML shell
// and from __data.json and enhanced form actions.
type payload struct {
	// Type is "data", "error", "redirect", "success" or "failure".
	Type     string            `json:"type"`
	Route    string            `json:"route,omitempty"`
	Params   map[string]string `json:"params,omitempty"`
	Status   int               `json:"status"`
	Nodes    []json.RawMessage `json:"nodes,omitempty"`
	Uses     []*nodeUses       `json:"uses,omitempty"`
	Reused   []int             `json:"reused,omitempty"`
	Failed   *int              `json:"failed,omitempty"`
	Error    *errorBody        `json:"error,omitempty"`
	Location string            `json:"location,omitempty"`
	Data     any               `json:"data,omitempty"`
	Form     any               `json:"form,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	Locale   string            `json:"locale,omitempty"`
	HasForm  bool              `json:"hasForm,omitempty"`
}

type errorBody struct {
	Message string `json:"message"`
}

func (a *App) writeDataJSON(c *Ctx, status int, p payload) {
	p.Locale = c.rootCtx().locale
	b, err := json.Marshal(p)
	if err != nil {
		a.logError(c, fmt.Errorf("kit: encoding response: %w", err))
		b = []byte(`{"type":"error","status":500,"error":{"message":"Internal Error"}}`)
		status = http.StatusInternalServerError
	}
	c.applyHeaders()
	h := c.writer.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "private, no-store")
	c.writer.WriteHeader(status)
	_, _ = c.writer.Write(b)
	if p.Type == "data" && len(c.pendingDeferreds()) > 0 {
		// Deferred values follow as one JSON object per line.
		_, _ = c.writer.Write([]byte("\n"))
		_ = http.NewResponseController(c.writer).Flush()
		a.streamDeferred(c, func(chunk []byte) {
			_, _ = c.writer.Write(append(chunk, '\n'))
		})
	}
}

// shell is a parsed src/app.html.
type shell struct {
	parts []string // text between placeholders
	slots []string // placeholder names, len(parts)-1
}

var placeholders = []string{"%tern.head%", "%tern.body%", "%tern.assets%", "%tern.nonce%", "%tern.lang%"}

func parseShell(src string) (*shell, error) {
	if src == "" {
		return nil, errors.New("kit: src/app.html is empty")
	}
	for _, required := range []string{"%tern.head%", "%tern.body%"} {
		if !strings.Contains(src, required) {
			return nil, fmt.Errorf("kit: src/app.html must contain %s", required)
		}
	}
	s := &shell{}
	rest := src
	for {
		idx, which := -1, ""
		for _, p := range placeholders {
			if i := strings.Index(rest, p); i >= 0 && (idx < 0 || i < idx) {
				idx, which = i, p
			}
		}
		if idx < 0 {
			s.parts = append(s.parts, rest)
			return s, nil
		}
		s.parts = append(s.parts, rest[:idx])
		s.slots = append(s.slots, which)
		rest = rest[idx+len(which):]
	}
}

func (s *shell) render(head, body, nonce, base, lang string) string {
	var b strings.Builder
	for i, part := range s.parts {
		b.WriteString(part)
		if i < len(s.slots) {
			switch s.slots[i] {
			case "%tern.head%":
				b.WriteString(head)
			case "%tern.body%":
				b.WriteString(body)
			case "%tern.nonce%":
				b.WriteString(nonce)
			case "%tern.assets%":
				b.WriteString(base)
			case "%tern.lang%":
				b.WriteString(lang)
			}
		}
	}
	return b.String()
}

// writeDocument responds with the HTML shell for a page or error.
func (a *App) writeDocument(c *Ctx, status int, def *RouteDef, p payload) {
	if len(a.publicEnv) > 0 {
		p.Env = a.publicEnv // full page loads only; the client keeps it
	}
	p.Locale = c.rootCtx().locale
	data, err := json.Marshal(p) // escapes <, > and & for safe inlining
	if err != nil {
		a.logError(c, fmt.Errorf("kit: encoding page data: %w", err))
		status = http.StatusInternalServerError
		data = []byte(`{"type":"error","status":500,"error":{"message":"Internal Error"}}`)
	}

	nonce := newNonce()
	var head strings.Builder
	shell := a.shell
	if a.dev != nil {
		var err error
		if shell, err = a.dev.shell(); err != nil {
			a.logError(c, err)
			http.Error(c.writer, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, src := range a.dev.scripts(a.m.BasePath) {
			fmt.Fprintf(&head, "\n\t\t<script type=\"module\" src=\"%s\" nonce=\"%s\"></script>", html.EscapeString(src), nonce)
		}
	} else if a.assets != nil {
		components := a.components(def)
		if l := c.rootCtx().locale; l != "" {
			// Preload the locale's messages with the page's code.
			components = append(components, "src/locales/"+l+".json")
			if d := a.i18n.Default; d != l {
				components = append(components, "src/locales/"+d+".json")
			}
		}
		scripts, styles := a.assets.forPage(components)
		base := a.m.BasePath
		for _, href := range styles {
			fmt.Fprintf(&head, "\n\t\t<link rel=\"stylesheet\" href=\"%s\">", html.EscapeString(base+href))
		}
		for _, href := range scripts[1:] {
			fmt.Fprintf(&head, "\n\t\t<link rel=\"modulepreload\" href=\"%s\">", html.EscapeString(base+href))
		}
		fmt.Fprintf(&head, "\n\t\t<script type=\"module\" src=\"%s\" nonce=\"%s\"></script>", html.EscapeString(base+scripts[0]), nonce)
	}
	for _, fn := range a.head {
		if h := fn(c, nonce); h != "" {
			head.WriteString("\n\t\t")
			head.WriteString(h)
		}
	}
	streaming := len(c.pendingDeferreds()) > 0 && c.Request.Method != http.MethodHead
	if streaming {
		fmt.Fprintf(&head, "\n\t\t<script nonce=\"%s\">%s</script>", nonce, resolveScript)
	}
	fmt.Fprintf(&head, "\n\t\t<script type=\"application/json\" id=\"__tern_data\" nonce=\"%s\">%s</script>", nonce, data)

	headHTML := head.String()
	body := `<div id="tern-root" style="display: contents"></div>`
	if a.ssr != nil && (def == nil || def.Page == nil || !def.Page.NoSSR) {
		res, err := a.ssr.Render(context.WithValue(c.Request.Context(), liveLocalsKey{}, maps.Clone(c.Locals)), data, absoluteURL(c.Request, a.m.BasePath))
		switch {
		case err != nil:
			// Render on the client instead; the page still works.
			a.logger.Warn("tern: server rendering failed; falling back to client rendering",
				"path", c.Request.URL.Path, "route", c.routeID, "error", err)
		case !res.Skip:
			if res.Head != "" {
				headHTML = "\n\t\t" + res.Head + headHTML
			}
			body = `<div id="tern-root" data-tern-ssr style="display: contents">` + res.Body + `</div>`
		}
	}
	doc := shell.render(headHTML, body, nonce, a.m.BasePath, a.htmlLang(c))

	c.applyHeaders()
	h := c.writer.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	if a.opts.csp != "" {
		name := "Content-Security-Policy"
		if a.opts.cspReportOnly {
			name += "-Report-Only"
		}
		h.Set(name, withNonce(a.opts.csp, nonce))
	}
	if h.Get("Cache-Control") == "" {
		h.Set("Cache-Control", "private, no-cache")
	}
	c.writer.WriteHeader(status)
	if c.Request.Method == http.MethodHead {
		return
	}
	if !streaming {
		_, _ = c.writer.Write([]byte(doc))
		return
	}
	// Send the page now, then each deferred value as it resolves.
	before, after := splitBody(doc)
	_, _ = c.writer.Write([]byte(before))
	_ = http.NewResponseController(c.writer).Flush()
	a.streamDeferred(c, func(chunk []byte) {
		fmt.Fprintf(c.writer, "<script nonce=\"%s\">__tern_resolve(%s)</script>\n", nonce, chunk)
	})
	_, _ = c.writer.Write([]byte(after))
}

// absoluteURL reconstructs the URL the client requested.
func absoluteURL(r *http.Request, base string) string {
	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host + base + r.URL.RequestURI()
}

// components returns the Svelte files a page renders, for preloading.
func (a *App) components(def *RouteDef) []string {
	layouts := a.m.Fallback.Layouts
	if def != nil {
		layouts = def.Layouts
	}
	var out []string
	for _, li := range layouts {
		if c := a.m.Layouts[li].Component; c != "" {
			out = append(out, c)
		}
	}
	if def != nil && def.Page != nil {
		out = append(out, def.Page.Component)
	}
	return out
}

// withNonce adds 'nonce-…' to a policy's script-src directive, creating it
// from default-src (or 'self') when absent.
func withNonce(policy, nonce string) string {
	token := "'nonce-" + nonce + "'"
	directives := strings.Split(policy, ";")
	var defaultSrc string
	for i, d := range directives {
		fields := strings.Fields(d)
		if len(fields) == 0 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "script-src":
			directives[i] = strings.TrimSpace(d) + " " + token
			return strings.Join(trimAll(directives), "; ")
		case "default-src":
			defaultSrc = strings.Join(fields[1:], " ")
		}
	}
	if defaultSrc == "" {
		defaultSrc = "'self'"
	}
	return strings.Join(append(trimAll(directives), "script-src "+defaultSrc+" "+token), "; ")
}

func trimAll(parts []string) []string {
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func newNonce() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return base64.RawStdEncoding.EncodeToString(b[:])
}
